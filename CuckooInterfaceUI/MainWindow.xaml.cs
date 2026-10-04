using System;
using System.Threading;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Threading;
using Wpf.Ui;
using Wpf.Ui.Controls;
using Wpf.Ui.Extensions;
using CuckooInterfaceUI.Services;

namespace CuckooInterfaceUI
{
    /// <summary>
    /// 主窗口只负责导航、全局连接状态和 WPF-UI 原生反馈控件。
    /// </summary>
    public partial class MainWindow : FluentWindow
    {
        private readonly CoreBackend _backend = CoreBackend.Instance;
        private readonly DispatcherTimer _connectionTimer;
        private readonly SemaphoreSlim _connectionCheckGate = new(1, 1);
        private readonly SnackbarService _snackbarService = new();
        private bool _isCheckingConnection;
        private bool _hasConnectionSnapshot;
        private bool _lastCoreConnected;
        private bool _lastDaemonConnected;

        public MainWindow()
        {
            InitializeComponent();
            _snackbarService.SetSnackbarPresenter(SnackbarPresenter);

            _backend.ConnectionStateChanged += Backend_ConnectionStateChanged;
            _connectionTimer = new DispatcherTimer { Interval = TimeSpan.FromSeconds(4) };
            _connectionTimer.Tick += async (_, _) => await RefreshConnectionStateAsync();

            Loaded += MainWindow_Loaded;
            Closed += MainWindow_Closed;
        }

        private async void MainWindow_Loaded(object sender, RoutedEventArgs e)
        {
            // 启动时先按系统主题渲染，待后端连接后再应用保存值。
            ThemeManager.ApplyTheme("system");
            RootNavigation.Navigate(typeof(Pages.HomePage));
            UpdateConnectionUi();
            _connectionTimer.Start();
            await RefreshConnectionStateAsync();
        }

        private void MainWindow_Closed(object? sender, EventArgs e)
        {
            _connectionTimer.Stop();
            _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
            _connectionCheckGate.Dispose();
        }

        private void Backend_ConnectionStateChanged(object? sender, BackendConnectionStateChangedEventArgs e)
        {
            if (!Dispatcher.CheckAccess())
            {
                _ = Dispatcher.InvokeAsync(UpdateConnectionUi);
                return;
            }

            UpdateConnectionUi();
        }

        private async Task RefreshConnectionStateAsync()
        {
            if (_isCheckingConnection || !await _connectionCheckGate.WaitAsync(0))
                return;

            try
            {
                _isCheckingConnection = true;
                ReconnectButton.IsEnabled = false;
                ReconnectButton.Content = "检查中…";
                await _backend.CheckConnectionAsync();
            }
            finally
            {
                _isCheckingConnection = false;
                ReconnectButton.IsEnabled = true;
                ReconnectButton.Content = "重连";
                UpdateConnectionUi();
                _connectionCheckGate.Release();
            }
        }

        private async void ReconnectButton_Click(object sender, RoutedEventArgs e)
        {
            ShowToast("正在重连", "正在检查 Core 和 Daemon…");
            await RefreshConnectionStateAsync();
        }

        public void ShowBackendUnavailable(string? detail = null)
        {
            UpdateConnectionUi();
            if (!string.IsNullOrWhiteSpace(detail))
            {
                GlobalConnectionInfoBar.Message = detail;
                GlobalConnectionInfoBar.IsOpen = true;
            }
        }

        private void UpdateConnectionUi()
        {
            var coreConnected = _backend.IsCoreConnected;
            var daemonConnected = _backend.IsDaemonConnected;
            var fullyConnected = coreConnected && daemonConnected;

            if (fullyConnected)
            {
                GlobalConnectionInfoBar.IsOpen = false;
                ReconnectButton.Visibility = Visibility.Collapsed;

                if (_hasConnectionSnapshot && (!_lastCoreConnected || !_lastDaemonConnected))
                    ShowToast("连接已恢复", "Core 和 Daemon 已重新连接。");

                _ = ApplySavedAppearanceAsync();
            }
            else
            {
                GlobalConnectionInfoBar.IsOpen = true;
                ReconnectButton.Visibility = Visibility.Visible;
                GlobalConnectionInfoBar.Severity = !coreConnected && !daemonConnected
                    ? InfoBarSeverity.Error
                    : InfoBarSeverity.Warning;

                if (!coreConnected && !daemonConnected)
                {
                    GlobalConnectionInfoBar.Title = "后端未连接";
                    GlobalConnectionInfoBar.Message = "Core、Daemon 未连接，正在自动重试。";
                }
                else if (!coreConnected)
                {
                    GlobalConnectionInfoBar.Title = "Core 未连接";
                    GlobalConnectionInfoBar.Message = "部分页面暂不可用，恢复连接后会自动刷新。";
                }
                else
                {
                    GlobalConnectionInfoBar.Title = "Daemon 未连接";
                    GlobalConnectionInfoBar.Message = "服务控制暂不可用，恢复连接后会自动重试。";
                }
            }

            _hasConnectionSnapshot = true;
            _lastCoreConnected = coreConnected;
            _lastDaemonConnected = daemonConnected;
        }

        /// <summary>
        /// 从后端读取已保存的外观设置并应用主题与强调色。
        /// 仅在 Core 首次连接或从断开恢复连接时执行，避免每 4 秒重复拉取。
        /// </summary>
        private async Task ApplySavedAppearanceAsync()
        {
            if (!_backend.IsCoreConnected) return;

            try
            {
                var values = await _backend.GetAppearanceSettingsAsync();
                if (values.TryGetValue("theme", out var themeObj))
                    ThemeManager.ApplyTheme(themeObj?.ToString() ?? "system");
                if (values.TryGetValue("accentColor", out var accentObj))
                    ThemeManager.ApplyAccent(accentObj?.ToString() ?? string.Empty);
            }
            catch
            {
                // 拉取失败时忽略，保持当前主题。
            }
        }

        public async Task<bool> ShowConfirmDialogAsync(string title, object content, string primaryText, string closeText = "取消", bool danger = false)
        {
            var appearance = danger ? ControlAppearance.Danger : ControlAppearance.Primary;
            var dialog = new ContentDialog(RootContentDialogHost)
            {
                Title = title,
                Content = content,
                PrimaryButtonText = primaryText,
                CloseButtonText = closeText,
                DefaultButton = ContentDialogButton.Close,
                DialogWidth = 460,
                DialogMaxWidth = 520,
                DialogMaxHeight = 380,
                PrimaryButtonAppearance = appearance
            };

            var result = await dialog.ShowAsync();
            return result == ContentDialogResult.Primary;
        }

        public void ShowToast(string title, string message, bool isError = false)
        {
            var appearance = isError ? ControlAppearance.Danger : ControlAppearance.Success;
            _snackbarService.Show(title, message, appearance, TimeSpan.FromSeconds(3.2));
        }
    }
}
