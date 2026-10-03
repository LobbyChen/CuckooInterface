using System;
using System.Linq;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;

namespace CuckooInterfaceUI.Pages
{
    public partial class HomePage : Page
    {
        private readonly CoreBackend _backend = CoreBackend.Instance;
        private bool _hasOverview;

        public HomePage()
        {
            InitializeComponent();
            Loaded += HomePage_Loaded;
            Unloaded += (_, _) => _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
        }

        private async void HomePage_Loaded(object sender, RoutedEventArgs e)
        {
            _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
            _backend.ConnectionStateChanged += Backend_ConnectionStateChanged;
            await LoadOverviewFromBackendAsync();
        }

        private void Backend_ConnectionStateChanged(object? sender, BackendConnectionStateChangedEventArgs e)
        {
            if (!e.IsCore || !IsVisible) return;
            _ = Dispatcher.InvokeAsync(async () =>
            {
                if (e.IsConnected)
                    await LoadOverviewFromBackendAsync();
                else
                    SetDisconnectedState();
            });
        }

        private async System.Threading.Tasks.Task LoadOverviewFromBackendAsync()
        {
            if (!_backend.IsCoreConnected)
            {
                SetDisconnectedState();
                return;
            }

            try
            {
                RefreshHomeButton.IsEnabled = false;
                SetLoadingStateIfNeeded();

                var overviewTask = _backend.GetOverviewAsync();
                var pluginsTask = _backend.GetPluginsAsync();
                var eventsTask = _backend.GetRecentEventsAsync();
                await System.Threading.Tasks.Task.WhenAll(overviewTask, pluginsTask, eventsTask);

                var overview = await overviewTask;
                var plugins = await pluginsTask;
                var recentEvents = await eventsTask;

                RunStatusText.Text = "运行中";
                RunStatusDetailText.Text = "Core 已连接，数据读取正常。";
                RunStatusDot.Fill = FindResource("CuckooStatusSuccessBrush") as SolidColorBrush;

                PluginCountText.Text = plugins.Count.ToString();
                KernelCount.Text = overview.TotalKernels.ToString();
                EventCountText.Text = overview.EventCountToday.ToString();
                ErrorCountText.Text = overview.ErrorCountToday.ToString();
                UptimeText.Text = string.IsNullOrWhiteSpace(overview.Uptime) ? "—" : overview.Uptime;

                var kernelNames = plugins.Where(p => p.Type == PluginType.Kernel).Select(p => p.Name).ToList();
                KernelNames.Text = kernelNames.Count == 0 ? "无" : string.Join(" · ", kernelNames);

                RecentEventsList.ItemsSource = recentEvents.Take(5).ToList();
                UpdateRecentEventsEmptyState(false);
                _hasOverview = true;
            }
            catch (Exception ex)
            {
                if (ex is BackendConnectionException)
                    SetDisconnectedState();
                await ShowBackendErrorAsync("加载概览失败", ex);
            }
            finally
            {
                RefreshHomeButton.IsEnabled = _backend.IsCoreConnected;
                SetActionButtonsEnabled(_backend.IsDaemonConnected);
            }
        }

        private void SetLoadingStateIfNeeded()
        {
            if (_hasOverview)
            {
                RunStatusDetailText.Text = "正在刷新…";
                return;
            }

            RunStatusText.Text = "加载中…";
            RunStatusDetailText.Text = "正在读取 Core 状态";
            RunStatusDot.Fill = FindResource("CuckooStatusWarningBrush") as SolidColorBrush;
            PluginCountText.Text = "…";
            KernelCount.Text = "…";
            EventCountText.Text = "…";
            ErrorCountText.Text = "…";
            UptimeText.Text = "…";
        }

        private void SetDisconnectedState()
        {
            _hasOverview = false;
            RunStatusText.Text = "未连接";
            RunStatusDetailText.Text = "等待 Core 连接，恢复后自动刷新。";
            RunStatusDot.Fill = FindResource("CuckooStatusErrorBrush") as SolidColorBrush;
            PluginCountText.Text = "—";
            KernelCount.Text = "—";
            EventCountText.Text = "—";
            ErrorCountText.Text = "—";
            UptimeText.Text = "—";
            KernelNames.Text = "后端未连接";
            RecentEventsList.ItemsSource = null;
            UpdateRecentEventsEmptyState(true);
            SetActionButtonsEnabled(false);
        }        private void UpdateRecentEventsEmptyState(bool disconnected)
        {
            var hasItems = RecentEventsList.Items.Count > 0;
            RecentEventsEmptyState.Visibility = hasItems ? Visibility.Collapsed : Visibility.Visible;
            RecentEventsEmptyTitle.Text = disconnected ? "Core 未连接" : "暂无事件";
            RecentEventsEmptyDetail.Text = disconnected
                ? "恢复连接后会自动刷新。"
                : "当前没有可显示的事件。";
        }

        private void SetActionButtonsEnabled(bool daemonConnected)
        {
            RestartServiceButton.IsEnabled = daemonConnected;
            StopServiceButton.IsEnabled = daemonConnected;
        }

        private async void RefreshHome_Click(object sender, RoutedEventArgs e) => await LoadOverviewFromBackendAsync();

        private void ViewAllEvents_Click(object sender, RoutedEventArgs e) => NavigateTo(typeof(EventsPage));

        private async void RestartService_Click(object sender, RoutedEventArgs e)
        {
            if (!_backend.IsDaemonConnected)
            {
                (Window.GetWindow(this) as MainWindow)?.ShowBackendUnavailable();
                return;
            }

            if (Window.GetWindow(this) is not MainWindow window) return;
            var confirmed = await window.ShowConfirmDialogAsync("确认重启", "确定要重启 Core 吗？", "重启");
            if (!confirmed) return;

            try
            {
                RestartServiceButton.IsEnabled = false;
                StopServiceButton.IsEnabled = false;
                await _backend.RestartCoreAsync();
                (Window.GetWindow(this) as MainWindow)?.ShowToast("正在重启", "Core 恢复后页面会自动刷新。");
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("重启 Core 失败", ex);
            }
            finally
            {
                SetActionButtonsEnabled(_backend.IsDaemonConnected);
            }
        }

        private async void StopService_Click(object sender, RoutedEventArgs e)
        {
            if (!_backend.IsDaemonConnected)
            {
                (Window.GetWindow(this) as MainWindow)?.ShowBackendUnavailable();
                return;
            }

            if (Window.GetWindow(this) is not MainWindow window) return;
            var confirmed = await window.ShowConfirmDialogAsync("确认停止", "确定要停止 Core 吗？所有运行中的插件都会停止。", "停止", danger: true);
            if (!confirmed) return;

            try
            {
                RestartServiceButton.IsEnabled = false;
                StopServiceButton.IsEnabled = false;
                await _backend.StopCoreAsync();
                SetDisconnectedState();
                (Window.GetWindow(this) as MainWindow)?.ShowToast("Core 已停止", "需要时可通过服务端重新启动。");
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("停止 Core 失败", ex);
            }
            finally
            {
                SetActionButtonsEnabled(_backend.IsDaemonConnected);
            }
        }

        private System.Threading.Tasks.Task ShowBackendErrorAsync(string title, Exception ex)
        {
            if (ex is BackendConnectionException)
            {
                (Window.GetWindow(this) as MainWindow)?.ShowBackendUnavailable();
                return System.Threading.Tasks.Task.CompletedTask;
            }

            (Window.GetWindow(this) as MainWindow)?.ShowToast(title, ex.Message, true);
            return System.Threading.Tasks.Task.CompletedTask;
        }

        private void NavigateTo(Type pageType)
        {
            if (Window.GetWindow(this) is MainWindow window)
                window.RootNavigation.Navigate(pageType);
        }
    }
}
