using System.Collections.ObjectModel;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// EventsPage.xaml 的交互逻辑
    /// </summary>
    public partial class EventsPage : Page
    {
        public ObservableCollection<EventOption> EventOptions { get; set; }
        public ObservableCollection<RecentEvent> RecentEvents { get; set; }
        public ObservableCollection<RegisteredEvent> RegisteredEvents { get; set; }

        public EventsPage()
        {
            InitializeComponent();
            LoadHardcodedData();
            EventNameComboBox.ItemsSource = EventOptions;
            RecentEventsList.ItemsSource = RecentEvents;
            RegisteredEventsGrid.ItemsSource = RegisteredEvents;
            RegisteredCountText.Text = $"共 {RegisteredEvents.Count} 个事件";
        }

        /// <summary>
        /// 加载硬编码数据（模拟后端 Go 核心的事件总线数据）
        /// </summary>
        private void LoadHardcodedData()
        {
            // 可发布的事件列表
            EventOptions = new ObservableCollection<EventOption>
            {
                new EventOption { Name = "foundation.device.connected", DefaultPayload = "{\"device_id\":\"proj-01\",\"type\":\"projector\"}" },
                new EventOption { Name = "foundation.device.disconnected", DefaultPayload = "{\"device_id\":\"proj-01\"}" },
                new EventOption { Name = "foundation.process.exited", DefaultPayload = "{\"pid\":1234,\"name\":\"teacher_tool.exe\"}" },
                new EventOption { Name = "network.lan.changed", DefaultPayload = "{\"online_count\":32}" },
                new EventOption { Name = "sys.cpu.high", DefaultPayload = "{\"usage\":92.5}" },
                new EventOption { Name = "plugin.load.error", DefaultPayload = "{\"plugin\":\"课时计时\",\"reason\":\"listener not found\"}" }
            };

            // 历史事件记录
            RecentEvents = new ObservableCollection<RecentEvent>
            {
                new RecentEvent
                {
                    Time = "10:24:18",
                    EventName = "foundation.device.connected",
                    Payload = "{\"device_id\":\"proj-01\",\"type\":\"projector\"}",
                    LevelText = "正常",
                    LevelColor = new SolidColorBrush(Color.FromRgb(0x43, 0xB5, 0x81))
                },
                new RecentEvent
                {
                    Time = "10:23:02",
                    EventName = "foundation.process.exited",
                    Payload = "{\"pid\":1234,\"name\":\"teacher_tool.exe\"}",
                    LevelText = "警告",
                    LevelColor = new SolidColorBrush(Color.FromRgb(0xF5, 0xA6, 0x23))
                },
                new RecentEvent
                {
                    Time = "10:20:45",
                    EventName = "network.lan.changed",
                    Payload = "{\"online_count\":32}",
                    LevelText = "正常",
                    LevelColor = new SolidColorBrush(Color.FromRgb(0x43, 0xB5, 0x81))
                },
                new RecentEvent
                {
                    Time = "10:18:30",
                    EventName = "foundation.device.connected",
                    Payload = "{\"device_id\":\"pc-lab-07\",\"type\":\"computer\"}",
                    LevelText = "正常",
                    LevelColor = new SolidColorBrush(Color.FromRgb(0x43, 0xB5, 0x81))
                },
                new RecentEvent
                {
                    Time = "10:15:12",
                    EventName = "plugin.load.error",
                    Payload = "{\"plugin\":\"课时计时\",\"reason\":\"listener not found\"}",
                    LevelText = "错误",
                    LevelColor = new SolidColorBrush(Color.FromRgb(0xE5, 0x48, 0x4D))
                }
            };

            // 已注册事件
            RegisteredEvents = new ObservableCollection<RegisteredEvent>
            {
                new RegisteredEvent { EventName = "foundation.device.connected", Provider = "设备监控", ListenerCount = 3 },
                new RegisteredEvent { EventName = "foundation.device.disconnected", Provider = "设备监控", ListenerCount = 2 },
                new RegisteredEvent { EventName = "foundation.process.exited", Provider = "进程监视", ListenerCount = 1 },
                new RegisteredEvent { EventName = "network.lan.changed", Provider = "网络扫描", ListenerCount = 4 },
                new RegisteredEvent { EventName = "sys.cpu.high", Provider = "系统信息", ListenerCount = 2 },
                new RegisteredEvent { EventName = "sys.memory.high", Provider = "系统信息", ListenerCount = 1 },
                new RegisteredEvent { EventName = "file.changed", Provider = "文件监听", ListenerCount = 0 },
                new RegisteredEvent { EventName = "plugin.load.error", Provider = "PluginManager", ListenerCount = 2 }
            };
        }

        // ===== 叠加层控制 =====

        private void OpenPublishOverlay_Click(object sender, RoutedEventArgs e)
        {
            EventNameComboBox.SelectedIndex = -1;
            PayloadTextBox.Text = string.Empty;
            MainContent.Visibility = Visibility.Collapsed;
            PublishOverlay.Visibility = Visibility.Visible;
        }

        private void ClosePublishOverlay_Click(object sender, RoutedEventArgs e)
        {
            PublishOverlay.Visibility = Visibility.Collapsed;
            MainContent.Visibility = Visibility.Visible;
        }

        private void OpenRegisteredOverlay_Click(object sender, RoutedEventArgs e)
        {
            MainContent.Visibility = Visibility.Collapsed;
            RegisteredOverlay.Visibility = Visibility.Visible;
        }

        private void CloseRegisteredOverlay_Click(object sender, RoutedEventArgs e)
        {
            RegisteredOverlay.Visibility = Visibility.Collapsed;
            MainContent.Visibility = Visibility.Visible;
        }

        // ===== 事件发布逻辑 =====

        private void EventNameComboBox_SelectionChanged(object sender, SelectionChangedEventArgs e)
        {
            if (EventNameComboBox.SelectedItem is EventOption option)
            {
                PayloadTextBox.Text = option.DefaultPayload;
            }
        }

        private void PublishEvent_Click(object sender, RoutedEventArgs e)
        {
            if (EventNameComboBox.SelectedItem is not EventOption option)
            {
                MessageBox.Show("请先选择要发布的事件名称", "提示", MessageBoxButton.OK, MessageBoxImage.Information);
                return;
            }

            var payload = string.IsNullOrWhiteSpace(PayloadTextBox.Text) ? "{}" : PayloadTextBox.Text.Trim();

            // 模拟发布：将新事件插入历史事件列表头部
            RecentEvents.Insert(0, new RecentEvent
            {
                Time = DateTime.Now.ToString("HH:mm:ss"),
                EventName = option.Name,
                Payload = payload,
                LevelText = "正常",
                LevelColor = new SolidColorBrush(Color.FromRgb(0x43, 0xB5, 0x81))
            });

            PublishOverlay.Visibility = Visibility.Collapsed;
            MainContent.Visibility = Visibility.Visible;
        }

        private void RepublishEvent_Click(object sender, RoutedEventArgs e)
        {
            if (sender is Button btn && btn.Tag is RecentEvent evt)
            {
                // 模拟重发：将该事件再次插入历史事件列表头部
                RecentEvents.Insert(0, new RecentEvent
                {
                    Time = DateTime.Now.ToString("HH:mm:ss"),
                    EventName = evt.EventName,
                    Payload = evt.Payload,
                    LevelText = "正常",
                    LevelColor = new SolidColorBrush(Color.FromRgb(0x43, 0xB5, 0x81))
                });
            }
        }
    }

    /// <summary>
    /// 可发布事件选项
    /// </summary>
    public class EventOption
    {
        public string Name { get; set; } = string.Empty;
        public string DefaultPayload { get; set; } = "{}";
    }

    /// <summary>
    /// 历史事件记录
    /// </summary>
    public class RecentEvent
    {
        public string Time { get; set; } = string.Empty;
        public string EventName { get; set; } = string.Empty;
        public string Payload { get; set; } = string.Empty;
        public string LevelText { get; set; } = string.Empty;
        public Brush LevelColor { get; set; } = Brushes.Gray;
    }

    /// <summary>
    /// 已注册事件
    /// </summary>
    public class RegisteredEvent
    {
        public string EventName { get; set; } = string.Empty;
        public string Provider { get; set; } = string.Empty;
        public int ListenerCount { get; set; }
    }
}
