using System;
using System.Collections.Generic;
using System.Collections.ObjectModel;
using System.Linq;
using CuckooInterfaceUI.Models;

namespace CuckooInterfaceUI.Services
{
    /// <summary>
    /// Mock 后端接口服务，模拟 Go 核心的 IPC 接口。
    /// 实际部署时可替换为命名管道通信实现。
    /// </summary>
    public class MockBackend
    {
        private static readonly Lazy<MockBackend> _instance = new(() => new MockBackend());
        public static MockBackend Instance => _instance.Value;

        // ===== 内存数据存储 =====
        private readonly ObservableCollection<PluginInfo> _plugins = new();
        private readonly ObservableCollection<EventRecord> _events = new();
        private readonly List<RegisteredEvent> _registeredEvents = new();
        private readonly List<EventOption> _eventOptions = new();
        private readonly ObservableCollection<LogEntry> _logs = new();
        private SettingPanel _settingsPanel = new();

        private readonly Random _random = new();

        private MockBackend()
        {
            SeedMockData();
        }

        // ===== 数据初始化 =====

        private void SeedMockData()
        {
            // 设置面板（镜像 core/config/globalPage.go 的 BuildSettingsPanel）
            _settingsPanel = BuildSettingsPanel();

            // 插件
            var seedPlugins = new List<PluginInfo>
            {
                new() { Id = "lua-kernel", Name = "Lua 内核", Version = "1.2.0", Description = "提供 Lua 5.4 运行时，支持脚本类插件执行", Type = PluginType.Kernel, IsEnabled = true, IsLoaded = true, Author = "CuckooTeam", ProvidedEvents = new List<string>(), Listeners = new List<string>() },
                new() { Id = "python-kernel", Name = "Python 内核", Version = "0.9.1", Description = "提供 Python 3.11 运行时", Type = PluginType.Kernel, IsEnabled = true, IsLoaded = true, Author = "CuckooTeam", ProvidedEvents = new List<string>(), Listeners = new List<string>() },
                new() { Id = "device-monitor", Name = "设备监控", Version = "2.1.0", Description = "监控电教设备连接状态，产生设备事件", Type = PluginType.Base, IsEnabled = true, IsLoaded = true, Author = "CuckooTeam", ProvidedEvents = new List<string> { "foundation.device.connected", "foundation.device.disconnected" }, Listeners = new List<string>() },
                new() { Id = "process-watcher", Name = "进程监视", Version = "1.0.5", Description = "监视关键进程的运行状态", Type = PluginType.Base, IsEnabled = true, IsLoaded = true, Author = "CuckooTeam", ProvidedEvents = new List<string> { "foundation.process.exited" }, Listeners = new List<string>() },
                new() { Id = "network-scanner", Name = "网络扫描", Version = "1.3.2", Description = "扫描局域网内在线设备", Type = PluginType.Base, IsEnabled = false, IsLoaded = false, Author = "Community", ProvidedEvents = new List<string> { "network.lan.changed" }, Listeners = new List<string>() },
                new() { Id = "sys-info", Name = "系统信息", Version = "1.1.0", Description = "采集 CPU、内存等系统指标", Type = PluginType.Base, IsEnabled = true, IsLoaded = true, Author = "CuckooTeam", ProvidedEvents = new List<string> { "sys.cpu.high", "sys.memory.high" }, Listeners = new List<string>() },
                new() { Id = "class-timer", Name = "课时计时", Version = "3.0.0", Description = "自动记录上下课时间并生成报表", Type = PluginType.Active, IsEnabled = true, IsLoaded = true, Author = "CuckooTeam", ProvidedEvents = new List<string>(), Listeners = new List<string> { "foundation.device.connected" } },
                new() { Id = "auto-restart", Name = "自动重启", Version = "1.0.0", Description = "设备异常断开后自动重启", Type = PluginType.Active, IsEnabled = false, IsLoaded = false, Author = "Community", ProvidedEvents = new List<string>(), Listeners = new List<string> { "foundation.device.disconnected" } },
                new() { Id = "screen-capture", Name = "屏幕捕获", Version = "2.2.1", Description = "定时捕获屏幕并上传", Type = PluginType.Active, IsEnabled = true, IsLoaded = true, Author = "CuckooTeam", ProvidedEvents = new List<string>(), Listeners = new List<string> { "sys.cpu.high" } },
                new() { Id = "file-watcher", Name = "文件监听", Version = "1.0.0", Description = "监听指定目录的文件变化", Type = PluginType.Base, IsEnabled = true, IsLoaded = true, Author = "Community", ProvidedEvents = new List<string> { "file.changed" }, Listeners = new List<string>() }
            };
            foreach (var p in seedPlugins) _plugins.Add(p);

            // 已注册事件
            _registeredEvents.AddRange(new List<RegisteredEvent>
            {
                new() { EventName = "foundation.device.connected", Provider = "设备监控", ListenerCount = 3 },
                new() { EventName = "foundation.device.disconnected", Provider = "设备监控", ListenerCount = 2 },
                new() { EventName = "foundation.process.exited", Provider = "进程监视", ListenerCount = 1 },
                new() { EventName = "network.lan.changed", Provider = "网络扫描", ListenerCount = 4 },
                new() { EventName = "sys.cpu.high", Provider = "系统信息", ListenerCount = 2 },
                new() { EventName = "sys.memory.high", Provider = "系统信息", ListenerCount = 1 },
                new() { EventName = "file.changed", Provider = "文件监听", ListenerCount = 0 },
                new() { EventName = "plugin.load.error", Provider = "PluginManager", ListenerCount = 2 }
            });

            // 可发布事件
            _eventOptions.AddRange(new List<EventOption>
            {
                new() { Name = "foundation.device.connected", DefaultPayload = "{\"device_id\":\"proj-01\",\"type\":\"projector\"}" },
                new() { Name = "foundation.device.disconnected", DefaultPayload = "{\"device_id\":\"proj-01\"}" },
                new() { Name = "foundation.process.exited", DefaultPayload = "{\"pid\":1234,\"name\":\"teacher_tool.exe\"}" },
                new() { Name = "network.lan.changed", DefaultPayload = "{\"online_count\":32}" },
                new() { Name = "sys.cpu.high", DefaultPayload = "{\"usage\":92.5}" },
                new() { Name = "plugin.load.error", DefaultPayload = "{\"plugin\":\"课时计时\",\"reason\":\"listener not found\"}" }
            });

            // 历史事件
            var now = DateTime.Now;
            _events.Add(new EventRecord { Timestamp = now.AddSeconds(-40), EventName = "foundation.device.connected", Payload = "{\"device_id\":\"proj-01\",\"type\":\"projector\"}", Level = EventLevel.Normal });
            _events.Add(new EventRecord { Timestamp = now.AddSeconds(-120), EventName = "foundation.process.exited", Payload = "{\"pid\":1234,\"name\":\"teacher_tool.exe\"}", Level = EventLevel.Warning });
            _events.Add(new EventRecord { Timestamp = now.AddSeconds(-260), EventName = "network.lan.changed", Payload = "{\"online_count\":32}", Level = EventLevel.Normal });
            _events.Add(new EventRecord { Timestamp = now.AddSeconds(-400), EventName = "foundation.device.connected", Payload = "{\"device_id\":\"pc-lab-07\",\"type\":\"computer\"}", Level = EventLevel.Normal });
            _events.Add(new EventRecord { Timestamp = now.AddSeconds(-600), EventName = "plugin.load.error", Payload = "{\"plugin\":\"课时计时\",\"reason\":\"listener not found\"}", Level = EventLevel.Error });

            // 日志
            var logTimes = new[] { 10, 45, 120, 300, 500, 800, 1200, 1800 };
            var logMessages = new (LogLevel, string, string)[]
            {
                (LogLevel.Info, "CuckooInterface 内核启动完成", "core"),
                (LogLevel.Debug, "加载插件配置: 10 个插件", "plugin_manager"),
                (LogLevel.Info, "Kernel [lua-kernel] 初始化成功", "kernel"),
                (LogLevel.Warn, "Plugin [network-scanner] 加载失败: 已禁用", "plugin_manager"),
                (LogLevel.Info, "事件总线启动: 16 workers", "event_bus"),
                (LogLevel.Error, "Kernel [python-kernel] 进入循环超时", "kernel"),
                (LogLevel.Info, "IPC 命名管道已就绪: \\\\.\\pipe\\CuckooInterfaceBridge", "ipc"),
                (LogLevel.Debug, "UI 桥接连接成功", "ipc")
            };
            for (int i = 0; i < logMessages.Length; i++)
            {
                var (lvl, msg, logger) = logMessages[i];
                _logs.Add(new LogEntry { Timestamp = now.AddSeconds(-logTimes[i]), Level = lvl, Message = msg, Logger = logger });
            }
        }

        // ===== 插件接口 =====

        public ObservableCollection<PluginInfo> GetPlugins() => _plugins;

        public PluginInfo? GetPlugin(string id) => _plugins.FirstOrDefault(p => p.Id == id);

        public bool TogglePlugin(string id, bool enabled)
        {
            var plugin = GetPlugin(id);
            if (plugin == null) return false;
            plugin.IsEnabled = enabled;
            plugin.IsLoaded = enabled;
            AddLog(enabled ? LogLevel.Info : LogLevel.Warn,
                $"插件 [{plugin.Name}] 已{(enabled ? "启用" : "禁用")}", "plugin_manager");
            return true;
        }

        public bool RemovePlugin(string id)
        {
            var plugin = GetPlugin(id);
            if (plugin == null) return false;
            _plugins.Remove(plugin);
            AddLog(LogLevel.Warn, $"插件 [{plugin.Name}] 已卸载", "plugin_manager");
            return true;
        }

        // ===== 事件接口 =====

        public ObservableCollection<EventRecord> GetRecentEvents() => _events;

        public List<RegisteredEvent> GetRegisteredEvents() => _registeredEvents;

        public List<EventOption> GetEventOptions() => _eventOptions;

        public EventRecord PublishEvent(string eventName, string payload)
        {
            var record = new EventRecord
            {
                Timestamp = DateTime.Now,
                EventName = eventName,
                Payload = string.IsNullOrWhiteSpace(payload) ? "{}" : payload.Trim(),
                Level = EventLevel.Normal
            };
            _events.Insert(0, record);
            if (_events.Count > 200) _events.RemoveAt(_events.Count - 1);
            AddLog(LogLevel.Debug, $"事件发布: {eventName}", "event_bus");
            return record;
        }

        // ===== 设置接口 =====

        // 外观设置为前端固定页面，其值由后端单独存储（不在 SettingPanel 结构中）
        private readonly Dictionary<string, object> _appearanceValues = new()
        {
            { "theme", "system" },
            { "accentColor", "#0078D4" },
            { "fontScale", 100.0 }
        };

        /// <summary>外观设置的 key 集合，用于 SaveSettings 时区分存储目标。</summary>
        private static readonly HashSet<string> AppearanceKeys = new() { "theme", "accentColor", "fontScale" };

        public SettingPanel GetSettingsPanel() => _settingsPanel;

        /// <summary>获取外观设置的当前值（前端固定页面使用）。</summary>
        public IReadOnlyDictionary<string, object> GetAppearanceSettings() => _appearanceValues;

        /// <summary>
        /// 保存设置：按 key 更新各 Setting 的 Value。
        /// 外观设置（theme/accentColor/fontScale）存入独立字典，其余存入 SettingPanel。
        /// </summary>
        public void SaveSettings(IDictionary<string, object> values)
        {
            foreach (var kv in values)
            {
                if (AppearanceKeys.Contains(kv.Key))
                {
                    _appearanceValues[kv.Key] = kv.Value;
                    continue;
                }

                foreach (var page in _settingsPanel.Pages)
                {
                    foreach (var section in page.Sections)
                    {
                        foreach (var setting in section.Settings)
                        {
                            if (setting.Key == kv.Key)
                            {
                                setting.Value = kv.Value;
                            }
                        }
                    }
                }
            }
            AddLog(LogLevel.Info, "设置已保存", "config");
        }

        // ===== 日志接口 =====

        public ObservableCollection<LogEntry> GetLogs() => _logs;

        public void AddLog(LogLevel level, string message, string logger = "core")
        {
            _logs.Insert(0, new LogEntry { Timestamp = DateTime.Now, Level = level, Message = message, Logger = logger });
            if (_logs.Count > 500) _logs.RemoveAt(_logs.Count - 1);
        }

        // ===== 概览接口 =====

        public SystemOverview GetOverview()
        {
            return new SystemOverview
            {
                TotalPlugins = _plugins.Count,
                ActivePlugins = _plugins.Count(p => p.IsEnabled),
                TotalKernels = _plugins.Count(p => p.Type == PluginType.Kernel),
                ActiveKernels = _plugins.Count(p => p.Type == PluginType.Kernel && p.IsEnabled),
                EventCountToday = _events.Count,
                ErrorCountToday = _events.Count(e => e.Level == EventLevel.Error),
                CpuUsage = Math.Round(_random.NextDouble() * 40 + 10, 1),
                MemoryUsageMB = Math.Round(_random.NextDouble() * 200 + 80, 1),
                Uptime = "2h 37m"
            };
        }

        // ===== 设置面板构建（镜像 core/config/globalPage.go 的 BuildSettingsPanel） =====

        private SettingPanel BuildSettingsPanel()
        {
            return new SettingPanel
            {
                Pages = new List<SettingPage>
                {
                    // 1. 通用
                    new SettingPage
                    {
                        Key = "general", Name = "通用",
                        Sections = new List<SettingSection>
                        {
                            new SettingSection
                            {
                                Key = "startup", Name = "启动",
                                Settings = new List<Setting>
                                {
                                    NewSetting("autoStart", "开机自启动", "登录系统后自动启动 CuckooInterface",
                                        new SwitchEditor(), false)
                                }
                            },
                            new SettingSection
                            {
                                Key = "language", Name = "语言",
                                Settings = new List<Setting>
                                {
                                    NewSetting("language", "界面显示语言", "选择应用程序的显示语言",
                                        new SelectEditor { Options = new List<TextOption>
                                        {
                                            new() { Value = "zh-CN", Text = "简体中文" },
                                            new() { Value = "en-US", Text = "English" }
                                        }}, "zh-CN")
                                }
                            },
                            new SettingSection
                            {
                                Key = "instance", Name = "单实例",
                                Settings = new List<Setting>
                                {
                                    NewSetting("singleInstance", "仅允许运行一个实例", "新启动的实例将通知旧实例退出",
                                        new SwitchEditor(), true)
                                }
                            }
                        }
                    },

                    // 2. 日志
                    new SettingPage
                    {
                        Key = "logging", Name = "日志",
                        Sections = new List<SettingSection>
                        {
                            new SettingSection
                            {
                                Key = "level", Name = "日志级别",
                                Settings = new List<Setting>
                                {
                                    NewSetting("logLevel", "日志级别", "控制日志输出的详细程度",
                                        new SelectEditor { Options = new List<TextOption>
                                        {
                                            new() { Value = "debug", Text = "Debug" },
                                            new() { Value = "info", Text = "Info" },
                                            new() { Value = "warn", Text = "Warn" },
                                            new() { Value = "error", Text = "Error" }
                                        }}, "info")
                                }
                            },
                            new SettingSection
                            {
                                Key = "maxSize", Name = "文件大小限制",
                                Settings = new List<Setting>
                                {
                                    NewSetting("logMaxSize", "单文件最大大小", "当日志文件超过此大小时进行轮转",
                                        new SliderEditor { Min = 10, Max = 1024, Step = 10, Unit = "MB" },
                                        100.0, displayDescription: false)
                                }
                            },
                            new SettingSection
                            {
                                Key = "compression", Name = "压缩策略",
                                Settings = new List<Setting>
                                {
                                    NewSetting("logCompress", "压缩旧日志", "对超过保留天数的日志文件进行压缩以节省空间",
                                        new SwitchEditor(), true)
                                }
                            }
                        }
                    },

                    // 3. 插件
                    new SettingPage
                    {
                        Key = "plugins", Name = "插件",
                        Sections = new List<SettingSection>
                        {
                            new SettingSection
                            {
                                Key = "loading", Name = "加载行为",
                                Settings = new List<Setting>
                                {
                                    NewSetting("autoLoadPlugins", "启动时自动加载插件", "程序启动后自动扫描并加载所有已安装插件",
                                        new SwitchEditor(), true),
                                    NewSetting("continueOnError", "加载失败时继续", "某个插件加载失败时不中断整体启动流程",
                                        new SwitchEditor(), true)
                                }
                            },
                            new SettingSection
                            {
                                Key = "directory", Name = "插件目录",
                                Settings = new List<Setting>
                                {
                                    new Setting
                                    {
                                        Key = "pluginDir", Name = "插件目录路径", Description = "存放插件文件的文件夹路径",
                                        DisplayName = true, DisplayDescription = false,
                                        Editor = new TextEditor { ReadOnly = true },
                                        Actions = new List<SettingAction>
                                        {
                                            new() { Key = "browsePluginDir", Type = ActionType.BrowseDirectory, Text = "浏览" }
                                        },
                                        Value = ".\\plugins", DefaultValue = ".\\plugins"
                                    }
                                }
                            }
                        }
                    }
                }
            };
        }

        /// <summary>
        /// 构建设置项的辅助方法，简化重复性代码。
        /// </summary>
        private static Setting NewSetting(string key, string name, string description, Editor editor,
            object value, object? defaultValue = null, bool displayName = true, bool displayDescription = true)
        {
            return new Setting
            {
                Key = key,
                Name = name,
                Description = description,
                DisplayName = displayName,
                DisplayDescription = displayDescription,
                Editor = editor,
                Value = value,
                DefaultValue = defaultValue ?? value
            };
        }
    }
}
