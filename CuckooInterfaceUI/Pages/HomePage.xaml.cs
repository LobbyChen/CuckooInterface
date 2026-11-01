// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
using System;
using System.Collections.Generic;
using System.Linq;
using System.Text.Json;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;
using ToggleSwitch = Wpf.Ui.Controls.ToggleSwitch;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// 概览页：面向使用者的插件入口。
    /// <para>
    /// 只展示用户可见的插件（Active / Package），Kernel 运行时与 Base 事件源属于平台内部能力，不在首页出现。
    /// 最近动态也做了过滤与翻译，避免把 time.minute.tick 这类内部事件直接暴露给使用者。
    /// </para>
    /// </summary>
    public partial class HomePage : AutoRefreshPage
    {
        private readonly CoreBackend _backend = CoreBackend.Instance;

        private bool _hasOverview;
        private bool _isLoading;
        private string _featureSignature = string.Empty;
        private string _activitySignature = string.Empty;

        public HomePage()
        {
            InitializeComponent();
            Loaded += HomePage_Loaded;
            Unloaded += (_, _) => _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
        }

        // 定时刷新首页数据（静默模式：不闪按钮、不弹错误提示）
        protected override Task OnAutoRefreshAsync() => LoadHomeFromBackendAsync(interactive: false);

        private async void HomePage_Loaded(object sender, RoutedEventArgs e)
        {
            _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
            _backend.ConnectionStateChanged += Backend_ConnectionStateChanged;
            await LoadHomeFromBackendAsync();
        }

        private void Backend_ConnectionStateChanged(object? sender, BackendConnectionStateChangedEventArgs e)
        {
            if (!e.IsCore || !IsVisible) return;
            _ = Dispatcher.InvokeAsync(async () =>
            {
                if (e.IsConnected)
                    await LoadHomeFromBackendAsync();
                else
                    SetDisconnectedState();
            });
        }

        /// <summary>
        /// 加载首页数据。
        /// interactive=true：用户点击或连接恢复，给出加载态与错误提示；
        /// interactive=false：自动刷新的静默模式，不打扰界面。
        /// </summary>
        private async Task LoadHomeFromBackendAsync(bool interactive = true)
        {
            if (!_backend.IsCoreConnected)
            {
                SetDisconnectedState();
                return;
            }

            if (_isLoading)
                return;

            _isLoading = true;
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

                if (!IsLoaded)
                    return;

                var overview = await overviewTask;
                var plugins = await pluginsTask;
                var events = await eventsTask;

                ApplyServiceState();
                UptimeText.Text = string.IsNullOrWhiteSpace(overview.Uptime) ? "—" : overview.Uptime;
                ApplyFeatures(plugins);
                ApplyActivities(events);
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
                _isLoading = false;
                RefreshHomeButton.IsEnabled = _backend.IsCoreConnected;
                SetActionButtonsEnabled(_backend.IsDaemonConnected);
            }
        }

        /// <summary>把服务连接状态同步到页头状态胶囊和服务状态卡片。</summary>
        private void ApplyServiceState()
        {
            var daemonConnected = _backend.IsDaemonConnected;
            SetStatus(
                "运行中",
                FindResource("CuckooStatusSuccessBrush") as Brush,
                daemonConnected
                    ? "Core 与守护进程均已连接，插件在后台持续运行。"
                    : "Core 已连接，守护进程离线，部分服务操作不可用。");
        }

        private void SetStatus(string text, Brush? brush, string detail)
        {
            RunStatusText.Text = text;
            HeaderStatusText.Text = text;
            RunStatusDetailText.Text = detail;

            var fill = brush ?? FindResource("CuckooStatusNeutralBrush") as Brush ?? Brushes.Gray;
            HeaderStatusDot.Fill = fill;
            RunStatusDot.Fill = fill;
        }

        private void ApplyFeatures(List<PluginInfo> plugins)
        {
            var features = plugins
                .Where(p => p.Type != PluginType.Kernel && p.Type != PluginType.Base)
                .OrderByDescending(p => p.IsEnabled)
                .ThenBy(p => p.Name, StringComparer.CurrentCultureIgnoreCase)
                .ToList();

            FeatureSummaryText.Text = features.Count == 0
                ? "安装插件后，插件会显示在这里。"
                : $"共 {features.Count} 个插件，已启用 {features.Count(f => f.IsEnabled)} 个。";

            // 数据没变化时跳过重建，避免自动刷新打断开关操作与滚动位置。
            var signature = string.Join("|", features.Select(f =>
                $"{f.Id}:{f.Name}:{f.IsEnabled}:{f.IsLoaded}:{f.HasConfigPage}:{f.Description}"));
            if (signature != _featureSignature)
            {
                _featureSignature = signature;
                FeatureList.ItemsSource = features;
            }

            var hasItems = features.Count > 0;
            FeaturesEmptyState.Visibility = hasItems ? Visibility.Collapsed : Visibility.Visible;
            if (!hasItems)
            {
                FeaturesEmptyTitle.Text = "还没有可用插件";
                FeaturesEmptyDetail.Text = "安装插件后，插件会统一显示在这里。";
            }
        }

        private void ApplyActivities(List<EventRecord> events)
        {
            var filtered = events
                .OrderByDescending(e => e.Timestamp)
                .Where(IsHomeActivity)
                .Take(6)
                .ToList();

            var signature = string.Join("|", filtered.Select(e =>
                $"{e.Timestamp.Ticks}:{e.EventName}:{e.Payload}"));
            if (signature != _activitySignature)
            {
                _activitySignature = signature;
                ActivityList.ItemsSource = filtered.Select(ToActivityItem).ToList();
            }

            var hasItems = filtered.Count > 0;
            ActivityEmptyState.Visibility = hasItems ? Visibility.Collapsed : Visibility.Visible;
            if (!hasItems)
            {
                ActivityEmptyTitle.Text = "暂无动态";
                ActivityEmptyDetail.Text = "插件启动或系统事件会显示在这里。";
            }
        }

        /// <summary>
        /// 首页只保留对人友好的动态：过滤周期性 tick，以及 Kernel / Base 插件的装载事件。
        /// </summary>
        private static bool IsHomeActivity(EventRecord record)
        {
            if (record.EventName.EndsWith(".tick", StringComparison.OrdinalIgnoreCase))
                return false;

            if (record.EventName is "foundation.plugin.loaded" or "foundation.plugin.unloaded")
                return IsUserFacingPluginEvent(record.Payload);

            return true;
        }

        private static bool IsUserFacingPluginEvent(string payload)
        {
            try
            {
                using var doc = JsonDocument.Parse(payload);
                if (!doc.RootElement.TryGetProperty("plugin_type", out var type))
                    return false;

                var value = type.GetString();
                return value is "active" or "package";
            }
            catch
            {
                return false;
            }
        }

        private static HomeActivityItem ToActivityItem(EventRecord record)
        {
            var title = "系统事件";
            var detail = record.EventName;

            switch (record.EventName)
            {
                case "foundation.framework.started":
                    title = "服务已启动";
                    detail = "CuckooInterface 已开始运行。";
                    break;
                case "foundation.framework.stopping":
                    title = "服务正在停止";
                    detail = "正在安全退出。";
                    break;
                case "foundation.plugin.loaded":
                    title = "插件已启动";
                    detail = ReadPluginName(record.Payload);
                    break;
                case "foundation.plugin.unloaded":
                    title = "插件已停止";
                    detail = ReadPluginName(record.Payload);
                    break;
                case "system.disk.plugged":
                    title = "磁盘已接入";
                    detail = ReadDiskSymbols(record.Payload);
                    break;
                case "system.disk.unplug":
                    title = "磁盘已移除";
                    detail = ReadDiskSymbols(record.Payload);
                    break;
            }

            if (string.IsNullOrWhiteSpace(detail))
                detail = record.EventName;

            return new HomeActivityItem
            {
                Time = record.Timestamp.ToString("HH:mm"),
                Title = title,
                Detail = detail,
                LevelColor = record.LevelColor
            };
        }

        private static string ReadPluginName(string payload)
        {
            try
            {
                using var doc = JsonDocument.Parse(payload);
                return doc.RootElement.TryGetProperty("plugin_name", out var name)
                    ? name.GetString() ?? string.Empty
                    : string.Empty;
            }
            catch
            {
                return string.Empty;
            }
        }

        private static string ReadDiskSymbols(string payload)
        {
            try
            {
                using var doc = JsonDocument.Parse(payload);
                if (!doc.RootElement.TryGetProperty("changed_disks", out var disks) ||
                    disks.ValueKind != JsonValueKind.Array)
                {
                    return string.Empty;
                }

                var symbols = disks.EnumerateArray()
                    .Select(d => d.TryGetProperty("symbol", out var symbol) ? symbol.GetString() : null)
                    .Where(s => !string.IsNullOrWhiteSpace(s))
                    .ToList();

                return symbols.Count == 0 ? string.Empty : string.Join("、", symbols);
            }
            catch
            {
                return string.Empty;
            }
        }

        private void SetLoadingStateIfNeeded()
        {
            if (_hasOverview)
            {
                RunStatusDetailText.Text = "正在刷新…";
                return;
            }

            SetStatus("加载中", FindResource("CuckooStatusWarningBrush") as Brush, "正在读取服务状态。");
            UptimeText.Text = "…";
            FeatureSummaryText.Text = "正在读取已安装的插件…";
            FeaturesEmptyState.Visibility = Visibility.Visible;
            FeaturesEmptyTitle.Text = "正在加载";
            FeaturesEmptyDetail.Text = "正在读取已安装的插件。";
            ActivityEmptyState.Visibility = Visibility.Visible;
            ActivityEmptyTitle.Text = "正在加载";
            ActivityEmptyDetail.Text = "正在读取最近动态。";
        }

        private void SetDisconnectedState()
        {
            _hasOverview = false;
            _featureSignature = string.Empty;
            _activitySignature = string.Empty;

            SetStatus("未连接", FindResource("CuckooStatusErrorBrush") as Brush, "等待 Core 连接，恢复后自动刷新。");
            UptimeText.Text = "—";
            FeatureSummaryText.Text = "服务未连接。";

            FeatureList.ItemsSource = null;
            FeaturesEmptyState.Visibility = Visibility.Visible;
            FeaturesEmptyTitle.Text = "服务未连接";
            FeaturesEmptyDetail.Text = "连接恢复后会自动加载已安装的插件。";

            ActivityList.ItemsSource = null;
            ActivityEmptyState.Visibility = Visibility.Visible;
            ActivityEmptyTitle.Text = "服务未连接";
            ActivityEmptyDetail.Text = "连接恢复后会自动刷新。";

            SetActionButtonsEnabled(_backend.IsDaemonConnected);
        }

        private void SetActionButtonsEnabled(bool daemonConnected)
        {
            // 启动仅在 Core 离线且 Daemon 在线时可用；重启/停止只需 Daemon 在线
            StartServiceButton.IsEnabled = daemonConnected && !_backend.IsCoreConnected;
            RestartServiceButton.IsEnabled = daemonConnected;
            StopServiceButton.IsEnabled = daemonConnected;
        }

        private async void RefreshHome_Click(object sender, RoutedEventArgs e) =>
            await LoadHomeFromBackendAsync();

        private void ManageFeatures_Click(object sender, RoutedEventArgs e) =>
            NavigateTo(typeof(PluginsPage));

        private void ViewAllEvents_Click(object sender, RoutedEventArgs e) =>
            NavigateTo(typeof(EventsPage));

        private void FeatureConfig_Click(object sender, RoutedEventArgs e)
        {
            if (sender is not FrameworkElement element || element.Tag is not PluginInfo plugin)
                return;

            ConfigPage.PendingPluginId = plugin.Id;
            NavigateTo(typeof(ConfigPage));
        }

        private async void FeatureToggle_Click(object sender, RoutedEventArgs e)
        {
            if (sender is not ToggleSwitch toggle || toggle.Tag is not PluginInfo plugin)
                return;

            if (!_backend.IsCoreConnected)
            {
                toggle.IsChecked = plugin.IsEnabled;
                (Window.GetWindow(this) as MainWindow)?.ShowBackendUnavailable();
                return;
            }

            var targetEnabled = toggle.IsChecked == true;
            toggle.IsEnabled = false;

            try
            {
                var ok = await _backend.TogglePluginAsync(plugin.Id, targetEnabled);
                if (!ok)
                    throw new InvalidOperationException("Core 未确认插件状态变更。");

                await LoadHomeFromBackendAsync(interactive: false);

                (Window.GetWindow(this) as MainWindow)?.ShowToast(
                    targetEnabled ? "插件已启用" : "插件已禁用",
                    plugin.Name);
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("更新插件状态失败", ex);
                await LoadHomeFromBackendAsync(interactive: false);
            }
            finally
            {
                toggle.IsEnabled = _backend.IsCoreConnected;
            }
        }

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

        private Task ShowBackendErrorAsync(string title, Exception ex)
        {
            if (ex is BackendConnectionException)
            {
                (Window.GetWindow(this) as MainWindow)?.ShowBackendUnavailable();
                return Task.CompletedTask;
            }

            (Window.GetWindow(this) as MainWindow)?.ShowToast(title, ex.Message, true);
            return Task.CompletedTask;
        }

        private void NavigateTo(Type pageType)
        {
            if (Window.GetWindow(this) is MainWindow window)
                window.RootNavigation.Navigate(pageType);
        }

        /// <summary>首页“最近动态”的展示项，已把内部事件名翻译成面向人的描述。</summary>
        public sealed class HomeActivityItem
        {
            public string Time { get; init; } = string.Empty;

            public string Title { get; init; } = string.Empty;

            public string Detail { get; init; } = string.Empty;

            public Brush LevelColor { get; init; } = Brushes.Gray;
        }
    }
}
