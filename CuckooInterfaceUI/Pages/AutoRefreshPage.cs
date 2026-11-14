// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
using System;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;
using System.Windows.Threading;
using CuckooInterfaceUI.Services;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// 页面基类：集中提供后端访问、宿主窗口、连接状态监听与统一错误反馈，
    /// 避免各页面重复实现同一套样板代码。
    /// </summary>
    public abstract class BackendPage : Page
    {
        private Func<Task>? _reloadOnConnected;

        protected BackendPage()
        {
            // 页面卸载后不再接收连接状态变化，避免后台页面重复刷新。
            Unloaded += (_, _) => Backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
        }

        protected static CoreBackend Backend => CoreBackend.Instance;

        protected MainWindow? HostWindow => Window.GetWindow(this) as MainWindow;

        protected Brush PrimaryBrush =>
            FindResource("TextFillColorPrimaryBrush") as Brush ?? Brushes.Gray;

        protected Brush SecondaryBrush =>
            FindResource("TextFillColorSecondaryBrush") as Brush ?? Brushes.Gray;

        protected Brush ControlBrush =>
            FindResource("ControlFillColorDefaultBrush") as Brush ?? Brushes.Transparent;

        /// <summary>
        /// 监听 Core 连接状态：恢复连接时执行 <paramref name="reload"/>，断开时进入断开态。
        /// </summary>
        protected void WatchConnection(Func<Task> reload)
        {
            _reloadOnConnected = reload;
            Backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
            Backend.ConnectionStateChanged += Backend_ConnectionStateChanged;
        }

        private void Backend_ConnectionStateChanged(object? sender, BackendConnectionStateChangedEventArgs e)
        {
            if (!e.IsCore || !IsVisible || _reloadOnConnected == null)
                return;

            _ = Dispatcher.InvokeAsync(async () =>
            {
                if (e.IsConnected)
                    await _reloadOnConnected();
                else
                    SetDisconnectedState();
            });
        }

        /// <summary>后端断开时由派生页面切换到离线展示。</summary>
        protected abstract void SetDisconnectedState();

        protected void NavigateTo(Type pageType) => HostWindow?.RootNavigation.Navigate(pageType);

        /// <summary>统一的错误反馈：连接异常进入全局未连接提示，其余错误弹出 Toast。</summary>
        protected Task ShowBackendErrorAsync(string title, Exception ex)
        {
            if (ex is BackendConnectionException)
                HostWindow?.ShowBackendUnavailable();
            else
                HostWindow?.ShowToast(title, ex.Message, true);

            return Task.CompletedTask;
        }

        /// <summary>CSV 字段转义。</summary>
        protected static string EscapeCsv(string value) =>
            value.IndexOfAny(new[] { ',', '"', '\r', '\n' }) < 0
                ? value
                : $"\"{value.Replace("\"", "\"\"")}\"";
    }

    /// <summary>
    /// 支持数据自动刷新的页面基类：页面可见时每隔 <see cref="RefreshInterval"/> 触发一次
    /// <see cref="OnAutoRefreshAsync"/>。
    /// <para>
    /// 刷新在 UI 线程触发，带防重入保护（上一次刷新未完成时跳过本次）；
    /// 页面 Unloaded 后自动停止，重新 Loaded 时自动恢复。
    /// </para>
    /// </summary>
    public abstract class AutoRefreshPage : BackendPage
    {
        /// <summary>自动刷新间隔。</summary>
        public static readonly TimeSpan RefreshInterval = TimeSpan.FromSeconds(1);

        private readonly DispatcherTimer _refreshTimer;
        private bool _isRefreshing;

        protected AutoRefreshPage()
        {
            _refreshTimer = new DispatcherTimer { Interval = RefreshInterval };
            _refreshTimer.Tick += async (_, _) => await TickAsync();

            Loaded += (_, _) => _refreshTimer.Start();
            Unloaded += (_, _) => _refreshTimer.Stop();
        }

        private async Task TickAsync()
        {
            if (_isRefreshing || !IsVisible)
                return;

            _isRefreshing = true;
            try
            {
                await OnAutoRefreshAsync();
            }
            catch
            {
                // Tick 是 async void，异常逃逸会拖垮进程；
                // 常规错误应由各页面的加载方法内部处理，这里只做兜底。
            }
            finally
            {
                _isRefreshing = false;
            }
        }

        /// <summary>
        /// 自动刷新回调：页面可见时按 <see cref="RefreshInterval"/> 周期性调用。
        /// </summary>
        protected abstract Task OnAutoRefreshAsync();
    }
}
