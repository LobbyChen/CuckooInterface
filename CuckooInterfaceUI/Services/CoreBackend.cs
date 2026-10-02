using System;
using System.Buffers.Binary;
using System.Collections.Generic;
using System.IO;
using System.IO.Pipes;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading;
using System.Threading.Tasks;
using CuckooInterfaceUI.Models;

namespace CuckooInterfaceUI.Services
{
    /// <summary>
    /// CuckooInterface UI 的真实后端客户端。
    /// Core 和 Daemon 分别使用独立的 Windows Named Pipe，传输协议与 Go 端一致：
    /// [0x6F][uint32 big-endian payload length][JSON payload][CRC32]
    /// </summary>
    public sealed class CoreBackend
    {
        private const string CorePipeName = "CuckooCoreInterfaceBridge";
        private const string DaemonPipeName = "CuckooDaemonInterfaceBridge";
        private const byte PacketMarker = 0x6F;
        private const int MaxPayloadLength = 1024 * 1024;
        private const int ConnectTimeoutMilliseconds = 1500;
        private const int ConnectAttempts = 3;

        private static readonly Lazy<CoreBackend> InstanceHolder = new(() => new CoreBackend());
        private static long _requestId;

        private static readonly JsonSerializerOptions JsonOptions = CreateJsonOptions();

        private readonly object _connectionStateLock = new();
        private bool _isCoreConnected;
        private bool _isDaemonConnected;

        public event EventHandler<BackendConnectionStateChangedEventArgs>? ConnectionStateChanged;

        public bool IsCoreConnected
        {
            get
            {
                lock (_connectionStateLock)
                    return _isCoreConnected;
            }
        }

        public bool IsDaemonConnected
        {
            get
            {
                lock (_connectionStateLock)
                    return _isDaemonConnected;
            }
        }

        public static CoreBackend Instance => InstanceHolder.Value;

        private CoreBackend()
        {
        }

        // ===== 概览 =====

        public Task<SystemOverview> GetOverviewAsync(CancellationToken cancellationToken = default) =>
            CallCoreAsync<SystemOverview>("getOverview", null, cancellationToken);

        // ===== 插件 =====

        public Task<List<PluginInfo>> GetPluginsAsync(CancellationToken cancellationToken = default) =>
            CallCoreAsync<List<PluginInfo>>("getPlugins", null, cancellationToken);

        public Task<bool> TogglePluginAsync(string id, bool enabled, CancellationToken cancellationToken = default) =>
            CallCoreAsync<bool>("togglePlugin", new { id, enabled }, cancellationToken);

        public Task<bool> RemovePluginAsync(string id, CancellationToken cancellationToken = default) =>
            CallCoreAsync<bool>("removePlugin", new { id }, cancellationToken);

        public Task<bool> LoadPluginAsync(string id, string type, CancellationToken cancellationToken = default) =>
            CallCoreAsync<bool>("loadPlugin", new { id, type }, cancellationToken);

        public Task<bool> UnloadPluginAsync(string id, string type, CancellationToken cancellationToken = default) =>
            CallCoreAsync<bool>("unloadPlugin", new { id, type }, cancellationToken);

        public Task<bool> InstallPluginAsync(string type, string path, CancellationToken cancellationToken = default) =>
            CallCoreAsync<bool>("installPlugin", new { type, path }, cancellationToken);

        // ===== 事件 =====

        public async Task<List<EventRecord>> GetRecentEventsAsync(CancellationToken cancellationToken = default)
        {
            var data = await CallCoreAsync<List<EventRecordDto>>("getRecentEvents", null, cancellationToken);
            var result = new List<EventRecord>(data.Count);
            foreach (var item in data)
            {
                result.Add(new EventRecord
                {
                    Timestamp = DateTimeOffset.FromUnixTimeSeconds(item.Timestamp).LocalDateTime,
                    EventName = item.EventName,
                    Payload = string.IsNullOrWhiteSpace(item.Payload) ? "{}" : item.Payload,
                    Level = ParseEventLevel(item.Level)
                });
            }
            return result;
        }

        public Task<List<RegisteredEvent>> GetRegisteredEventsAsync(CancellationToken cancellationToken = default) =>
            CallCoreAsync<List<RegisteredEvent>>("getRegisteredEvents", null, cancellationToken);

        public Task<List<EventOption>> GetEventOptionsAsync(CancellationToken cancellationToken = default) =>
            CallCoreAsync<List<EventOption>>("getEventOptions", null, cancellationToken);

        public async Task<EventRecord> PublishEventAsync(string eventName, string payload, CancellationToken cancellationToken = default)
        {
            var dto = await CallCoreAsync<EventRecordDto>(
                "publishEvent",
                new { name = eventName, payload = string.IsNullOrWhiteSpace(payload) ? "{}" : payload.Trim() },
                cancellationToken);

            return new EventRecord
            {
                Timestamp = DateTimeOffset.FromUnixTimeSeconds(dto.Timestamp).LocalDateTime,
                EventName = dto.EventName,
                Payload = string.IsNullOrWhiteSpace(dto.Payload) ? "{}" : dto.Payload,
                Level = ParseEventLevel(dto.Level)
            };
        }

        // ===== 日志 =====

        public async Task<List<LogEntry>> GetLogsAsync(CancellationToken cancellationToken = default)
        {
            var data = await CallCoreAsync<List<LogEntryDto>>("getLogs", null, cancellationToken);
            var result = new List<LogEntry>(data.Count);
            foreach (var item in data)
            {
                result.Add(new LogEntry
                {
                    Timestamp = DateTimeOffset.FromUnixTimeSeconds(item.Timestamp).LocalDateTime,
                    Level = ParseLogLevel(item.Level),
                    Message = item.Message,
                    Logger = item.Logger
                });
            }
            return result;
        }

        public Task<bool> ClearLogsAsync(CancellationToken cancellationToken = default) =>
            CallCoreAsync<bool>("clearLogs", null, cancellationToken);

        // ===== 设置 =====

        public Task<SettingPanel> GetSettingsPanelAsync(CancellationToken cancellationToken = default) =>
            CallCoreAsync<SettingPanel>("getSettingsPanel", null, cancellationToken);

        public Task<Dictionary<string, object>> GetAppearanceSettingsAsync(CancellationToken cancellationToken = default) =>
            CallCoreAsync<Dictionary<string, object>>("getAppearanceSettings", null, cancellationToken);

        public Task<bool> SaveSettingsAsync(Dictionary<string, object> values, CancellationToken cancellationToken = default) =>
            CallCoreAsync<bool>("saveSettings", values, cancellationToken);

        // ===== Daemon =====

        public Task<string> PingDaemonAsync(CancellationToken cancellationToken = default) =>
            CallDaemonAsync<string>("ping", null, cancellationToken);

        public Task<bool> StartCoreAsync(CancellationToken cancellationToken = default) =>
            CallDaemonAsync<bool>("startCore", null, cancellationToken);

        public Task<bool> StopCoreAsync(CancellationToken cancellationToken = default) =>
            CallDaemonAsync<bool>("stopCore", null, cancellationToken);

        public Task<bool> RestartCoreAsync(CancellationToken cancellationToken = default) =>
            CallDaemonAsync<bool>("restartCore", null, cancellationToken);

        /// <summary>
        /// 轻量级连接探测。Core 使用只读设置接口，Daemon 使用 ping。
        /// </summary>
        public async Task CheckConnectionAsync(CancellationToken cancellationToken = default)
        {
            async Task ProbeCoreAsync()
            {
                try
                {
                    await GetSettingsPanelAsync(cancellationToken);
                }
                catch (BackendConnectionException)
                {
                    // 状态已由 CallAsync 统一更新。
                }
            }

            async Task ProbeDaemonAsync()
            {
                try
                {
                    await PingDaemonAsync(cancellationToken);
                }
                catch (BackendConnectionException)
                {
                    // 状态已由 CallAsync 统一更新。
                }
            }

            await Task.WhenAll(ProbeCoreAsync(), ProbeDaemonAsync());
        }

        // ===== IPC 核心 =====

        private Task<T> CallCoreAsync<T>(string method, object? parameters, CancellationToken cancellationToken) =>
            CallAsync<T>(CorePipeName, method, parameters, cancellationToken);

        private Task<T> CallDaemonAsync<T>(string method, object? parameters, CancellationToken cancellationToken) =>
            CallAsync<T>(DaemonPipeName, method, parameters, cancellationToken);

        private async Task<T> CallAsync<T>(string pipeName, string method, object? parameters, CancellationToken cancellationToken)
        {
            var request = new IpcRequest
            {
                Id = Interlocked.Increment(ref _requestId),
                Method = method,
                Params = parameters == null ? null : JsonSerializer.SerializeToElement(parameters, JsonOptions)
            };
            var requestBytes = JsonSerializer.SerializeToUtf8Bytes(request, JsonOptions);
            if (requestBytes.Length > MaxPayloadLength)
                throw new InvalidOperationException($"IPC request is too large: {requestBytes.Length} bytes.");

            Exception? lastConnectError = null;

            for (var attempt = 1; attempt <= ConnectAttempts; attempt++)
            {
                cancellationToken.ThrowIfCancellationRequested();
                using var pipe = new NamedPipeClientStream(
                    ".",
                    pipeName,
                    PipeDirection.InOut,
                    PipeOptions.Asynchronous);

                try
                {
                    await ConnectAsync(pipe, ConnectTimeoutMilliseconds, cancellationToken);
                }
                catch (IOException ex) when (attempt < ConnectAttempts)
                {
                    lastConnectError = ex;
                    await Task.Delay(150 * attempt, cancellationToken);
                    continue;
                }
                catch (TimeoutException ex) when (attempt < ConnectAttempts)
                {
                    lastConnectError = ex;
                    await Task.Delay(150 * attempt, cancellationToken);
                    continue;
                }
                catch (IOException ex)
                {
                    lastConnectError = ex;
                    SetConnectionState(pipeName, false);
                    throw new BackendConnectionException(pipeName, ex);
                }
                catch (TimeoutException ex)
                {
                    lastConnectError = ex;
                    SetConnectionState(pipeName, false);
                    throw new BackendConnectionException(pipeName, ex);
                }

                try
                {
                    // 建立连接成功后不重发请求，避免非幂等操作在“请求已到达但响应丢失”时重复执行。
                    await WritePacketAsync(pipe, requestBytes, cancellationToken);
                    var responseBytes = await ReadPacketAsync(pipe, cancellationToken);
                    var result = DeserializeResponse<T>(responseBytes, request.Id);
                    SetConnectionState(pipeName, true);
                    return result;
                }
                catch (BackendConnectionException)
                {
                    throw;
                }
                catch (EndOfStreamException ex)
                {
                    SetConnectionState(pipeName, false);
                    throw new BackendConnectionException(pipeName, ex);
                }
                catch (IOException ex)
                {
                    SetConnectionState(pipeName, false);
                    throw new BackendConnectionException(pipeName, ex);
                }
                catch (TimeoutException ex)
                {
                    SetConnectionState(pipeName, false);
                    throw new BackendConnectionException(pipeName, ex);
                }
            }

            SetConnectionState(pipeName, false);
            throw new BackendConnectionException(pipeName, lastConnectError);
        }

        private static Task ConnectAsync(NamedPipeClientStream pipe, int timeoutMilliseconds, CancellationToken cancellationToken) =>
            pipe.ConnectAsync(timeoutMilliseconds, cancellationToken);

        private static async Task WritePacketAsync(Stream stream, byte[] payload, CancellationToken cancellationToken)
        {
            var packet = new byte[1 + 4 + payload.Length + 4];
            packet[0] = PacketMarker;
            BinaryPrimitives.WriteUInt32BigEndian(packet.AsSpan(1, 4), checked((uint)payload.Length));
            payload.CopyTo(packet, 5);
            BinaryPrimitives.WriteUInt32BigEndian(packet.AsSpan(5 + payload.Length, 4), ComputeCrc32(payload));
            await stream.WriteAsync(packet, cancellationToken);
            await stream.FlushAsync(cancellationToken);
        }

        private static async Task<byte[]> ReadPacketAsync(Stream stream, CancellationToken cancellationToken)
        {
            var marker = new byte[1];
            await ReadExactlyAsync(stream, marker, cancellationToken);
            if (marker[0] != PacketMarker)
                throw new InvalidDataException($"Invalid IPC start marker: 0x{marker[0]:X2}, expected 0x{PacketMarker:X2}.");

            var lengthBytes = new byte[4];
            await ReadExactlyAsync(stream, lengthBytes, cancellationToken);
            var payloadLength = BinaryPrimitives.ReadUInt32BigEndian(lengthBytes);
            if (payloadLength > MaxPayloadLength)
                throw new InvalidDataException($"IPC payload is too large: {payloadLength} bytes.");

            var payloadLengthInt = checked((int)payloadLength);
            var payload = new byte[payloadLengthInt];
            if (payloadLengthInt > 0)
                await ReadExactlyAsync(stream, payload, cancellationToken);

            var crcBytes = new byte[4];
            await ReadExactlyAsync(stream, crcBytes, cancellationToken);
            var expected = BinaryPrimitives.ReadUInt32BigEndian(crcBytes);
            var actual = ComputeCrc32(payload);
            if (expected != actual)
                throw new InvalidDataException($"IPC CRC mismatch: received 0x{expected:X8}, calculated 0x{actual:X8}.");

            return payload;
        }

        private static async Task ReadExactlyAsync(Stream stream, Memory<byte> buffer, CancellationToken cancellationToken)
        {
            var offset = 0;
            while (offset < buffer.Length)
            {
                var read = await stream.ReadAsync(buffer[offset..], cancellationToken);
                if (read == 0)
                    throw new EndOfStreamException("IPC pipe closed before a complete packet was received.");
                offset += read;
            }
        }

        private static T DeserializeResponse<T>(byte[] responseBytes, long expectedId)
        {
            var response = JsonSerializer.Deserialize<IpcResponse>(responseBytes, JsonOptions)
                           ?? throw new InvalidDataException("IPC response is empty.");

            if (response.Id != expectedId)
                throw new InvalidDataException($"IPC response id mismatch: expected {expectedId}, got {response.Id}.");

            if (!response.Success)
                throw new InvalidOperationException(string.IsNullOrWhiteSpace(response.Error)
                    ? "Backend request failed."
                    : response.Error);

            if (response.Data.ValueKind == JsonValueKind.Undefined || response.Data.ValueKind == JsonValueKind.Null)
                return default!;

            return JsonSerializer.Deserialize<T>(response.Data.GetRawText(), JsonOptions)
                   ?? throw new InvalidDataException("IPC response data could not be deserialized.");
        }

        private static EventLevel ParseEventLevel(string? level) => level?.ToLowerInvariant() switch
        {
            "warning" => EventLevel.Warning,
            "error" => EventLevel.Error,
            _ => EventLevel.Normal
        };

        private static LogLevel ParseLogLevel(string? level) => level?.ToLowerInvariant() switch
        {
            "debug" => LogLevel.Debug,
            "warn" => LogLevel.Warn,
            "error" => LogLevel.Error,
            _ => LogLevel.Info
        };

        private static JsonSerializerOptions CreateJsonOptions()
        {
            var options = new JsonSerializerOptions
            {
                PropertyNameCaseInsensitive = true
            };
            options.Converters.Add(new ActionTypeJsonConverter());
            options.Converters.Add(new JsonStringEnumConverter(JsonNamingPolicy.CamelCase));
            options.Converters.Add(new EditorJsonConverter());
            options.Converters.Add(new PrimitiveObjectJsonConverter());
            return options;
        }

        private static uint ComputeCrc32(ReadOnlySpan<byte> data)
        {
            uint crc = 0xFFFFFFFF;
            foreach (var value in data)
            {
                crc ^= value;
                for (var bit = 0; bit < 8; bit++)
                    crc = (crc & 1) != 0 ? (crc >> 1) ^ 0xEDB88320u : crc >> 1;
            }
            return ~crc;
        }

        private void SetConnectionState(string pipeName, bool connected)
        {
            bool changed;
            lock (_connectionStateLock)
            {
                if (pipeName == CorePipeName)
                {
                    changed = _isCoreConnected != connected;
                    _isCoreConnected = connected;
                }
                else
                {
                    changed = _isDaemonConnected != connected;
                    _isDaemonConnected = connected;
                }
            }

            if (changed)
                ConnectionStateChanged?.Invoke(this, new BackendConnectionStateChangedEventArgs(
                    pipeName == CorePipeName, connected));
        }

        // 与 Go IPC DTO 对齐的内部协议模型。
        private sealed class IpcRequest
        {
            [JsonPropertyName("id")]
            public long Id { get; set; }

            [JsonPropertyName("method")]
            public string Method { get; set; } = string.Empty;

            [JsonPropertyName("params")]
            public JsonElement? Params { get; set; }
        }

        private sealed class IpcResponse
        {
            [JsonPropertyName("id")]
            public long Id { get; set; }

            [JsonPropertyName("success")]
            public bool Success { get; set; }

            [JsonPropertyName("data")]
            public JsonElement Data { get; set; }

            [JsonPropertyName("error")]
            public string? Error { get; set; }
        }

        private sealed class EventRecordDto
        {
            [JsonPropertyName("timestamp")]
            public long Timestamp { get; set; }

            [JsonPropertyName("eventName")]
            public string EventName { get; set; } = string.Empty;

            [JsonPropertyName("payload")]
            public string Payload { get; set; } = "{}";

            [JsonPropertyName("level")]
            public string Level { get; set; } = "normal";
        }

        private sealed class LogEntryDto
        {
            [JsonPropertyName("timestamp")]
            public long Timestamp { get; set; }

            [JsonPropertyName("level")]
            public string Level { get; set; } = "INFO";

            [JsonPropertyName("message")]
            public string Message { get; set; } = string.Empty;

            [JsonPropertyName("logger")]
            public string Logger { get; set; } = string.Empty;
        }

        private sealed class ActionTypeJsonConverter : JsonConverter<ActionType>
        {
            public override ActionType Read(ref Utf8JsonReader reader, Type typeToConvert, JsonSerializerOptions options)
            {
                var value = reader.GetString()?.ToLowerInvariant();
                return value switch
                {
                    "browse-file" => ActionType.BrowseFile,
                    "browse-directory" => ActionType.BrowseDirectory,
                    "reset" => ActionType.Reset,
                    _ => throw new JsonException($"Unknown setting action type '{value}'.")
                };
            }

            public override void Write(Utf8JsonWriter writer, ActionType value, JsonSerializerOptions options)
            {
                var text = value switch
                {
                    ActionType.BrowseFile => "browse-file",
                    ActionType.BrowseDirectory => "browse-directory",
                    ActionType.Reset => "reset",
                    _ => throw new JsonException($"Unknown setting action type '{value}'.")
                };
                writer.WriteStringValue(text);
            }
        }

        private sealed class EditorJsonConverter : JsonConverter<Editor>
        {
            public override Editor? Read(ref Utf8JsonReader reader, Type typeToConvert, JsonSerializerOptions options)
            {
                using var document = JsonDocument.ParseValue(ref reader);
                var root = document.RootElement;
                if (!root.TryGetProperty("type", out var typeProperty))
                    throw new JsonException("Editor JSON is missing 'type'.");

                var type = typeProperty.GetString()?.ToLowerInvariant();
                return type switch
                {
                    "text" => JsonSerializer.Deserialize<TextEditor>(root.GetRawText(), options),
                    "slider" => JsonSerializer.Deserialize<SliderEditor>(root.GetRawText(), options),
                    "switch" => JsonSerializer.Deserialize<SwitchEditor>(root.GetRawText(), options),
                    "select" => JsonSerializer.Deserialize<SelectEditor>(root.GetRawText(), options),
                    "color" => JsonSerializer.Deserialize<ColorEditor>(root.GetRawText(), options),
                    _ => throw new JsonException($"Unknown editor type '{type}'.")
                };
            }

            public override void Write(Utf8JsonWriter writer, Editor value, JsonSerializerOptions options)
            {
                JsonSerializer.Serialize(writer, value, value.GetType(), options);
            }
        }

        private sealed class PrimitiveObjectJsonConverter : JsonConverter<object>
        {
            public override object? Read(ref Utf8JsonReader reader, Type typeToConvert, JsonSerializerOptions options)
            {
                switch (reader.TokenType)
                {
                    case JsonTokenType.String:
                        return reader.GetString();
                    case JsonTokenType.True:
                    case JsonTokenType.False:
                        return reader.GetBoolean();
                    case JsonTokenType.Number:
                        if (reader.TryGetInt64(out var longValue))
                            return longValue;
                        return reader.GetDouble();
                    case JsonTokenType.Null:
                        return null;
                    default:
                        using (var document = JsonDocument.ParseValue(ref reader))
                            return document.RootElement.Clone();
                }
            }

            public override void Write(Utf8JsonWriter writer, object value, JsonSerializerOptions options)
            {
                JsonSerializer.Serialize(writer, value, value?.GetType() ?? typeof(object), options);
            }
        }

    }

    public sealed class BackendConnectionException : IOException
    {
        public string PipeName { get; }

        public BackendConnectionException(string pipeName, Exception? innerException = null)
            : base(GetMessage(pipeName), innerException)
        {
            PipeName = pipeName;
        }

        private static string GetMessage(string pipeName) =>
            pipeName == "CuckooCoreInterfaceBridge"
                ? "无法连接到 Cuckoo Core。"
                : "无法连接到 Cuckoo Daemon。";
    }

    public sealed class BackendConnectionStateChangedEventArgs : EventArgs
    {
        public bool IsCore { get; }
        public bool IsConnected { get; }

        public BackendConnectionStateChangedEventArgs(bool isCore, bool isConnected)
        {
            IsCore = isCore;
            IsConnected = isConnected;
        }
    }
}
