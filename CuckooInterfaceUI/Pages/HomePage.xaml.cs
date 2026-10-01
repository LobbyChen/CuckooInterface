using System;
using System.Linq;
using System.Windows;
using System.Windows.Controls;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// HomePage.xaml 的交互逻辑
    /// </summary>
    public partial class HomePage : Page
    {
        private readonly MockBackend _backend = MockBackend.Instance;

        public HomePage()
        {
            InitializeComponent();
            LoadOverviewFromBackend();
        }

        private void LoadOverviewFromBackend()
        {
            var overview = _backend.GetOverview();
            var plugins = _backend.GetPlugins();

            // 状态卡片
            KernelCount.Text = overview.TotalKernels.ToString();
            var kernelNames = plugins.Where(p => p.Type == PluginType.Kernel).Select(p => p.Name).ToList();
            KernelNames.Text = kernelNames.Count > 0 ? string.Join(" · ", kernelNames) : "无";

            BasePluginCount.Text = plugins.Count(p => p.Type == PluginType.Base).ToString();
            var baseActive = plugins.Count(p => p.Type == PluginType.Base && p.IsEnabled);
            BasePluginStatus.Text = $"{baseActive} 在线";

            ActivePluginCount.Text = plugins.Count(p => p.Type == PluginType.Active).ToString();
            var activeRunning = plugins.Count(p => p.Type == PluginType.Active && p.IsEnabled);
            ActivePluginStatus.Text = $"{activeRunning} 运行中";

            // 最近事件（取前 5 条）
            RecentEventsList.ItemsSource = _backend.GetRecentEvents().Take(5).ToList();

            // 系统信息
            VersionText.Text = "v0.1.0-alpha";
            UptimeText.Text = overview.Uptime;
            ModeText.Text = "Normal";
            IpcPipeText.Text = "\\\\.\\pipe\\CuckooInterfaceBridge";
        }

        // ===== 跳转到事件页面 =====

        private void ViewAllEvents_Click(object sender, RoutedEventArgs e)
        {
            NavigateTo(typeof(EventsPage));
        }

        // ===== 快捷操作 =====

        private void ReloadPlugins_Click(object sender, RoutedEventArgs e)
        {
            NavigateTo(typeof(PluginsPage));
        }

        private void OpenLogs_Click(object sender, RoutedEventArgs e)
        {
            NavigateTo(typeof(LogsPage));
        }

        private async void RestartService_Click(object sender, RoutedEventArgs e)
        {
            var result = await new Wpf.Ui.Controls.MessageBox
            {
                Title = "确认重启",
                Content = "确定要重启 CuckooInterface 服务吗？",
                PrimaryButtonText = "是",
                SecondaryButtonText = "否"
            }.ShowDialogAsync();

            if (result == Wpf.Ui.Controls.MessageBoxResult.Primary)
            {
                _backend.AddLog(Models.LogLevel.Info, "服务重启中...", "core");
                await new Wpf.Ui.Controls.MessageBox
                {
                    Title = "重启",
                    Content = "服务已发送重启指令。",
                    PrimaryButtonText = "确定"
                }.ShowDialogAsync();
            }
        }

        private async void StopService_Click(object sender, RoutedEventArgs e)
        {
            var result = await new Wpf.Ui.Controls.MessageBox
            {
                Title = "确认停止",
                Content = "确定要停止 CuckooInterface 服务吗？此操作将终止所有运行中的插件。",
                PrimaryButtonText = "是",
                SecondaryButtonText = "否"
            }.ShowDialogAsync();

            if (result == Wpf.Ui.Controls.MessageBoxResult.Primary)
            {
                _backend.AddLog(Models.LogLevel.Warn, "服务已停止", "core");
                await new Wpf.Ui.Controls.MessageBox
                {
                    Title = "已停止",
                    Content = "服务已停止。",
                    PrimaryButtonText = "确定"
                }.ShowDialogAsync();
            }
        }

        // ===== 导航辅助 =====

        private void NavigateTo(Type pageType)
        {
            if (Window.GetWindow(this) is MainWindow window)
            {
                window.RootNavigation.Navigate(pageType);
            }
        }
    }
}

