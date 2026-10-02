using System;
using System.Collections.Generic;
using System.Linq;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Controls.Primitives;
using System.Windows.Media;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;
using Wpf.Ui.Controls;
using TextBox = Wpf.Ui.Controls.TextBox;
using TextBlock = Wpf.Ui.Controls.TextBlock;
using ComboBox = System.Windows.Controls.ComboBox;
using ComboBoxItem = System.Windows.Controls.ComboBoxItem;
using Slider = System.Windows.Controls.Slider;
using UniformGrid = System.Windows.Controls.Primitives.UniformGrid;
using ToggleSwitch = Wpf.Ui.Controls.ToggleSwitch;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// 设置页面：左侧导航，右侧轻量设置行。设置结构由后端动态驱动。
    /// </summary>
    public partial class SettingsPage : Page
    {
        private readonly CoreBackend _backend = CoreBackend.Instance;
        private SettingPanel _settingsPanel = new();
        private readonly Dictionary<string, FrameworkElement> _controlMap = new();
        private readonly Dictionary<string, string> _colorValues = new();
        private readonly Dictionary<string, FrameworkElement> _pageViews = new();
        private List<SettingPage> _pages = new();
        private bool _suppressDirtyTracking;
        private bool _hasPendingChanges;

        public SettingsPage()
        {
            InitializeComponent();
            Loaded += SettingsPage_Loaded;
            Unloaded += (_, _) => _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
        }

        private async void SettingsPage_Loaded(object sender, RoutedEventArgs e)
        {
            _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
            _backend.ConnectionStateChanged += Backend_ConnectionStateChanged;
            await BuildFromBackendAsync();
        }

        private void Backend_ConnectionStateChanged(object? sender, BackendConnectionStateChangedEventArgs e)
        {
            if (!e.IsCore || !IsVisible) return;
            _ = Dispatcher.InvokeAsync(async () =>
            {
                if (e.IsConnected)
                    await BuildFromBackendAsync();
                else
                    SetDisconnectedState();
            });
        }

        private async System.Threading.Tasks.Task BuildFromBackendAsync()
        {
            _suppressDirtyTracking = true;
            SetPendingChanges(false);

            var selectedKey = SettingsNav.SelectedItem is SettingPage selectedPage ? selectedPage.Key : null;

            try
            {
                if (!_backend.IsCoreConnected)
                {
                    SetDisconnectedState();
                    return;
                }

                var appearancePage = await BuildAppearancePageAsync();
                var nextPanel = await _backend.GetSettingsPanelAsync();
                var pages = new List<SettingPage> { appearancePage };
                pages.AddRange(nextPanel.Pages);

                _controlMap.Clear();
                _colorValues.Clear();
                _pageViews.Clear();

                foreach (var page in pages)
                    _pageViews[page.Key] = BuildPageView(page);

                _settingsPanel = nextPanel;
                _pages = pages;
                SettingsNav.ItemsSource = null;
                SettingsNav.ItemsSource = _pages;
                SettingsNav.SelectedItem = _pages.FirstOrDefault(p => p.Key == selectedKey) ?? _pages.FirstOrDefault();
                SettingsDisconnectedState.Visibility = Visibility.Collapsed;
                SettingsNavEmpty.Visibility = _pages.Count == 0 ? Visibility.Visible : Visibility.Collapsed;
            }
            catch (Exception ex)
            {
                if (ex is BackendConnectionException) SetDisconnectedState();
                await ShowBackendErrorAsync("加载设置失败", ex);
            }
            finally
            {
                _suppressDirtyTracking = false;
                SaveSettingsButton.IsEnabled = _backend.IsCoreConnected && _hasPendingChanges;
            }
        }

        private async System.Threading.Tasks.Task<SettingPage> BuildAppearancePageAsync()
        {
            var values = await _backend.GetAppearanceSettingsAsync();
            object Get(string key, object fallback) => values.TryGetValue(key, out var v) ? v : fallback;

            return new SettingPage
            {
                Key = "appearance",
                Name = "外观",
                Sections = new List<SettingSection>
                {
                    new SettingSection
                    {
                        Key = "theme", Name = "主题",
                        Settings = new List<Setting>
                        {
                            new Setting
                            {
                                Key = "theme", Name = "应用主题", Description = "选择浅色或深色模式",
                                DisplayName = true, DisplayDescription = true,
                                Editor = new SelectEditor { Options = new List<TextOption>
                                {
                                    new() { Value = "light", Text = "浅色" },
                                    new() { Value = "dark", Text = "深色" },
                                    new() { Value = "system", Text = "跟随系统" }
                                }},
                                Value = Get("theme", "system"),
                                DefaultValue = "system"
                            }
                        }
                    },
                    new SettingSection
                    {
                        Key = "accentColor", Name = "强调色",
                        Settings = new List<Setting>
                        {
                            new Setting
                            {
                                Key = "accentColor", Name = "强调色", Description = "选择界面的主要强调颜色",
                                DisplayName = true, DisplayDescription = false,
                                Editor = new ColorEditor
                                {
                                    Colors = new List<string> { "#0078D4", "#E81123", "#107C10", "#FFB900", "#881798" },
                                    Columns = 5
                                },
                                Value = Get("accentColor", "#0078D4"),
                                DefaultValue = "#0078D4"
                            }
                        }
                    },
                    new SettingSection
                    {
                        Key = "fontScale", Name = "字号",
                        Settings = new List<Setting>
                        {
                            new Setting
                            {
                                Key = "fontScale", Name = "字号缩放", Description = "调整界面字体大小比例",
                                DisplayName = true, DisplayDescription = false,
                                Editor = new SliderEditor { Min = 50, Max = 200, Step = 5, Unit = "%" },
                                Value = Convert.ToDouble(Get("fontScale", 100.0)),
                                DefaultValue = 100.0
                            }
                        }
                    }
                }
            };
        }

        private FrameworkElement BuildPageView(SettingPage page)
        {
            var scroll = new ScrollViewer
            {
                VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
                HorizontalScrollBarVisibility = ScrollBarVisibility.Disabled
            };

            var root = new StackPanel { Margin = new Thickness(0, 0, 8, 16) };
            foreach (var section in page.Sections)
                root.Children.Add(BuildSection(section));
            scroll.Content = root;
            return scroll;
        }

        private FrameworkElement BuildSection(SettingSection section)
        {
            var stack = new StackPanel { Margin = new Thickness(0, 0, 0, 22) };
            stack.Children.Add(new TextBlock
            {
                Text = section.Name,
                FontSize = 15,
                FontWeight = FontWeights.SemiBold,
                Margin = new Thickness(0, 0, 0, 4)
            });

            foreach (var setting in section.Settings)
            {
                var row = BuildSettingRow(setting);
                stack.Children.Add(new Border
                {
                    Padding = new Thickness(0, 11, 0, 11),
                    BorderBrush = FindResource("ControlStrokeColorDefaultBrush") as Brush,
                    BorderThickness = new Thickness(0, 1, 0, 0),
                    Child = row
                });
            }
            return stack;
        }

        private FrameworkElement BuildSettingRow(Setting setting)
        {
            return setting.Editor switch
            {
                SwitchEditor => BuildSwitchRow(setting),
                SelectEditor => BuildSelectRow(setting),
                SliderEditor => BuildSliderRow(setting),
                TextEditor => BuildTextRow(setting),
                ColorEditor => BuildColorRow(setting),
                _ => new TextBlock { Text = $"未知编辑器类型: {setting.Editor.Type}" }
            };
        }

        private FrameworkElement BuildSwitchRow(Setting setting)
        {
            var toggle = new ToggleSwitch
            {
                Tag = setting.Key,
                IsChecked = Convert.ToBoolean(setting.Value),
                HorizontalAlignment = HorizontalAlignment.Right,
                VerticalAlignment = VerticalAlignment.Center
            };
            _controlMap[setting.Key] = toggle;
            toggle.Checked += (_, _) => MarkSettingsDirty();
            toggle.Unchecked += (_, _) => MarkSettingsDirty();

            var grid = CreateTwoColumnRow(setting);
            Grid.SetColumn(toggle, 1);
            grid.Children.Add(toggle);
            return grid;
        }

        private FrameworkElement BuildSelectRow(Setting setting)
        {
            var editor = (SelectEditor)setting.Editor;
            var combo = new ComboBox
            {
                Tag = setting.Key,
                MinWidth = 180,
                HorizontalAlignment = HorizontalAlignment.Right,
                Foreground = FindResource("TextFillColorPrimaryBrush") as Brush,
                Background = FindResource("ControlFillColorDefaultBrush") as Brush
            };
            foreach (var option in editor.Options)
            {
                var item = new ComboBoxItem { Content = option.Text, Tag = option.Value, Foreground = FindResource("TextFillColorPrimaryBrush") as Brush, Background = FindResource("ControlFillColorDefaultBrush") as Brush };
                combo.Items.Add(item);
                if (option.Value?.ToString() == setting.Value?.ToString())
                    combo.SelectedItem = item;
            }
            _controlMap[setting.Key] = combo;
            combo.SelectionChanged += (_, _) => MarkSettingsDirty();

            var grid = CreateTwoColumnRow(setting);
            Grid.SetColumn(combo, 1);
            grid.Children.Add(combo);
            return grid;
        }

        private FrameworkElement BuildSliderRow(Setting setting)
        {
            var editor = (SliderEditor)setting.Editor;
            var slider = new Slider
            {
                Tag = setting.Key,
                Minimum = editor.Min,
                Maximum = editor.Max,
                TickFrequency = editor.Step,
                IsSnapToTickEnabled = true,
                Value = Convert.ToDouble(setting.Value),
                MinWidth = 220,
                HorizontalAlignment = HorizontalAlignment.Right
            };
            _controlMap[setting.Key] = slider;
            slider.ValueChanged += (_, _) => MarkSettingsDirty();

            var valueText = new TextBlock
            {
                Text = $"{slider.Value:0.##}{editor.Unit}",
                FontSize = 12,
                Foreground = FindResource("TextFillColorSecondaryBrush") as Brush,
                VerticalAlignment = VerticalAlignment.Center,
                Margin = new Thickness(8, 0, 0, 0)
            };
            slider.ValueChanged += (_, _) => valueText.Text = $"{slider.Value:0.##}{editor.Unit}";

            var right = new StackPanel { Orientation = Orientation.Horizontal, HorizontalAlignment = HorizontalAlignment.Right };
            right.Children.Add(slider);
            right.Children.Add(valueText);

            var grid = CreateTwoColumnRow(setting);
            Grid.SetColumn(right, 1);
            grid.Children.Add(right);
            return grid;
        }

        private FrameworkElement BuildTextRow(Setting setting)
        {
            var editor = (TextEditor)setting.Editor;
            var textBox = new TextBox
            {
                Tag = setting.Key,
                Foreground = FindResource("TextFillColorPrimaryBrush") as Brush,
                Background = FindResource("ControlFillColorDefaultBrush") as Brush,
                Text = setting.Value?.ToString() ?? string.Empty,
                IsReadOnly = editor.ReadOnly,
                MinHeight = editor.MultiLine ? 90 : 32,
                AcceptsReturn = editor.MultiLine,
                TextWrapping = editor.MultiLine ? TextWrapping.Wrap : TextWrapping.NoWrap,
                VerticalScrollBarVisibility = editor.MultiLine ? ScrollBarVisibility.Auto : ScrollBarVisibility.Disabled,
                ToolTip = string.IsNullOrWhiteSpace(editor.Placeholder) ? null : editor.Placeholder
            };
            _controlMap[setting.Key] = textBox;
            textBox.TextChanged += (_, _) => MarkSettingsDirty();

            var stack = new StackPanel();
            stack.Children.Add(BuildLabel(setting));
            stack.Children.Add(textBox);
            AddActionButtons(stack, setting, textBox);
            return stack;
        }

        private FrameworkElement BuildColorRow(Setting setting)
        {
            var editor = (ColorEditor)setting.Editor;
            _colorValues[setting.Key] = setting.Value?.ToString() ?? string.Empty;

            var grid = new UniformGrid
            {
                Columns = editor.Columns > 0 ? editor.Columns : Math.Max(1, editor.Colors.Count),
                HorizontalAlignment = HorizontalAlignment.Right,
                Width = Math.Max(160, Math.Min(280, (editor.Columns > 0 ? editor.Columns : editor.Colors.Count) * 44)),
                Margin = new Thickness(0, 2, 0, 0)
            };

            foreach (var colorHex in editor.Colors)
            {
                var swatch = new Wpf.Ui.Controls.Button
                {
                    Width = 32,
                    Height = 32,
                    Padding = new Thickness(0),
                    Margin = new Thickness(4),
                    Appearance = ControlAppearance.Transparent,
                    Background = ParseColorBrush(colorHex),
                    BorderBrush = Brushes.Transparent,
                    BorderThickness = new Thickness(2),
                    ToolTip = colorHex,
                    Tag = colorHex
                };
                swatch.Click += (_, _) =>
                {
                    _colorValues[setting.Key] = colorHex;
                    MarkSettingsDirty();
                    UpdateColorSelection(grid, colorHex);
                };
                grid.Children.Add(swatch);
            }
            UpdateColorSelection(grid, _colorValues[setting.Key]);

            var row = CreateTwoColumnRow(setting);
            Grid.SetColumn(grid, 1);
            row.Children.Add(grid);
            return row;
        }

        private Grid CreateTwoColumnRow(Setting setting)
        {
            var grid = new Grid { MinHeight = 42 };
            grid.ColumnDefinitions.Add(new ColumnDefinition { Width = new GridLength(1, GridUnitType.Star) });
            grid.ColumnDefinitions.Add(new ColumnDefinition { Width = GridLength.Auto });

            var label = BuildLabel(setting);
            Grid.SetColumn(label, 0);
            grid.Children.Add(label);
            return grid;
        }

        private FrameworkElement BuildLabel(Setting setting)
        {
            var stack = new StackPanel { VerticalAlignment = VerticalAlignment.Center, Margin = new Thickness(0, 0, 18, 0) };
            stack.Children.Add(new TextBlock
            {
                Text = setting.DisplayName ? setting.Name : setting.Key,
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
                    Foreground = FindResource("TextFillColorSecondaryBrush") as Brush,
                    Margin = new Thickness(0, 2, 0, 0),
                    TextWrapping = TextWrapping.Wrap
                });
            }
            return stack;
        }

        private void AddActionButtons(StackPanel host, Setting setting, TextBox textBox)
        {
            if (setting.Actions == null || setting.Actions.Count == 0) return;
            var panel = new StackPanel { Orientation = Orientation.Horizontal, Margin = new Thickness(0, 6, 0, 0) };
            foreach (var action in setting.Actions)
            {
                var button = new Wpf.Ui.Controls.Button
                {
                    Content = action.Text,
                    Appearance = ControlAppearance.Secondary,
                    Margin = new Thickness(0, 0, 8, 0)
                };
                button.Click += (_, _) => HandleAction(action, textBox);
                panel.Children.Add(button);
            }
            host.Children.Add(panel);
        }

        private static Brush ParseColorBrush(string hex)
        {
            try
            {
                var color = (Color)ColorConverter.ConvertFromString(hex);
                return new SolidColorBrush(color);
            }
            catch
            {
                return new SolidColorBrush(Colors.Gray);
            }
        }

        private static void UpdateColorSelection(UniformGrid grid, string selectedHex)
        {
            foreach (var child in grid.Children.OfType<Wpf.Ui.Controls.Button>())
            {
                var selected = child.Tag is string hex && hex == selectedHex;
                child.BorderBrush = selected
                    ? System.Windows.Media.Brushes.White
                    : System.Windows.Media.Brushes.Transparent;
                child.BorderThickness = selected
                    ? new Thickness(2)
                    : new Thickness(0);
            }
        }

        private void HandleAction(SettingAction action, TextBox target)
        {
            switch (action.Type)
            {
                case ActionType.BrowseDirectory:
                    using (var dialog = new System.Windows.Forms.FolderBrowserDialog
                    {
                        Description = "选择目录",
                        ShowNewFolderButton = true
                    })
                    {
                        if (!string.IsNullOrEmpty(target.Text))
                        {
                            try { dialog.SelectedPath = System.IO.Path.GetFullPath(target.Text); } catch { }
                        }
                        if (dialog.ShowDialog() == System.Windows.Forms.DialogResult.OK)
                            target.Text = dialog.SelectedPath;
                    }
                    break;

                case ActionType.BrowseFile:
                    var openDialog = new Microsoft.Win32.OpenFileDialog { CheckFileExists = true };
                    if (openDialog.ShowDialog() == true)
                        target.Text = openDialog.FileName;
                    break;

                case ActionType.Reset:
                    var setting = _settingsPanel.Pages
                        .SelectMany(p => p.Sections)
                        .SelectMany(s => s.Settings)
                        .FirstOrDefault(st => st.Actions != null && st.Actions.Any(a => a.Key == action.Key));
                    if (setting != null)
                        target.Text = setting.DefaultValue?.ToString() ?? string.Empty;
                    break;
            }
        }

        private void SettingsNav_SelectionChanged(object sender, SelectionChangedEventArgs e)
        {
            if (SettingsNav.SelectedItem is not SettingPage page) return;
            if (_pageViews.TryGetValue(page.Key, out var view))
                SettingsContent.Content = view;
        }

        private void MarkSettingsDirty()
        {
            if (_suppressDirtyTracking || !_backend.IsCoreConnected) return;
            SetPendingChanges(true);
        }

        private void SetPendingChanges(bool pending)
        {
            _hasPendingChanges = pending;
            if (UnsavedChangesText != null)
                UnsavedChangesText.Visibility = pending ? Visibility.Visible : Visibility.Collapsed;
            if (SaveSettingsButton != null)
                SaveSettingsButton.IsEnabled = _backend.IsCoreConnected && pending;
        }

        private async void SaveSettings_Click(object sender, RoutedEventArgs e)
        {
            if (!_backend.IsCoreConnected)
            {
                (Window.GetWindow(this) as MainWindow)?.ShowBackendUnavailable();
                return;
            }

            var values = new Dictionary<string, object>();
            foreach (var kv in _controlMap)
            {
                switch (kv.Value)
                {
                    case ToggleSwitch toggle:
                        values[kv.Key] = toggle.IsChecked == true;
                        break;
                    case ComboBox combo when combo.SelectedItem is ComboBoxItem item:
                        values[kv.Key] = item.Tag ?? string.Empty;
                        break;
                    case Slider slider:
                        values[kv.Key] = slider.Value;
                        break;
                    case TextBox textBox:
                        values[kv.Key] = textBox.Text;
                        break;
                }
            }
            foreach (var kv in _colorValues)
                values[kv.Key] = kv.Value;

            try
            {
                SaveSettingsButton.IsEnabled = false;
                var ok = await _backend.SaveSettingsAsync(values);
                if (!ok) throw new InvalidOperationException("Core 未确认设置保存成功。");
                SetPendingChanges(false);
                (Window.GetWindow(this) as MainWindow)?.ShowToast("设置已保存", "修改已经提交到 Core。");
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("保存设置失败", ex);
            }
            finally
            {
                SaveSettingsButton.IsEnabled = _backend.IsCoreConnected && _hasPendingChanges;
            }
        }

        private void SetDisconnectedState()
        {
            _pageViews.Clear();
            _pages.Clear();
            _controlMap.Clear();
            _colorValues.Clear();
            SettingsNav.ItemsSource = null;
            SettingsContent.Content = null;
            SettingsDisconnectedState.Visibility = Visibility.Visible;
            SettingsNavEmpty.Visibility = Visibility.Visible;
            SetPendingChanges(false);
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
    }
}

