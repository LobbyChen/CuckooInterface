// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Text.Json;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;
using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.Wpf;
using Wpf.Ui.Controls;
using TextBlock = System.Windows.Controls.TextBlock;
using WpfTextBox = Wpf.Ui.Controls.TextBox;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// ConfigPage.xaml 的交互逻辑：包含「插件配置」与「运行时」两个分区。
    /// <para>
    /// 插件配置页由 META-INF.json 的 config_page 指定 HTML。
    /// 每个插件独享一个 WebView2 实例，按插件 ID 缓存；只有在用户选中该插件时才创建并加载，
    /// 避免多个插件共用同一页面导致配置内容混用。
    /// </para>
    /// <para>
    /// 宿主与配置页的约定：
    /// 1. 页面加载完成后，宿主发送 <c>{"type":"cuckoo:init","config":"..."}</c>，
    ///    同时注入 <c>window.__cuckooConfig</c> 并派发 <c>cuckoo-config-ready</c> 事件；
    /// 2. 配置页调用 <c>window.chrome.webview.postMessage({type:"cuckoo:saveConfig", config:"..."})</c> 保存；
    /// 3. 宿主保存后回复 <c>{"type":"cuckoo:saved","ok":true|false,"error":"..."}</c>。
    /// </para>
    /// </summary>
    public partial class ConfigPage : AutoRefreshPage
    {
        /// <summary>按插件 ID 缓存的独立 WebView2 实例。</summary>
        private readonly Dictionary<string, ConfigView> _views = new(StringComparer.OrdinalIgnoreCase);

        private string _currentPluginId = string.Empty;
        private int _configLoadVersion;
        private bool _suppressSelectionChanged;

        private static readonly TimeSpan RuntimeDetailAutoRefreshInterval = TimeSpan.FromSeconds(3);

        private static readonly JsonSerializerOptions IndentedJsonOptions = new()
        {
            WriteIndented = true
        };

        private List<RuntimeInfo> _runtimes = new();
        private RuntimeInfo? _selectedRuntime;
        private DateTime _lastRuntimeRefresh = DateTime.MinValue;
        private int _runtimeDetailRequestId;
        private MainWindow? _hostWindow;

        /// <summary>从插件页跳转过来时需要自动选中的插件 ID。</summary>
        public static string? PendingPluginId { get; set; }

        /// <summary>从插件页跳转过来时需要自动选中的 Kernel ID。</summary>
        public static string? PendingKernelId { get; set; }

        /// <summary>从插件页跳转过来时是否直接展示运行时分区。</summary>
        public static bool PendingShowRuntimes { get; set; }

        public ConfigPage()
        {
            InitializeComponent();
            Loaded += ConfigPage_Loaded;
            Unloaded += ConfigPage_Unloaded;
        }

        // 只刷新当前可见分区的数据。
        protected override async Task OnAutoRefreshAsync()
        {
            if (PluginsSection.Visibility == Visibility.Visible)
            {
                await LoadPluginsFromBackendAsync(interactive: false);
                return;
            }

            if (DateTime.UtcNow - _lastRuntimeRefresh >= RuntimeDetailAutoRefreshInterval)
                await LoadRuntimesAsync(silent: true);
        }

        private async void ConfigPage_Loaded(object sender, RoutedEventArgs e)
        {
            if (PendingShowRuntimes)
            {
                PendingShowRuntimes = false;
                ShowSection(runtimes: true);
            }
            else if (PendingPluginId != null)
            {
                // 指定了插件时确保停留在插件分区。
                ShowSection(runtimes: false);
            }

            _hostWindow = HostWindow;
            if (_hostWindow != null)
            {
                _hostWindow.RootNavigation.SizeChanged += HostNavigation_SizeChanged;
                ApplyHeightLimit();
            }

            WatchConnection(ReloadAllAsync);
            await ReloadAllAsync();
        }

        private async Task ReloadAllAsync()
        {
            await LoadPluginsFromBackendAsync();
            await LoadRuntimesAsync();
        }

        private void PluginsTab_Click(object sender, RoutedEventArgs e) => ShowSection(runtimes: false);

        private void RuntimesTab_Click(object sender, RoutedEventArgs e) => ShowSection(runtimes: true);

        /// <summary>在「插件配置」与「运行时」两个分区之间切换。</summary>
        private void ShowSection(bool runtimes)
        {
            PluginsSection.Visibility = runtimes ? Visibility.Collapsed : Visibility.Visible;
            RuntimesSection.Visibility = runtimes ? Visibility.Visible : Visibility.Collapsed;
            PluginsTabButton.Appearance = runtimes ? ControlAppearance.Secondary : ControlAppearance.Primary;
            RuntimesTabButton.Appearance = runtimes ? ControlAppearance.Primary : ControlAppearance.Secondary;
        }

        private void ConfigPage_Unloaded(object sender, RoutedEventArgs e)
        {
            if (_hostWindow != null)
            {
                _hostWindow.RootNavigation.SizeChanged -= HostNavigation_SizeChanged;
                _hostWindow = null;
            }

            // 离开页面时释放 WebView2，避免后台继续占用浏览器进程。
            _configLoadVersion++;
            foreach (var entry in _views.Values)
            {
                ConfigViewHost.Children.Remove(entry.View);
                try
                {
                    entry.View.Dispose();
                }
                catch
                {
                    // 释放失败不影响页面切换。
                }
            }
            _views.Clear();
            _currentPluginId = string.Empty;
        }

        /// <summary>
        /// 加载插件列表。Kernel 插件的运行时配置由“运行时”分区管理，
        /// 未声明 config_page 的插件没有可编辑配置，均不在此页展示。
        /// interactive=true：用户点击或连接恢复，错误时给出提示；
        /// interactive=false：自动刷新的静默模式，保留当前选中的插件。
        /// </summary>
        private async Task LoadPluginsFromBackendAsync(bool interactive = true)
        {
            try
            {
                if (!Backend.IsCoreConnected)
                {
                    SetDisconnectedState();
                    return;
                }
                var plugins = await Backend.GetPluginsAsync();
                if (!IsLoaded)
                    return;

                var pluginList = plugins
                    .Where(p => p.Type != PluginType.Kernel && p.HasConfigPage)
                    .ToList();

                // 记住当前选中项，避免自动刷新把用户正在查看的插件跳走
                var pendingId = PendingPluginId;
                PendingPluginId = null;
                var selectedId = pendingId
                    ?? (PluginListBox.SelectedItem as PluginInfo)?.Id
                    ?? _currentPluginId;

                _suppressSelectionChanged = true;
                try
                {
                    PluginListBox.ItemsSource = pluginList;

                    if (!string.IsNullOrEmpty(selectedId))
                        PluginListBox.SelectedItem = pluginList.FirstOrDefault(p => p.Id == selectedId);

                    // 首次进入配置页时自动选择第一项，避免右侧出现空白标题区域。
                    if (PluginListBox.SelectedIndex < 0 && pluginList.Count > 0)
                        PluginListBox.SelectedIndex = 0;
                }
                finally
                {
                    _suppressSelectionChanged = false;
                }

                if (pluginList.Count == 0)
                {
                    ClearPluginSelection("暂无可配置插件", "Core 当前没有返回任何提供配置页面的 Base / Active 插件。");
                    return;
                }

                if (PluginListBox.SelectedItem is PluginInfo selected)
                {
                    UpdateHeader(selected);
                    if (selected.Id != _currentPluginId)
                        await ShowPluginConfigAsync(selected);
                }
            }
            catch (Exception ex)
            {
                PluginListBox.ItemsSource = null;
                PluginListBox.SelectedItem = null;
                if (ex is BackendConnectionException)
                    SetDisconnectedState();
                else if (interactive)
                    HostWindow?.ShowToast("加载插件失败", ex.Message, true);
            }
        }

        protected override void SetDisconnectedState()
        {
            // 插件配置分区
            PluginListBox.ItemsSource = null;
            PluginListBox.SelectedItem = null;
            ClearPluginSelection("Core 未连接", "等待后端恢复后会自动重新加载配置内容。");

            // 运行时分区
            _runtimes.Clear();
            _selectedRuntime = null;
            RuntimeList.ItemsSource = null;
            RuntimeListEmptyState.Visibility = Visibility.Visible;
            RuntimeListStatusText.Text = "Core 未连接，恢复连接后会自动刷新";
            RefreshRuntimesButton.IsEnabled = false;
            RestartCoreButton.IsEnabled = false;
            ClearRuntimeDetail("Core 未连接", "恢复连接后会自动加载运行时信息");
        }

        private void ClearPluginSelection(string title, string description)
        {
            // 取消尚未完成的配置加载，避免空状态下又弹出旧插件的配置页。
            _configLoadVersion++;
            _currentPluginId = string.Empty;
            UpdateHeader(title, description);
            ShowEmptyState("暂无配置项", "从左侧列表选择一个插件以查看其配置页面。");
        }

        private void PluginListBox_SelectionChanged(object sender, SelectionChangedEventArgs e)
        {
            if (_suppressSelectionChanged)
                return;
            if (PluginListBox.SelectedItem is not PluginInfo plugin)
                return;

            UpdateHeader(plugin);
            if (plugin.Id != _currentPluginId)
            {
                _ = ShowPluginConfigAsync(plugin);
            }
        }

        private void UpdateHeader(PluginInfo plugin)
        {
            UpdateHeader(
                plugin.Name,
                string.IsNullOrWhiteSpace(plugin.Description) ? "该插件未提供描述。" : plugin.Description);
        }

        private void UpdateHeader(string title, string description)
        {
            SelectedPluginName.Text = title;
            SelectedPluginDesc.Text = description;
        }

        /// <summary>加载并展示单个插件的配置页。</summary>
        private async Task ShowPluginConfigAsync(PluginInfo plugin)
        {
            _currentPluginId = plugin.Id;
            var version = ++_configLoadVersion;
            HideAllViews();
            SetLoading(true);

            try
            {
                var config = await Backend.GetPluginConfigAsync(plugin.Id);
                if (version != _configLoadVersion || !IsLoaded)
                    return;

                if (!config.HasConfigPage)
                {
                    ShowEmptyState(
                        "该插件未提供配置页面",
                        "请在插件的 META-INF.json 中声明 config_page，指向插件目录内的 HTML 文件。");
                    return;
                }

                if (!File.Exists(config.ConfigPage))
                {
                    ShowEmptyState("配置页面不存在", $"找不到配置页文件：{config.ConfigPage}");
                    return;
                }

                var entry = GetOrCreateView(plugin.Id);
                entry.Config = config.Config ?? string.Empty;
                entry.ConfigPagePath = config.ConfigPage;

                if (!entry.IsReady)
                {
                    try
                    {
                        await entry.View.EnsureCoreWebView2Async();
                    }
                    catch (Exception ex)
                    {
                        RemoveView(entry);
                        ShowEmptyState("无法加载配置页面", $"WebView2 运行时不可用：{ex.Message}");
                        return;
                    }

                    if (version != _configLoadVersion || !IsLoaded)
                        return;

                    var core = entry.View.CoreWebView2;
                    core.NavigationCompleted += (_, args) => ConfigView_NavigationCompleted(entry, args);
                    core.WebMessageReceived += (_, args) => ConfigView_WebMessageReceived(entry, args);
                    entry.IsReady = true;
                }

                if (entry.IsLoaded)
                {
                    await InjectConfigAsync(entry);
                    if (version != _configLoadVersion || !IsLoaded)
                        return;

                    ShowView(entry);
                    SetLoading(false);
                    return;
                }

                // 首次打开：导航完成后由 NavigationCompleted 完成配置注入与展示。
                if (!entry.IsNavigating)
                {
                    entry.IsNavigating = true;
                    entry.View.CoreWebView2.Navigate(new Uri(entry.ConfigPagePath).AbsoluteUri);
                }
            }
            catch (BackendConnectionException)
            {
                SetDisconnectedState();
            }
            catch (Exception ex)
            {
                ShowEmptyState("配置页面加载失败", ex.Message);
            }
        }

        private ConfigView GetOrCreateView(string pluginId)
        {
            if (_views.TryGetValue(pluginId, out var existing))
                return existing;

            var view = new WebView2
            {
                HorizontalAlignment = HorizontalAlignment.Stretch,
                VerticalAlignment = VerticalAlignment.Stretch
            };
            ConfigViewHost.Children.Add(view);

            var entry = new ConfigView(pluginId, view);
            _views[pluginId] = entry;
            return entry;
        }

        private void RemoveView(ConfigView entry)
        {
            _views.Remove(entry.PluginId);
            ConfigViewHost.Children.Remove(entry.View);
            try
            {
                entry.View.Dispose();
            }
            catch
            {
                // 释放失败无需处理。
            }
        }

        private async void ConfigView_NavigationCompleted(ConfigView entry, CoreWebView2NavigationCompletedEventArgs e)
        {
            if (!IsLoaded || !_views.TryGetValue(entry.PluginId, out var current) || current != entry)
                return;

            entry.IsNavigating = false;

            if (!e.IsSuccess)
            {
                entry.IsLoaded = false;
                if (entry.PluginId == _currentPluginId)
                    ShowEmptyState("配置页面加载失败", $"WebView2 导航失败：{e.WebErrorStatus}");
                return;
            }

            entry.IsLoaded = true;
            if (entry.PluginId != _currentPluginId)
                return;

            try
            {
                await InjectConfigAsync(entry);
            }
            catch (Exception ex)
            {
                ShowEmptyState("配置页面加载失败", ex.Message);
                return;
            }

            if (!IsLoaded || entry.PluginId != _currentPluginId)
                return;

            ShowView(entry);
            SetLoading(false);
        }

        /// <summary>把该插件的最新配置下发给它自己的配置页。</summary>
        private async Task InjectConfigAsync(ConfigView entry)
        {
            if (entry.View.CoreWebView2 == null)
                return;

            var message = JsonSerializer.Serialize(new { type = "cuckoo:init", config = entry.Config });
            entry.View.CoreWebView2.PostWebMessageAsJson(message);

            // 兼容同步读取：脚本可直接读取 window.__cuckooConfig，
            // 也可监听 cuckoo-config-ready 事件。
            var literal = JsonSerializer.Serialize(entry.Config);
            await entry.View.CoreWebView2.ExecuteScriptAsync(
                $"window.__cuckooConfig = {literal}; " +
                "window.dispatchEvent(new Event('cuckoo-config-ready'));");
        }

        private async void ConfigView_WebMessageReceived(ConfigView entry, CoreWebView2WebMessageReceivedEventArgs e)
        {
            try
            {
                using var doc = JsonDocument.Parse(e.WebMessageAsJson);
                var root = doc.RootElement;

                if (!root.TryGetProperty("type", out var type) ||
                    type.GetString() != "cuckoo:saveConfig")
                {
                    return;
                }

                if (!root.TryGetProperty("config", out var config) ||
                    config.ValueKind != JsonValueKind.String)
                {
                    PostSaveResult(entry, false, "config 必须是字符串");
                    return;
                }

                var value = config.GetString() ?? string.Empty;
                if (value == entry.Config)
                {
                    PostSaveResult(entry, true, string.Empty);
                    return;
                }

                var saved = await Backend.SavePluginConfigAsync(entry.PluginId, value);
                if (saved)
                {
                    entry.Config = value;
                    PostSaveResult(entry, true, string.Empty);
                }
                else
                {
                    PostSaveResult(entry, false, "Core 未确认保存结果");
                }
            }
            catch (Exception ex)
            {
                PostSaveResult(entry, false, ex.Message);
            }
        }

        private void PostSaveResult(ConfigView entry, bool ok, string error)
        {
            try
            {
                if (entry.View.CoreWebView2 == null)
                    return;

                var message = JsonSerializer.Serialize(new
                {
                    type = "cuckoo:saved",
                    ok,
                    error = ok ? null : error
                });
                entry.View.CoreWebView2.PostWebMessageAsJson(message);
            }
            catch
            {
                // 页面可能已关闭，忽略回执失败。
            }
        }

        private void HideAllViews()
        {
            foreach (var entry in _views.Values)
                entry.View.Visibility = Visibility.Collapsed;
        }

        private void ShowView(ConfigView entry)
        {
            foreach (var item in _views.Values)
                item.View.Visibility = item == entry ? Visibility.Visible : Visibility.Collapsed;

            ConfigEmptyState.Visibility = Visibility.Collapsed;
        }

        private void SetLoading(bool loading)
        {
            ConfigLoadingIndicator.Visibility = loading ? Visibility.Visible : Visibility.Collapsed;
            if (loading)
                ConfigEmptyState.Visibility = Visibility.Collapsed;
        }

        private void ShowEmptyState(string title, string description)
        {
            HideAllViews();
            ConfigEmptyTitle.Text = title;
            ConfigEmptyDesc.Text = description;
            ConfigEmptyState.Visibility = Visibility.Visible;
            SetLoading(false);
        }

        // ===== 运行时分区 =====

        private void HostNavigation_SizeChanged(object sender, SizeChangedEventArgs e) => ApplyHeightLimit();

        private void ApplyHeightLimit()
        {
            var height = _hostWindow?.RootNavigation.ActualHeight ?? 0;
            if (height > 0)
                RuntimesSection.MaxHeight = height;
        }

        private async Task LoadRuntimesAsync(bool silent = false)
        {
            _lastRuntimeRefresh = DateTime.UtcNow;

            if (!Backend.IsCoreConnected)
            {
                SetDisconnectedState();
                return;
            }

            try
            {
                if (!silent)
                {
                    RefreshRuntimesButton.IsEnabled = false;
                    RestartCoreButton.IsEnabled = false;
                }

                var runtimes = await Backend.GetRuntimesAsync();
                var previousSignature = string.Join("|", _runtimes.Select(r => r.Signature));
                var nextSignature = string.Join("|", runtimes.Select(r => r.Signature));

                RuntimeListStatusText.Text = runtimes.Count == 0
                    ? "未发现 Kernel 运行时"
                    : $"{runtimes.Count} 个 Kernel";
                RuntimeListEmptyState.Visibility = runtimes.Count == 0
                    ? Visibility.Visible
                    : Visibility.Collapsed;

                if (previousSignature != nextSignature)
                {
                    _runtimes = runtimes;
                    var preferredId = PendingKernelId ?? _selectedRuntime?.KernelId;
                    RuntimeList.ItemsSource = null;
                    RuntimeList.ItemsSource = _runtimes;

                    var target = _runtimes.FirstOrDefault(r => r.KernelId == preferredId)
                                 ?? _runtimes.FirstOrDefault();
                    if (target != null)
                    {
                        RuntimeList.SelectedItem = target;
                        PendingKernelId = null;
                    }
                    else
                    {
                        ClearRuntimeDetail("没有可用的运行时", "请安装或启用 Kernel 后重启 Core");
                    }
                }
                else if (_selectedRuntime != null)
                {
                    // 状态未变化时仅同步详情绑定对象，列表保持稳定
                    var latest = runtimes.FirstOrDefault(r => r.KernelId == _selectedRuntime.KernelId);
                    if (latest != null && !ReferenceEquals(latest, _selectedRuntime))
                    {
                        _selectedRuntime = latest;
                        RuntimeDetailPanel.DataContext = latest;
                    }
                }
            }
            catch (Exception ex)
            {
                if (ex is BackendConnectionException)
                    SetDisconnectedState();

                if (!silent)
                    await ShowBackendErrorAsync("加载运行时失败", ex);
            }
            finally
            {
                var connected = Backend.IsCoreConnected;
                RefreshRuntimesButton.IsEnabled = connected;
                RestartCoreButton.IsEnabled = connected && Backend.IsDaemonConnected;
            }
        }

        private async void RuntimeList_SelectionChanged(object sender, SelectionChangedEventArgs e)
        {
            if (RuntimeList.SelectedItem is not RuntimeInfo info)
                return;

            _selectedRuntime = info;
            await ShowRuntimeDetailAsync(info);
        }

        private async Task ShowRuntimeDetailAsync(RuntimeInfo info, bool refreshSelection = false)
        {
            var requestId = ++_runtimeDetailRequestId;

            RuntimeDetailPanel.DataContext = info;
            RuntimeDetailScroll.Visibility = Visibility.Visible;
            RuntimeDetailEmptyState.Visibility = Visibility.Collapsed;
            RuntimeSelectionInfoBar.IsOpen = false;

            if (!string.IsNullOrWhiteSpace(info.LastError) &&
                string.Equals(info.Status, "error", StringComparison.OrdinalIgnoreCase))
            {
                RuntimeErrorInfoBar.Message = info.LastError;
                RuntimeErrorInfoBar.IsOpen = true;
            }
            else
            {
                RuntimeErrorInfoBar.IsOpen = false;
            }

            await LoadRuntimeSelectionPanelAsync(info, requestId, refreshSelection);

            if (requestId != _runtimeDetailRequestId)
                return;

            await LoadRuntimeSettingsAsync(info, requestId);
            if (requestId != _runtimeDetailRequestId)
                return;

            await LoadRuntimeOperationsAsync(info, requestId);
            if (requestId != _runtimeDetailRequestId)
                return;

            await LoadRuntimeStateAsync(info, requestId);
        }

        private async Task LoadRuntimeSelectionPanelAsync(RuntimeInfo info, int requestId, bool refresh)
        {
            RuntimeSelectionHost.Children.Clear();

            try
            {
                var panel = await Backend.GetRuntimeSelectionPanelAsync(info.KernelId, refresh);
                if (requestId != _runtimeDetailRequestId)
                    return;

                if (panel.Pages.Count == 0)
                {
                    RuntimeSelectionHost.Children.Add(new TextBlock
                    {
                        Text = "未提供运行时选择信息",
                        FontSize = 11,
                        Foreground = SecondaryBrush,
                        TextWrapping = TextWrapping.Wrap
                    });
                    return;
                }

                foreach (var page in panel.Pages)
                {
                    RuntimeSelectionHost.Children.Add(new TextBlock
                    {
                        Text = page.Name,
                        FontSize = 15,
                        FontWeight = FontWeights.SemiBold,
                        Margin = new Thickness(0, 0, 0, 8)
                    });

                    foreach (var section in page.Sections)
                    {
                        if (section.Settings.Count == 0)
                            continue;

                        RuntimeSelectionHost.Children.Add(new TextBlock
                        {
                            Text = section.Name,
                            FontSize = 12,
                            Foreground = SecondaryBrush,
                            Margin = new Thickness(0, 4, 0, 6)
                        });

                        foreach (var setting in section.Settings)
                            RuntimeSelectionHost.Children.Add(BuildRuntimeSelectionSetting(setting, info, requestId));
                    }
                }
            }
            catch (Exception ex)
            {
                if (ex is BackendConnectionException)
                {
                    SetDisconnectedState();
                    return;
                }

                RuntimeSelectionHost.Children.Add(new TextBlock
                {
                    Text = $"读取运行时选择信息失败：{ex.Message}",
                    FontSize = 11,
                    Foreground = SecondaryBrush,
                    TextWrapping = TextWrapping.Wrap
                });
            }
        }

        private FrameworkElement BuildRuntimeSelectionSetting(Setting setting, RuntimeInfo info, int requestId)
        {
            var label = setting.DisplayName ? setting.Name : setting.Key;
            var container = new StackPanel { Margin = new Thickness(0, 0, 0, 12) };
            container.Children.Add(BuildSettingLabel(setting, label));

            if (setting.Editor is SelectEditor selectEditor)
            {
                var combo = new ComboBox
                {
                    MinWidth = 380,
                    MaxWidth = 620,
                    HorizontalAlignment = HorizontalAlignment.Left,
                    Margin = new Thickness(0, 6, 0, 0),
                    Foreground = PrimaryBrush,
                    Background = ControlBrush
                };

                ComboBoxItem? selected = null;
                var current = ValueToString(setting.Value);
                foreach (var option in selectEditor.Options)
                {
                    var item = new ComboBoxItem
                    {
                        Content = option.Text,
                        Tag = option.Value,
                        Foreground = PrimaryBrush,
                        Background = ControlBrush
                    };
                    combo.Items.Add(item);
                    if (string.Equals(ValueToString(option.Value), current, StringComparison.OrdinalIgnoreCase))
                        selected = item;
                }
                if (selected == null && combo.Items.Count > 0)
                    selected = (ComboBoxItem)combo.Items[0];
                combo.SelectedItem = selected;

                container.Children.Add(combo);
                AddRuntimeSelectionActions(container, setting.Actions, info, requestId, combo);
                return container;
            }

            if (setting.Editor is TextEditor textEditor)
            {
                var textBox = new WpfTextBox
                {
                    Text = ValueToString(setting.Value),
                    IsReadOnly = true,
                    FontSize = 11,
                    Margin = new Thickness(0, 6, 0, 0),
                    TextWrapping = TextWrapping.Wrap,
                    AcceptsReturn = textEditor.MultiLine,
                    MinHeight = textEditor.MultiLine ? 76 : 0,
                    MaxHeight = textEditor.MultiLine ? 180 : double.PositiveInfinity,
                    VerticalScrollBarVisibility = textEditor.MultiLine
                        ? ScrollBarVisibility.Auto
                        : ScrollBarVisibility.Disabled
                };
                container.Children.Add(textBox);
                return container;
            }

            container.Children.Add(new TextBlock
            {
                Text = ValueToString(setting.Value),
                FontSize = 11,
                Foreground = SecondaryBrush,
                TextWrapping = TextWrapping.Wrap,
                Margin = new Thickness(0, 4, 0, 0)
            });
            return container;
        }

        private void AddRuntimeSelectionActions(StackPanel host, List<SettingAction>? actions,
            RuntimeInfo info, int requestId, ComboBox combo)
        {
            if (actions == null || actions.Count == 0)
                return;

            var panel = new WrapPanel { Margin = new Thickness(0, 8, 0, 0) };
            foreach (var action in actions)
            {
                var button = new Wpf.Ui.Controls.Button
                {
                    Content = action.Text,
                    Appearance = ControlAppearance.Secondary,
                    Margin = new Thickness(0, 0, 8, 0)
                };
                button.Click += async (_, _) =>
                    await HandleRuntimeSelectionActionAsync(action, info, requestId, combo);
                panel.Children.Add(button);
            }
            host.Children.Add(panel);
        }

        private async Task HandleRuntimeSelectionActionAsync(SettingAction action, RuntimeInfo info,
            int requestId, ComboBox combo)
        {
            switch (action.Type)
            {
                case ActionType.RefreshRuntime:
                    await LoadRuntimeSelectionPanelAsync(info, requestId, refresh: true);
                    break;

                case ActionType.BrowseRuntime:
                    BrowseRuntimeIntoCombo(combo);
                    break;

                case ActionType.ApplyRuntime:
                    await ApplyRuntimeSelectionAsync(info, requestId, combo);
                    break;
            }
        }

        private void BrowseRuntimeIntoCombo(ComboBox combo)
        {
            using var dialog = new System.Windows.Forms.FolderBrowserDialog
            {
                Description = "选择 Python 安装目录（包含 python.exe）",
                ShowNewFolderButton = true
            };

            var current = combo.SelectedItem is ComboBoxItem currentItem
                ? currentItem.Tag?.ToString()
                : null;
            if (!string.IsNullOrWhiteSpace(current) && Directory.Exists(current))
                dialog.SelectedPath = current;

            if (dialog.ShowDialog() != System.Windows.Forms.DialogResult.OK)
                return;

            var path = dialog.SelectedPath;
            ComboBoxItem? existing = null;
            foreach (ComboBoxItem item in combo.Items)
            {
                if (string.Equals(item.Tag?.ToString(), path, StringComparison.OrdinalIgnoreCase))
                {
                    existing = item;
                    break;
                }
            }

            if (existing == null)
            {
                existing = new ComboBoxItem
                {
                    Content = "手动指定：" + path,
                    Tag = path,
                    Foreground = PrimaryBrush,
                    Background = ControlBrush
                };
                combo.Items.Add(existing);
            }
            combo.SelectedItem = existing;
        }

        private async Task ApplyRuntimeSelectionAsync(RuntimeInfo info, int requestId, ComboBox combo)
        {
            var home = combo.SelectedItem is ComboBoxItem item
                ? item.Tag?.ToString() ?? string.Empty
                : string.Empty;

            try
            {
                var result = await Backend.SelectRuntimeAsync(info.KernelId, home);
                if (!result.Ok)
                    throw new InvalidOperationException("Core 未确认运行时选择");

                await LoadRuntimeSelectionPanelAsync(info, requestId, refresh: true);
                await LoadRuntimesAsync(silent: true);

                if (!result.RequiresRestart)
                {
                    HostWindow?.ShowToast(
                        "运行时选择未变化",
                        string.IsNullOrWhiteSpace(home) ? "自动探测" : home);
                    return;
                }

                RuntimeSelectionInfoBar.Message = "运行时选择已保存，重启 Core 后生效";
                RuntimeSelectionInfoBar.IsOpen = true;

                var window = HostWindow;
                if (window == null)
                    return;

                var confirmed = await window.ShowConfirmDialogAsync(
                    "应用运行时",
                    $"Python 运行时已设置为：\n{(string.IsNullOrWhiteSpace(home) ? "自动探测" : home)}\n\n是否立即重启 Core 以应用？",
                    "重启并应用",
                    "稍后");
                if (confirmed)
                    await RestartCoreInternalAsync();
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("应用运行时选择失败", ex);
            }
        }

        private async Task LoadRuntimeSettingsAsync(RuntimeInfo info, int requestId)
        {
            RuntimeSettingsHost.Children.Clear();
            RuntimeSettingsEmptyText.Visibility = Visibility.Collapsed;

            if (!info.SupportsControlPlane)
            {
                RuntimeSettingsEmptyText.Text = "该 Kernel 未实现 Runtime Control Plane";
                RuntimeSettingsEmptyText.Visibility = Visibility.Visible;
                return;
            }

            try
            {
                var panel = await Backend.GetRuntimeSettingsPanelAsync(info.KernelId);
                if (requestId != _runtimeDetailRequestId)
                    return;

                var hasSettings = false;
                foreach (var page in panel.Pages)
                {
                    foreach (var section in page.Sections)
                    {
                        if (section.Settings.Count == 0)
                            continue;

                        hasSettings = true;
                        RuntimeSettingsHost.Children.Add(new TextBlock
                        {
                            Text = section.Name,
                            FontSize = 12,
                            Foreground = SecondaryBrush,
                            Margin = new Thickness(0, 0, 0, 6)
                        });

                        foreach (var setting in section.Settings)
                            RuntimeSettingsHost.Children.Add(BuildSettingEditor(setting));
                    }
                }

                if (!hasSettings)
                {
                    RuntimeSettingsEmptyText.Text = "该 Kernel 未提供运行时设置";
                    RuntimeSettingsEmptyText.Visibility = Visibility.Visible;
                }
            }
            catch (Exception ex)
            {
                if (ex is BackendConnectionException)
                {
                    SetDisconnectedState();
                    return;
                }
                RuntimeSettingsEmptyText.Text = $"读取 Kernel 设置失败：{ex.Message}";
                RuntimeSettingsEmptyText.Visibility = Visibility.Visible;
            }
        }

        private FrameworkElement BuildSettingEditor(Setting setting)
        {
            var label = setting.DisplayName ? setting.Name : setting.Key;
            var container = new Border
            {
                BorderBrush = FindResource("ControlStrokeColorDefaultBrush") as Brush,
                BorderThickness = new Thickness(0, 1, 0, 0),
                Padding = new Thickness(0, 9, 0, 9)
            };

            if (setting.Editor is SwitchEditor)
            {
                var grid = new Grid();
                grid.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
                grid.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });

                grid.Children.Add(BuildSettingLabel(setting, label));

                var toggle = new ToggleSwitch
                {
                    IsChecked = ValueToBool(setting.Value),
                    VerticalAlignment = VerticalAlignment.Center
                };
                Grid.SetColumn(toggle, 1);
                toggle.Checked += async (_, _) => await ApplyRuntimeSettingAsync(setting.Key, true);
                toggle.Unchecked += async (_, _) => await ApplyRuntimeSettingAsync(setting.Key, false);
                grid.Children.Add(toggle);

                container.Child = grid;
                return container;
            }

            if (setting.Editor is SelectEditor selectEditor)
            {
                var grid = new Grid();
                grid.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
                grid.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });

                grid.Children.Add(BuildSettingLabel(setting, label));

                var combo = new ComboBox
                {
                    MinWidth = 180,
                    HorizontalAlignment = HorizontalAlignment.Right,
                    VerticalAlignment = VerticalAlignment.Center,
                    Foreground = PrimaryBrush,
                    Background = ControlBrush
                };
                var current = ValueToString(setting.Value);
                foreach (var option in selectEditor.Options)
                {
                    var item = new ComboBoxItem
                    {
                        Content = option.Text,
                        Tag = option.Value,
                        Foreground = PrimaryBrush,
                        Background = ControlBrush
                    };
                    combo.Items.Add(item);
                    if (string.Equals(ValueToString(option.Value), current, StringComparison.Ordinal))
                        combo.SelectedItem = item;
                }
                if (combo.SelectedItem == null && combo.Items.Count > 0)
                    combo.SelectedIndex = 0;

                combo.SelectionChanged += async (_, _) =>
                {
                    if (combo.SelectedItem is ComboBoxItem selectedItem && selectedItem.Tag != null)
                        await ApplyRuntimeSettingAsync(setting.Key, selectedItem.Tag);
                };

                Grid.SetColumn(combo, 1);
                grid.Children.Add(combo);
                container.Child = grid;
                return container;
            }

            if (setting.Editor is TextEditor textEditor)
            {
                var stack = new StackPanel();
                stack.Children.Add(BuildSettingLabel(setting, label));

                var value = ValueToString(setting.Value);
                if (textEditor.ReadOnly)
                {
                    stack.Children.Add(new WpfTextBox
                    {
                        Text = value,
                        IsReadOnly = true,
                        FontSize = 11,
                        TextWrapping = TextWrapping.Wrap,
                        Margin = new Thickness(0, 6, 0, 0)
                    });
                }
                else
                {
                    var grid = new Grid { Margin = new Thickness(0, 6, 0, 0) };
                    grid.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
                    grid.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });

                    var input = new WpfTextBox { Text = value };
                    grid.Children.Add(input);

                    var apply = new Wpf.Ui.Controls.Button
                    {
                        Content = "应用",
                        Appearance = ControlAppearance.Secondary,
                        Margin = new Thickness(8, 0, 0, 0)
                    };
                    apply.Click += async (_, _) => await ApplyRuntimeSettingAsync(setting.Key, input.Text);
                    Grid.SetColumn(apply, 1);
                    grid.Children.Add(apply);

                    stack.Children.Add(grid);
                }

                container.Child = stack;
                return container;
            }

            var fallback = new StackPanel();
            fallback.Children.Add(BuildSettingLabel(setting, label));
            fallback.Children.Add(new TextBlock
            {
                Text = ValueToString(setting.Value),
                FontSize = 11,
                Foreground = SecondaryBrush,
                Margin = new Thickness(0, 4, 0, 0),
                TextWrapping = TextWrapping.Wrap
            });
            container.Child = fallback;
            return container;
        }

        private FrameworkElement BuildSettingLabel(Setting setting, string label)
        {
            var stack = new StackPanel
            {
                VerticalAlignment = VerticalAlignment.Center,
                Margin = new Thickness(0, 0, 16, 0)
            };
            stack.Children.Add(new TextBlock
            {
                Text = label,
                FontSize = 13,
                Foreground = PrimaryBrush,
                TextWrapping = TextWrapping.Wrap
            });
            if (setting.DisplayDescription && !string.IsNullOrWhiteSpace(setting.Description))
            {
                stack.Children.Add(new TextBlock
                {
                    Text = setting.Description,
                    FontSize = 11,
                    Foreground = SecondaryBrush,
                    TextWrapping = TextWrapping.Wrap,
                    Margin = new Thickness(0, 2, 0, 0)
                });
            }
            return stack;
        }

        private async Task ApplyRuntimeSettingAsync(string key, object value)
        {
            if (_selectedRuntime == null)
                return;

            try
            {
                var result = await Backend.SetRuntimeSettingAsync(_selectedRuntime.KernelId, key, value);
                if (!result.Ok)
                {
                    throw new InvalidOperationException(string.IsNullOrWhiteSpace(result.Error)
                        ? "Kernel 拒绝了该设置"
                        : result.Error);
                }

                HostWindow?.ShowToast("运行时设置已应用", key);
                await LoadRuntimeStateAsync(_selectedRuntime, _runtimeDetailRequestId);
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("更新运行时设置失败", ex);
                await LoadRuntimeSettingsAsync(_selectedRuntime, _runtimeDetailRequestId);
            }
        }

        private async Task LoadRuntimeOperationsAsync(RuntimeInfo info, int requestId)
        {
            RuntimeOperationsHost.Children.Clear();
            RuntimeOperationsEmptyText.Visibility = Visibility.Collapsed;

            if (!info.SupportsControlPlane)
            {
                RuntimeOperationsEmptyText.Text = "该 Kernel 未声明运行时操作";
                RuntimeOperationsEmptyText.Visibility = Visibility.Visible;
                return;
            }

            try
            {
                var operations = await Backend.GetRuntimeOperationsAsync(info.KernelId);
                if (requestId != _runtimeDetailRequestId)
                    return;

                if (operations.Count == 0)
                {
                    RuntimeOperationsEmptyText.Text = "该 Kernel 未声明运行时操作";
                    RuntimeOperationsEmptyText.Visibility = Visibility.Visible;
                    return;
                }

                foreach (var operation in operations)
                {
                    var button = new Wpf.Ui.Controls.Button
                    {
                        Content = string.IsNullOrWhiteSpace(operation.Name) ? operation.Id : operation.Name,
                        Appearance = ControlAppearance.Secondary,
                        Margin = new Thickness(0, 0, 8, 8),
                        ToolTip = operation.Description
                    };
                    button.Click += async (_, _) => await InvokeRuntimeOperationAsync(operation, button);
                    RuntimeOperationsHost.Children.Add(button);
                }
            }
            catch (Exception ex)
            {
                if (ex is BackendConnectionException)
                {
                    SetDisconnectedState();
                    return;
                }
                RuntimeOperationsEmptyText.Text = $"读取运行时操作失败：{ex.Message}";
                RuntimeOperationsEmptyText.Visibility = Visibility.Visible;
            }
        }

        private async Task InvokeRuntimeOperationAsync(RuntimeOperation operation, Wpf.Ui.Controls.Button button)
        {
            if (_selectedRuntime == null)
                return;

            try
            {
                button.IsEnabled = false;
                var result = await Backend.InvokeRuntimeOperationAsync(_selectedRuntime.KernelId, operation.Id);
                if (result.ValueKind == JsonValueKind.Object &&
                    result.TryGetProperty("ok", out var okProp) &&
                    okProp.ValueKind == JsonValueKind.False)
                {
                    var error = result.TryGetProperty("error", out var errorProp)
                        ? errorProp.GetString()
                        : null;
                    throw new InvalidOperationException(string.IsNullOrWhiteSpace(error)
                        ? "Kernel 返回失败结果"
                        : error);
                }

                HostWindow?.ShowToast("操作完成", operation.Name);
                await LoadRuntimeStateAsync(_selectedRuntime, _runtimeDetailRequestId);
                await LoadRuntimesAsync(silent: true);
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("执行运行时操作失败", ex);
            }
            finally
            {
                button.IsEnabled = true;
            }
        }

        private async Task LoadRuntimeStateAsync(RuntimeInfo info, int requestId)
        {
            if (!info.SupportsControlPlane)
            {
                RuntimeStateTextBox.Text = "该 Kernel 未实现 Runtime Control Plane";
                return;
            }

            try
            {
                var state = await Backend.GetRuntimeStateAsync(info.KernelId);
                if (requestId != _runtimeDetailRequestId)
                    return;

                RuntimeStateTextBox.Text = JsonSerializer.Serialize(state, IndentedJsonOptions);
            }
            catch (Exception ex)
            {
                if (ex is BackendConnectionException)
                {
                    SetDisconnectedState();
                    return;
                }
                RuntimeStateTextBox.Text = $"读取运行时状态失败：{ex.Message}";
            }
        }

        private async void RefreshRuntimes_Click(object sender, RoutedEventArgs e)
        {
            await LoadRuntimesAsync();
            if (RuntimeList.SelectedItem is RuntimeInfo info)
                await ShowRuntimeDetailAsync(info, refreshSelection: true);
        }

        private async void RestartCore_Click(object sender, RoutedEventArgs e)
        {
            var window = HostWindow;
            if (window == null)
                return;

            var confirmed = await window.ShowConfirmDialogAsync(
                "重启 Core",
                "重启 Core 会中止当前插件运行，并在启动后应用新的运行时选择。确定继续吗？",
                "重启");
            if (!confirmed)
                return;

            await RestartCoreInternalAsync();
        }

        private async Task RestartCoreInternalAsync()
        {
            var window = HostWindow;
            if (window == null)
                return;

            try
            {
                RestartCoreButton.IsEnabled = false;
                var ok = await Backend.RestartCoreAsync();
                if (!ok)
                    throw new InvalidOperationException("Daemon 未确认 Core 重启");

                window.ShowToast("正在重启 Core", "运行时会重新加载，请稍候…");
                await Task.Delay(1800);
                await LoadRuntimesAsync();
                if (RuntimeList.SelectedItem is RuntimeInfo info)
                    await ShowRuntimeDetailAsync(info, refreshSelection: true);
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("重启 Core 失败", ex);
            }
            finally
            {
                RestartCoreButton.IsEnabled = Backend.IsDaemonConnected;
            }
        }

        private void ClearRuntimeDetail(string title, string detail)
        {
            _runtimeDetailRequestId++;
            RuntimeDetailScroll.Visibility = Visibility.Collapsed;
            RuntimeDetailEmptyState.Visibility = Visibility.Visible;
            RuntimeDetailEmptyTitle.Text = title;
            RuntimeDetailEmptyDetail.Text = detail;
            RuntimeDetailPanel.DataContext = null;
            RuntimeSettingsHost.Children.Clear();
            RuntimeOperationsHost.Children.Clear();
            RuntimeStateTextBox.Text = string.Empty;
            RuntimeSelectionHost.Children.Clear();
            RuntimeSelectionInfoBar.IsOpen = false;
        }

        private static string ValueToString(object? value)
        {
            return value switch
            {
                null => string.Empty,
                JsonElement element => element.ValueKind switch
                {
                    JsonValueKind.String => element.GetString() ?? string.Empty,
                    JsonValueKind.Null or JsonValueKind.Undefined => string.Empty,
                    _ => element.ToString()
                },
                IFormattable formattable => formattable.ToString(null, CultureInfo.InvariantCulture),
                _ => value.ToString() ?? string.Empty
            };
        }

        private static bool ValueToBool(object? value)
        {
            return value switch
            {
                bool boolean => boolean,
                JsonElement element when element.ValueKind == JsonValueKind.True => true,
                JsonElement element when element.ValueKind == JsonValueKind.False => false,
                string text => bool.TryParse(text, out var parsed) && parsed,
                _ => false
            };
        }

        /// <summary>单个插件的配置页状态与该插件独享的 WebView2 实例。</summary>
        private sealed class ConfigView
        {
            public ConfigView(string pluginId, WebView2 view)
            {
                PluginId = pluginId;
                View = view;
            }

            public string PluginId { get; }

            public WebView2 View { get; }

            public string Config { get; set; } = string.Empty;

            public string ConfigPagePath { get; set; } = string.Empty;

            /// <summary>CoreWebView2 已初始化并挂接事件。</summary>
            public bool IsReady { get; set; }

            /// <summary>配置页已完成首次导航并注入过配置。</summary>
            public bool IsLoaded { get; set; }

            public bool IsNavigating { get; set; }
        }
    }
}
