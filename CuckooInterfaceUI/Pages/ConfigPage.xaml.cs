using System.Collections.ObjectModel;
using System.Windows.Controls;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// ConfigPage.xaml 的交互逻辑
    /// </summary>
    public partial class ConfigPage : Page
    {
        public ObservableCollection<ConfigPluginItem> Plugins { get; set; }

        public ConfigPage()
        {
            InitializeComponent();
            LoadHardcodedPlugins();
            PluginListBox.ItemsSource = Plugins;
        }

        /// <summary>
        /// 加载硬编码的插件列表（模拟后端 Go 核心返回的插件元数据）
        /// </summary>
        private void LoadHardcodedPlugins()
        {
            Plugins = new ObservableCollection<ConfigPluginItem>
            {
                new ConfigPluginItem { Name = "Lua Runtime", TypeText = "内核", Description = "提供 Lua 5.4 脚本运行时" },
                new ConfigPluginItem { Name = "Python Runtime", TypeText = "内核", Description = "提供 Python 3.11 运行时" },
                new ConfigPluginItem { Name = "设备监控", TypeText = "基础", Description = "监听电教设备接入与断开事件" },
                new ConfigPluginItem { Name = "网络扫描", TypeText = "基础", Description = "扫描局域网设备并发布网络变化事件" },
                new ConfigPluginItem { Name = "进程监视", TypeText = "基础", Description = "监视指定进程的启动与退出" },
                new ConfigPluginItem { Name = "系统信息", TypeText = "基础", Description = "采集系统运行信息并定时上报" },
                new ConfigPluginItem { Name = "文件监听", TypeText = "基础", Description = "监控指定目录的文件变更事件" },
                new ConfigPluginItem { Name = "自动开机", TypeText = "应用", Description = "根据课表自动开机并启动授课软件" },
                new ConfigPluginItem { Name = "锁屏控制", TypeText = "应用", Description = "远程锁屏与解锁，支持批量操作" },
                new ConfigPluginItem { Name = "课时计时", TypeText = "应用", Description = "上下课计时与提醒" }
            };
        }

        private void PluginListBox_SelectionChanged(object sender, SelectionChangedEventArgs e)
        {
            if (PluginListBox.SelectedItem is ConfigPluginItem item)
            {
                SelectedPluginName.Text = item.Name;
                SelectedPluginDesc.Text = item.Description;
            }
        }
    }

    /// <summary>
    /// 配置页插件数据模型
    /// </summary>
    public class ConfigPluginItem
    {
        public string Name { get; set; } = string.Empty;
        public string TypeText { get; set; } = string.Empty;
        public string Description { get; set; } = string.Empty;
    }
}
