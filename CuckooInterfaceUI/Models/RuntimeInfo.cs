// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
using System.Collections.Generic;
using System.Text.Json.Serialization;
using System.Windows.Media;

namespace CuckooInterfaceUI.Models
{
    /// <summary>
    /// 与 Core RuntimeManager 下发的运行时信息对齐。
    /// </summary>
    public class RuntimeInfo
    {
        [JsonPropertyName("kernelId")]
        public string KernelId { get; set; } = string.Empty;

        [JsonPropertyName("name")]
        public string Name { get; set; } = string.Empty;

        [JsonPropertyName("kernelVersion")]
        public string KernelVersion { get; set; } = string.Empty;

        [JsonPropertyName("runtime")]
        public string Runtime { get; set; } = string.Empty;

        [JsonPropertyName("status")]
        public string Status { get; set; } = "stopped";

        [JsonPropertyName("initialized")]
        public bool Initialized { get; set; }

        [JsonPropertyName("loopStarted")]
        public bool LoopStarted { get; set; }

        [JsonPropertyName("runtimeVersion")]
        public string RuntimeVersion { get; set; } = string.Empty;

        [JsonPropertyName("home")]
        public string Home { get; set; } = string.Empty;

        [JsonPropertyName("source")]
        public string Source { get; set; } = string.Empty;

        [JsonPropertyName("embedded")]
        public bool Embedded { get; set; }

        [JsonPropertyName("pluginCount")]
        public int PluginCount { get; set; }

        [JsonPropertyName("capabilities")]
        public List<string> Capabilities { get; set; } = new();

        [JsonPropertyName("lastError")]
        public string LastError { get; set; } = string.Empty;

        public string RuntimeText => Runtime.ToLowerInvariant() switch
        {
            "python" => "Python",
            "lua" => "Lua",
            _ => string.IsNullOrWhiteSpace(Runtime) ? "未知运行时" : Runtime
        };

        public string StatusText => Status.ToLowerInvariant() switch
        {
            "running" => "运行中",
            "starting" => "启动中",
            "stopped" => "已停止",
            "error" => "异常",
            _ => Status
        };

        public Brush StatusColor => Status.ToLowerInvariant() switch
        {
            "running" => new SolidColorBrush(Color.FromRgb(0x63, 0xD3, 0x8B)),
            "starting" => new SolidColorBrush(Color.FromRgb(0xFF, 0xB9, 0x00)),
            "error" => new SolidColorBrush(Color.FromRgb(0xFF, 0x72, 0x72)),
            _ => new SolidColorBrush(Color.FromRgb(0x9A, 0xA0, 0xA6))
        };

        public string SourceText => Source.ToLowerInvariant() switch
        {
            "embedded" => "内置运行时",
            "system" => "系统安装",
            "config" => "手动选择",
            _ => "—"
        };

        public string VersionText => string.IsNullOrWhiteSpace(RuntimeVersion)
            ? $"Kernel {KernelVersion}"
            : $"{RuntimeText} {RuntimeVersion} · Kernel {KernelVersion}";

        public bool SupportsControlPlane => Capabilities.Count > 0;

        public string CapabilitiesText => Capabilities.Count == 0
            ? "未实现 Runtime Control Plane"
            : string.Join(" / ", Capabilities);

        /// <summary>运行时列表状态签名，用于自动刷新时避免无谓重建。</summary>
        public string Signature =>
            $"{KernelId}:{Status}:{RuntimeVersion}:{Home}:{PluginCount}:{LoopStarted}";
    }

    /// <summary>
    /// 选择运行时后的结果。
    /// </summary>
    public class RuntimeSelectionResult
    {
        [JsonPropertyName("ok")]
        public bool Ok { get; set; }

        [JsonPropertyName("requiresRestart")]
        public bool RequiresRestart { get; set; }

        [JsonPropertyName("selectedHome")]
        public string SelectedHome { get; set; } = string.Empty;

        [JsonPropertyName("effectiveHome")]
        public string EffectiveHome { get; set; } = string.Empty;

        [JsonPropertyName("effectiveVersion")]
        public string EffectiveVersion { get; set; } = string.Empty;

        [JsonPropertyName("effectiveSource")]
        public string EffectiveSource { get; set; } = string.Empty;
    }

    /// <summary>
    /// Kernel 通过 Runtime Control Plane 声明的操作。
    /// </summary>
    public class RuntimeOperation
    {
        [JsonPropertyName("id")]
        public string Id { get; set; } = string.Empty;

        [JsonPropertyName("name")]
        public string Name { get; set; } = string.Empty;

        [JsonPropertyName("description")]
        public string Description { get; set; } = string.Empty;
    }

    public class RuntimeOperationResult
    {
        [JsonPropertyName("ok")]
        public bool Ok { get; set; }

        [JsonPropertyName("error")]
        public string Error { get; set; } = string.Empty;
    }
}
