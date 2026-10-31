// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
using System;
using System.IO;
using System.Linq;
using System.Text.Json;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;
using Microsoft.Web.WebView2.Core;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// ConfigPage.xaml 的交互逻辑。
    /// <para>
    /// 插件配置页由 META-INF.json 的 config_page 指定 HTML，通过 WebView2 加载。
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

        private string _currentPluginId = string.Empty;
        private string _currentConfig = string.Empty;
        private int _configLoadVersion;
        private bool _webViewReady;
        private bool _suppressSelectionChanged;

        public ConfigPage()
        {
            InitializeComponent();
            Loaded += ConfigPage_Loaded;
            Unloaded += (_, _) => _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
        }

        // 定时刷新插件列表；配置页内容只在选中插件变化时重新加载。
        protected override Task OnAutoRefreshAsync() => LoadPluginsFromBackendAsync(interactive: false);

        private async void ConfigPage_Loaded(object sender, RoutedEventArgs e)
        {
            _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
            _backend.ConnectionStateChanged += Backend_ConnectionStateChanged;
            await LoadPluginsFromBackendAsync();
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
        /// 加载插件列表。Kernel 插件的运行时配置由“运行时”页面管理，不在此页展示。
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
                var pluginList = plugins.Where(p => p.Type != PluginType.Kernel).ToList();

                // 记住当前选中项，避免自动刷新把用户正在查看的插件跳走
                var selectedId = (PluginListBox.SelectedItem as PluginInfo)?.Id ?? _currentPluginId;

                _suppressSelectionChanged = true;
                try
                {
                    PluginListBox.ItemsSource = pluginList;

                    if (selectedId != null)
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
                    ClearPluginSelection("暂无可配置插件", "Core 当前没有返回任何 Base / Active 插件。");
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
            _currentConfig = string.Empty;
            var version = ++_configLoadVersion;
            try
            {
                var config = await _backend.GetPluginConfigAsync(plugin.Id);
                if (version != _configLoadVersion)
                    return;
                _currentConfig = config.Config ?? string.Empty;

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

                if (!await EnsureWebViewAsync())
                    return;
                if (version != _configLoadVersion)
                    return;

                ShowConfigPage();
                ConfigWebView.CoreWebView2.Navigate(new Uri(config.ConfigPage).AbsoluteUri);
            }
            catch (BackendConnectionException)
            {
                SetDisconnectedState();
            }
            catch (Exception ex)
            {
                _currentPluginId = string.Empty;
                ShowEmptyState("配置页面加载失败", ex.Message);
            }
        }

        /// <summary>确保 WebView2 已初始化。失败时显示空状态并返回 false。</summary>
        private async Task<bool> EnsureWebViewAsync()
        {
            if (_webViewReady)
                return true;

            try
            {
                await ConfigWebView.EnsureCoreWebView2Async();
                var core = ConfigWebView.CoreWebView2;
                core.NavigationCompleted += ConfigWebView_NavigationCompleted;
                core.WebMessageReceived += ConfigWebView_WebMessageReceived;
                _webViewReady = true;
                return true;
            }
            catch (Exception ex)
            {
                ShowEmptyState("无法加载配置页面", $"WebView2 运行时不可用：{ex.Message}");
                return false;
            }
        }

        private void ConfigWebView_NavigationCompleted(object? sender, CoreWebView2NavigationCompletedEventArgs e)
        {
            if (!e.IsSuccess)
            {
                ShowEmptyState("配置页面加载失败", $"WebView2 导航失败：{e.WebErrorStatus}");
                return;
            }
            _ = InjectConfigAsync();
        }

        /// <summary>把当前配置下发给配置页。</summary>
        private async Task InjectConfigAsync()
        {
            if (ConfigWebView.CoreWebView2 == null)
                return;

            var message = JsonSerializer.Serialize(new { type = "cuckoo:init", config = _currentConfig });
            ConfigWebView.CoreWebView2.PostWebMessageAsJson(message);

            // 兼容同步读取：脚本可直接读取 window.__cuckooConfig，
            // 也可监听 cuckoo-config-ready 事件。
            var literal = JsonSerializer.Serialize(_currentConfig);
            await ConfigWebView.CoreWebView2.ExecuteScriptAsync(
                $"window.__cuckooConfig = {literal}; " +
                "window.dispatchEvent(new Event('cuckoo-config-ready'));");
        }

        private async void ConfigWebView_WebMessageReceived(object? sender, CoreWebView2WebMessageReceivedEventArgs e)
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
                    PostSaveResult(false, "config 必须是字符串");
                    return;
                }

                var value = config.GetString() ?? string.Empty;
                if (value == _currentConfig)
                {
                    PostSaveResult(true, string.Empty);
                    return;
                }

                var saved = await _backend.SavePluginConfigAsync(_currentPluginId, value);
                if (saved)
                {
                    _currentConfig = value;
                    PostSaveResult(true, string.Empty);
                }
                else
                {
                    PostSaveResult(false, "Core 未确认保存结果");
                }
            }
            catch (Exception ex)
            {
                PostSaveResult(false, ex.Message);
            }
        }

        private void PostSaveResult(bool ok, string error)
        {
            if (ConfigWebView.CoreWebView2 == null)
                return;

            var message = JsonSerializer.Serialize(new
            {
                type = "cuckoo:saved",
                ok,
                error = ok ? null : error
            });
            ConfigWebView.CoreWebView2.PostWebMessageAsJson(message);
        }

        private void ShowConfigPage()
        {
            ConfigEmptyState.Visibility = Visibility.Collapsed;
        }

        private void ShowEmptyState(string title, string description)
        {
            ConfigEmptyTitle.Text = title;
            ConfigEmptyDesc.Text = description;
            ConfigEmptyState.Visibility = Visibility.Visible;
        }
    }
}
