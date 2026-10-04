using System;
using System.Linq;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;
using CuckooInterfaceUI.Services;

namespace CuckooInterfaceUI.Pages
{
    public partial class HomePage : AutoRefreshPage
    {
        private readonly CoreBackend _backend = CoreBackend.Instance;
        private bool _hasOverview;

        public HomePage()
        {
            InitializeComponent();
            Loaded += HomePage_Loaded;
            Unloaded += (_, _) => _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
        }

        // 每 2 秒自动刷新概览数据（静默模式：不闪按钮、不弹错误提示）
        protected override Task OnAutoRefreshAsync() => LoadOverviewFromBackendAsync(interactive: false);

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

        /// <summary>
        /// 加载概览数据。
        /// interactive=true：用户点击或连接恢复，反馈完整（按钮禁用、加载态、错误提示）；
        /// interactive=false：2s 自动刷新的静默模式，不打扰界面。
        /// </summary>
        private async Task LoadOverviewFromBackendAsync(bool interactive = true)
        {
            if (!_backend.IsCoreConnected)
            {
                SetDisconnectedState();
                return;
            }

            try
            {
                if (interactive)
                {
                    RefreshHomeButton.IsEnabled = false;
                    SetLoadingStateIfNeeded();
                }

                var overviewTask = _backend.GetOverviewAsync();
                var pluginsTask = _backend.GetPluginsAsync();
                var eventsTask = _backend.GetRecentEventsAsync();
                await Task.WhenAll(overviewTask, pluginsTask, eventsTask);

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

                RecentEventsList.ItemsSource = recentEvents.Take(5).ToList();
                UpdateRecentEventsEmptyState(false);
                _hasOverview = true;
            }
            catch (Exception ex)
            {
                if (ex is BackendConnectionException)
                    SetDisconnectedState();
                if (interactive)
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
            RecentEventsList.ItemsSource = null;
            UpdateRecentEventsEmptyState(true);
            // Core 离线时按 Daemon 实际连接状态恢复操作按钮，
            // 保证「启动 Core」在 Core 停止后仍然可用
            SetActionButtonsEnabled(_backend.IsDaemonConnected);
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
            // 启动仅在 Core 离线且 Daemon 在线时可用；重启/停止只需 Daemon 在线
            StartServiceButton.IsEnabled = daemonConnected && !_backend.IsCoreConnected;
            RestartServiceButton.IsEnabled = daemonConnected;
            StopServiceButton.IsEnabled = daemonConnected;
        }

        private async void RefreshHome_Click(object sender, RoutedEventArgs e) => await LoadOverviewFromBackendAsync();

        private void ViewAllEvents_Click(object sender, RoutedEventArgs e) => NavigateTo(typeof(EventsPage));

        private async void StartService_Click(object sender, RoutedEventArgs e)
        {
            if (!_backend.IsDaemonConnected)
            {
                (Window.GetWindow(this) as MainWindow)?.ShowBackendUnavailable();
                return;
            }

            try
            {
                StartServiceButton.IsEnabled = false;
                await _backend.StartCoreAsync();
                (Window.GetWindow(this) as MainWindow)?.ShowToast("正在启动", "Core 启动后会自动刷新。");
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("启动 Core 失败", ex);
            }
            finally
            {
                SetActionButtonsEnabled(_backend.IsDaemonConnected);
            }
        }

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
