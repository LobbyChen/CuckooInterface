// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
using System;
using System.Collections.Generic;
using System.Globalization;
using System.Linq;
using System.Text.Json;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;
using Wpf.Ui.Controls;
using TextBlock = System.Windows.Controls.TextBlock;
using WpfTextBox = Wpf.Ui.Controls.TextBox;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// 运行时页面：展示 RuntimeManager 聚合的 Kernel 状态，并渲染 Kernel
    /// 通过 Runtime Control Plane 下发的设置面板与操作。
    /// </summary>
    public partial class RuntimesPage : AutoRefreshPage
    {
        private static readonly TimeSpan DetailAutoRefreshInterval = TimeSpan.FromSeconds(3);

        private readonly CoreBackend _backend = CoreBackend.Instance;
        private List<RuntimeInfo> _runtimes = new();
        private RuntimeInfo? _selected;
        private DateTime _lastAutoRefresh = DateTime.MinValue;
        private int _detailRequestId;
        private MainWindow? _hostWindow;

        /// <summary>从插件页跳转时指定需要选中的 Kernel ID。</summary>
        public static string? PendingKernelId { get; set; }

        public RuntimesPage()
        {
            InitializeComponent();
            Loaded += RuntimesPage_Loaded;
            Unloaded += RuntimesPage_Unloaded;
        }

        protected override async Task OnAutoRefreshAsync()
        {
            if (DateTime.UtcNow - _lastAutoRefresh < DetailAutoRefreshInterval)
                return;

            await LoadRuntimesAsync(silent: true);
        }

        private async void RuntimesPage_Loaded(object sender, RoutedEventArgs e)
        {
            _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
            _backend.ConnectionStateChanged += Backend_ConnectionStateChanged;

            _hostWindow = Window.GetWindow(this) as MainWindow;
            if (_hostWindow != null)
            {
                _hostWindow.RootNavigation.SizeChanged -= HostNavigation_SizeChanged;
                _hostWindow.RootNavigation.SizeChanged += HostNavigation_SizeChanged;
                ApplyHeightLimit();
            }

            await LoadRuntimesAsync();
        }

        private void RuntimesPage_Unloaded(object sender, RoutedEventArgs e)
        {
            _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
            if (_hostWindow != null)
            {
                _hostWindow.RootNavigation.SizeChanged -= HostNavigation_SizeChanged;
                _hostWindow = null;
            }
        }

        private void HostNavigation_SizeChanged(object sender, SizeChangedEventArgs e)
        {
            ApplyHeightLimit();
        }

        private void ApplyHeightLimit()
        {
            var height = _hostWindow?.RootNavigation.ActualHeight ?? 0;
            if (height > 0)
                PageRootGrid.MaxHeight = height;
        }

        private void Backend_ConnectionStateChanged(object? sender, BackendConnectionStateChangedEventArgs e)
        {
            if (!e.IsCore || !IsVisible)
                return;

            _ = Dispatcher.InvokeAsync(async () =>
            {
                if (e.IsConnected)
                    await LoadRuntimesAsync();
                else
                    SetDisconnectedState();
            });
        }

        private async Task LoadRuntimesAsync(bool silent = false)
        {
            _lastAutoRefresh = DateTime.UtcNow;

            if (!_backend.IsCoreConnected)
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

                var runtimes = await _backend.GetRuntimesAsync();
                var previousSignature = string.Join("|", _runtimes.Select(r => r.Signature));
                var nextSignature = string.Join("|", runtimes.Select(r => r.Signature));

                RuntimeListStatusText.Text = runtimes.Count == 0
                    ? "未发现 Kernel 运行时。"
                    : $"{runtimes.Count} 个 Kernel · 由 RuntimeManager 统一管理";
                RuntimeListEmptyState.Visibility = runtimes.Count == 0
                    ? Visibility.Visible
                    : Visibility.Collapsed;

                if (previousSignature != nextSignature)
                {
                    _runtimes = runtimes;
                    var preferredId = PendingKernelId ?? _selected?.KernelId;
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
                        ClearDetail("没有可用的运行时", "请安装或启用 Kernel 后重启 Core。");
                    }
                }
                else if (_selected != null)
                {
                    // 状态未变化时仅同步详情绑定对象，列表保持稳定。
                    var latest = runtimes.FirstOrDefault(r => r.KernelId == _selected.KernelId);
                    if (latest != null && !ReferenceEquals(latest, _selected))
                    {
                        _selected = latest;
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
                var connected = _backend.IsCoreConnected;
                RefreshRuntimesButton.IsEnabled = connected;
                RestartCoreButton.IsEnabled = connected && _backend.IsDaemonConnected;
            }
        }

        private async void RuntimeList_SelectionChanged(object sender, SelectionChangedEventArgs e)
        {
            if (RuntimeList.SelectedItem is not RuntimeInfo info)
                return;

            _selected = info;
            await ShowRuntimeDetailAsync(info);
        }

        private async Task ShowRuntimeDetailAsync(RuntimeInfo info, bool refreshSelection = false)
        {
            var requestId = ++_detailRequestId;

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

            if (requestId != _detailRequestId)
                return;

            await LoadSettingsAsync(info, requestId);
            if (requestId != _detailRequestId)
                return;

            await LoadOperationsAsync(info, requestId);
            if (requestId != _detailRequestId)
                return;

            await LoadRawStateAsync(info, requestId);
        }

        private async Task LoadRuntimeSelectionPanelAsync(RuntimeInfo info, int requestId, bool refresh)
        {
            RuntimeSelectionHost.Children.Clear();

            try
            {
                var panel = await _backend.GetRuntimeSelectionPanelAsync(info.KernelId, refresh);
                if (requestId != _detailRequestId)
                    return;

                if (panel.Pages.Count == 0)
                {
                    RuntimeSelectionHost.Children.Add(new TextBlock
                    {
                        Text = "未提供运行时选择信息。",
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
                    Foreground = FindResource("TextFillColorPrimaryBrush") as Brush,
                    Background = FindResource("ControlFillColorDefaultBrush") as Brush
                };

                ComboBoxItem? selected = null;
                var current = ValueToString(setting.Value);
                foreach (var option in selectEditor.Options)
                {
                    var item = new ComboBoxItem
                    {
                        Content = option.Text,
                        Tag = option.Value,
                        Foreground = FindResource("TextFillColorPrimaryBrush") as Brush,
                        Background = FindResource("ControlFillColorDefaultBrush") as Brush
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
            if (!string.IsNullOrWhiteSpace(current) && System.IO.Directory.Exists(current))
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
                    Foreground = FindResource("TextFillColorPrimaryBrush") as Brush,
                    Background = FindResource("ControlFillColorDefaultBrush") as Brush
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
                var result = await _backend.SelectRuntimeAsync(info.KernelId, home);
                if (!result.Ok)
                    throw new InvalidOperationException("Core 未确认运行时选择。");

                await LoadRuntimeSelectionPanelAsync(info, requestId, refresh: true);
                await LoadRuntimesAsync(silent: true);

                if (!result.RequiresRestart)
                {
                    (Window.GetWindow(this) as MainWindow)?.ShowToast(
                        "运行时选择未变化",
                        string.IsNullOrWhiteSpace(home) ? "自动探测" : home);
                    return;
                }

                RuntimeSelectionInfoBar.Message = "运行时选择已保存，重启 Core 后生效。";
                RuntimeSelectionInfoBar.IsOpen = true;

                if (Window.GetWindow(this) is not MainWindow window)
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

        private async Task LoadSettingsAsync(RuntimeInfo info, int requestId)
        {
            RuntimeSettingsHost.Children.Clear();
            RuntimeSettingsEmptyText.Visibility = Visibility.Collapsed;

            if (!info.SupportsControlPlane)
            {
                RuntimeSettingsEmptyText.Text = "该 Kernel 未实现 Runtime Control Plane。";
                RuntimeSettingsEmptyText.Visibility = Visibility.Visible;
                return;
            }

            try
            {
                var panel = await _backend.GetRuntimeSettingsPanelAsync(info.KernelId);
                if (requestId != _detailRequestId)
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
                    RuntimeSettingsEmptyText.Text = "该 Kernel 未提供运行时设置。";
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

                var text = BuildSettingLabel(setting, label);
                grid.Children.Add(text);

                var toggle = new ToggleSwitch
                {
                    IsChecked = ValueToBool(setting.Value),
                    VerticalAlignment = VerticalAlignment.Center
                };
                Grid.SetColumn(toggle, 1);
                toggle.Checked += async (_, _) => await ApplySettingAsync(setting.Key, true);
                toggle.Unchecked += async (_, _) => await ApplySettingAsync(setting.Key, false);
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
                    Foreground = FindResource("TextFillColorPrimaryBrush") as Brush,
                    Background = FindResource("ControlFillColorDefaultBrush") as Brush
                };
                var current = ValueToString(setting.Value);
                foreach (var option in selectEditor.Options)
                {
                    var item = new ComboBoxItem
                    {
                        Content = option.Text,
                        Tag = option.Value,
                        Foreground = FindResource("TextFillColorPrimaryBrush") as Brush,
                        Background = FindResource("ControlFillColorDefaultBrush") as Brush
                    };
                    combo.Items.Add(item);
                    if (string.Equals(ValueToString(option.Value), current, StringComparison.Ordinal))
                        combo.SelectedItem = item;
                }
                if (combo.SelectedItem == null && combo.Items.Count > 0)
                    combo.SelectedIndex = 0;

                combo.SelectionChanged += async (_, _) =>
                {
                    if (combo.SelectedItem is ComboBoxItem selected && selected.Tag != null)
                        await ApplySettingAsync(setting.Key, selected.Tag);
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
                    apply.Click += async (_, _) => await ApplySettingAsync(setting.Key, input.Text);
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
                Foreground = FindResource("TextFillColorPrimaryBrush") as Brush,
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

        private async Task ApplySettingAsync(string key, object value)
        {
            if (_selected == null)
                return;

            try
            {
                var result = await _backend.SetRuntimeSettingAsync(_selected.KernelId, key, value);
                if (!result.Ok)
                {
                    throw new InvalidOperationException(string.IsNullOrWhiteSpace(result.Error)
                        ? "Kernel 拒绝了该设置。"
                        : result.Error);
                }

                (Window.GetWindow(this) as MainWindow)?.ShowToast("运行时设置已应用", key);
                await LoadRawStateAsync(_selected, _detailRequestId);
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("更新运行时设置失败", ex);
                await LoadSettingsAsync(_selected, _detailRequestId);
            }
        }

        private async Task LoadOperationsAsync(RuntimeInfo info, int requestId)
        {
            RuntimeOperationsHost.Children.Clear();
            RuntimeOperationsEmptyText.Visibility = Visibility.Collapsed;

            if (!info.SupportsControlPlane)
            {
                RuntimeOperationsEmptyText.Text = "该 Kernel 未声明运行时操作。";
                RuntimeOperationsEmptyText.Visibility = Visibility.Visible;
                return;
            }

            try
            {
                var operations = await _backend.GetRuntimeOperationsAsync(info.KernelId);
                if (requestId != _detailRequestId)
                    return;

                if (operations.Count == 0)
                {
                    RuntimeOperationsEmptyText.Text = "该 Kernel 未声明运行时操作。";
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
                    button.Click += async (_, _) => await InvokeOperationAsync(operation, button);
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

        private async Task InvokeOperationAsync(RuntimeOperation operation, Wpf.Ui.Controls.Button button)
        {
            if (_selected == null)
                return;

            try
            {
                button.IsEnabled = false;
                var result = await _backend.InvokeRuntimeOperationAsync(_selected.KernelId, operation.Id);
                if (result.ValueKind == JsonValueKind.Object &&
                    result.TryGetProperty("ok", out var okProp) &&
                    okProp.ValueKind == JsonValueKind.False)
                {
                    var error = result.TryGetProperty("error", out var errorProp)
                        ? errorProp.GetString()
                        : null;
                    throw new InvalidOperationException(string.IsNullOrWhiteSpace(error)
                        ? "Kernel 返回失败结果。"
                        : error);
                }

                (Window.GetWindow(this) as MainWindow)?.ShowToast("操作完成", operation.Name);
                await LoadRawStateAsync(_selected, _detailRequestId);
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

        private async Task LoadRawStateAsync(RuntimeInfo info, int requestId)
        {
            if (!info.SupportsControlPlane)
            {
                RuntimeStateTextBox.Text = "该 Kernel 未实现 Runtime Control Plane。";
                return;
            }

            try
            {
                var state = await _backend.GetRuntimeStateAsync(info.KernelId);
                if (requestId != _detailRequestId)
                    return;

                RuntimeStateTextBox.Text = JsonSerializer.Serialize(state, new JsonSerializerOptions
                {
                    WriteIndented = true
                });
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
            if (Window.GetWindow(this) is not MainWindow window)
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
            if (Window.GetWindow(this) is not MainWindow window)
                return;

            try
            {
                RestartCoreButton.IsEnabled = false;
                var ok = await _backend.RestartCoreAsync();
                if (!ok)
                    throw new InvalidOperationException("Daemon 未确认 Core 重启。");

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
                RestartCoreButton.IsEnabled = _backend.IsDaemonConnected;
            }
        }

        private void SetDisconnectedState()
        {
            _runtimes.Clear();
            _selected = null;
            RuntimeList.ItemsSource = null;
            RuntimeListEmptyState.Visibility = Visibility.Visible;
            RuntimeListStatusText.Text = "Core 未连接，恢复连接后会自动刷新。";
            RefreshRuntimesButton.IsEnabled = false;
            RestartCoreButton.IsEnabled = false;
            ClearDetail("Core 未连接", "恢复连接后会自动加载运行时信息。");
        }

        private void ClearDetail(string title, string detail)
        {
            _detailRequestId++;
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

        private Brush SecondaryBrush =>
            FindResource("TextFillColorSecondaryBrush") as Brush ?? Brushes.Gray;

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
    }
}
