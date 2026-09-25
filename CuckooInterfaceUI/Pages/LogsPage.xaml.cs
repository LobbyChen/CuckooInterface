using System.Collections.ObjectModel;
using System.Windows.Controls;
using System.Windows.Media;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// LogsPage.xaml 的交互逻辑
    /// </summary>
    public partial class LogsPage : Page
    {
        public ObservableCollection<LogItem> Logs { get; set; }

        public LogsPage()
        {
            InitializeComponent();
            LoadHardcodedLogs();
            LogsDataGrid.ItemsSource = Logs;
        }

        /// <summary>
        /// 加载硬编码的日志数据（模拟后端 Go 核心的输出）
        /// </summary>
        private void LoadHardcodedLogs()
        {
            Logs = new ObservableCollection<LogItem>
            {
                new LogItem { Time = "2023-10-27 10:00:01.123", Level = "INFO",  Source = "Main",             Message = "Initializing system folders...", LevelColor = Brushes.LimeGreen },
                new LogItem { Time = "2023-10-27 10:00:01.456", Level = "INFO",  Source = "PluginFileManager",Message = "Unzipped 5 plugin packages successfully.", LevelColor = Brushes.LimeGreen },
                new LogItem { Time = "2023-10-27 10:00:02.012", Level = "INFO",  Source = "KernelManager",    Message = "Loading kernel plugins...", LevelColor = Brushes.LimeGreen },
                new LogItem { Time = "2023-10-27 10:00:02.345", Level = "WARN",  Source = "PluginManager",    Message = "Plugin 'test_plugin' failed to register listener: event not found", LevelColor = Brushes.Orange },
                new LogItem { Time = "2023-10-27 10:00:03.789", Level = "INFO",  Source = "KernelManager",    Message = "All kernels initialized successfully.", LevelColor = Brushes.LimeGreen },
                new LogItem { Time = "2023-10-27 10:00:04.001", Level = "INFO",  Source = "KernelManager",    Message = "Starting all kernel loops...", LevelColor = Brushes.LimeGreen },
                new LogItem { Time = "2023-10-27 10:00:04.555", Level = "ERROR", Source = "SandboxHost",      Message = "Kernel 'lua_kernel' crashed with exception 0xC0000005 (Access Violation)", LevelColor = Brushes.OrangeRed },
                new LogItem { Time = "2023-10-27 10:00:05.123", Level = "INFO",  Source = "PluginManager",    Message = "Loading Base and Active plugins...", LevelColor = Brushes.LimeGreen },
                new LogItem { Time = "2023-10-27 10:00:05.678", Level = "INFO",  Source = "PluginManager",    Message = "Plugin 'ui_automation' loaded and registered.", LevelColor = Brushes.LimeGreen },
                new LogItem { Time = "2023-10-27 10:00:06.000", Level = "INFO",  Source = "Main",             Message = "CuckooInterface is now running. Press Ctrl+C to exit.", LevelColor = Brushes.LimeGreen },
                new LogItem { Time = "2023-10-27 10:05:12.890", Level = "WARN",  Source = "EventBus",         Message = "Task queue full, dropping event 'sys.cpu.high' for one listener", LevelColor = Brushes.Orange },
                new LogItem { Time = "2023-10-27 10:10:45.112", Level = "FATAL", Source = "IPC_Bridge",       Message = "Named pipe connection lost unexpectedly. Restarting listener...", LevelColor = Brushes.Red }
            };
        }
    }

    /// <summary>
    /// 日志数据模型
    /// </summary>
    public class LogItem
    {
        public string Time { get; set; } = string.Empty;
        public string Level { get; set; } = string.Empty;
        public string Source { get; set; } = string.Empty;
        public string Message { get; set; } = string.Empty;
        
        /// <summary>
        /// 用于在 UI 中绑定日志级别的颜色
        /// </summary>
        public Brush LevelColor { get; set; } = Brushes.Gray;
    }
}