// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Text.Json;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;
using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.Wpf;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// ConfigPage.xaml 的交互逻辑。
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
        private readonly CoreBackend _backend = CoreBackend.Instance;

        /// <summary>按插件 ID 缓存的独立 WebView2 实例。</summary>
        private readonly Dictionary<string, ConfigView> _views = new(StringComparer.OrdinalIgnoreCase);

        private string _currentPluginId = string.Empty;
        private int _configLoadVersion;
        private bool _suppressSelectionChanged;

        /// <summary>从插件页跳转过来时需要自动选中的插件 ID。</summary>
        public static string? PendingPluginId { get; set; }

        public ConfigPage()
        {
            InitializeComponent();
            Loaded += ConfigPage_Loaded;
            Unloaded += ConfigPage_Unloaded;
        }

        // 定时刷新插件列表；配置页内容只在选中插件变化时重新加载。
        protected override Task OnAutoRefreshAsync() => LoadPluginsFromBackendAsync(interactive: false);

        private async void ConfigPage_Loaded(object sender, RoutedEventArgs e)
        {
            _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
            _backend.ConnectionStateChanged += Backend_ConnectionStateChanged;
            await LoadPluginsFromBackendAsync();
        }

        private void ConfigPage_Unloaded(object sender, RoutedEventArgs e)
        {
            _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;

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

        private void Backend_ConnectionStateChanged(object? sender, BackendConnectionStateChangedEventArgs e)
        {
            if (!e.IsCore || !IsVisible) return;
            _ = Dispatcher.InvokeAsync(async () =>
            {
                if (e.IsConnected)
                    await LoadPluginsFromBackendAsync();
                else
                    SetDisconnectedState();
            });
        }

        /// <summary>
        /// 加载插件列表。Kernel 插件的运行时配置由“运行时”页面管理，
        /// 未声明 config_page 的插件没有可编辑配置，均不在此页展示。
        /// interactive=true：用户点击或连接恢复，错误时给出提示；
        /// interactive=false：自动刷新的静默模式，保留当前选中的插件。
        /// </summary>
        private async Task LoadPluginsFromBackendAsync(bool interactive = true)
        {
            try
            {
                if (!_backend.IsCoreConnected)
                {
                    SetDisconnectedState();
                    return;
                }
                var plugins = await _backend.GetPluginsAsync();
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
                    (Window.GetWindow(this) as CuckooInterfaceUI.MainWindow)?.ShowToast("加载插件失败", ex.Message, true);
            }
        }

        private void SetDisconnectedState()
        {
            _currentPluginId = string.Empty;
            PluginListBox.ItemsSource = null;
            PluginListBox.SelectedItem = null;
            ClearPluginSelection("Core 未连接", "等待后端恢复后会自动重新加载配置内容。");
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
                var config = await _backend.GetPluginConfigAsync(plugin.Id);
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

                var saved = await _backend.SavePluginConfigAsync(entry.PluginId, value);
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
