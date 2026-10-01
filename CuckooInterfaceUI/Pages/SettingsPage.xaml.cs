using System;
using System.Collections.Generic;
using System.Linq;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Controls.Primitives;
using System.Windows.Input;
using System.Windows.Media;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;
using Wpf.Ui.Controls;
using TextBox = System.Windows.Controls.TextBox;
using TextBlock = System.Windows.Controls.TextBlock;
using ComboBox = System.Windows.Controls.ComboBox;
using ComboBoxItem = System.Windows.Controls.ComboBoxItem;
using Slider = System.Windows.Controls.Slider;
using Border = System.Windows.Controls.Border;
using UniformGrid = System.Windows.Controls.Primitives.UniformGrid;
using Card = Wpf.Ui.Controls.Card;
using ToggleSwitch = Wpf.Ui.Controls.ToggleSwitch;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// SettingsPage.xaml 的交互逻辑。
    /// 设置项完全由后端 SettingPanel 驱动动态渲染，前端不硬编码任何设置。
    /// </summary>
    public partial class SettingsPage : Page
    {
        private readonly MockBackend _backend = MockBackend.Instance;

        /// <summary>
        /// 记录每个设置项对应的输入控件，便于保存时取值。
        /// key = setting.Key
        /// </summary>
        private readonly Dictionary<string, FrameworkElement> _controlMap = new();

        /// <summary>
        /// 记录颜色选择器当前选中的颜色值（key = setting.Key）。
        /// </summary>
        private readonly Dictionary<string, string> _colorValues = new();

        public SettingsPage()
        {
            InitializeComponent();
            BuildFromBackend();
        }

        // ===== 从后端加载并渲染设置面板 =====

        private void BuildFromBackend()
        {
            SettingsTabs.Items.Clear();

            // 1. 固定外观页面（始终作为第一个 Tab，结构由前端定义）
            var appearancePage = BuildAppearancePage();
            SettingsTabs.Items.Add(BuildTabItem(appearancePage));

            // 2. 后端动态下发的其他页面
            var panel = _backend.GetSettingsPanel();
            foreach (var page in panel.Pages)
            {
                SettingsTabs.Items.Add(BuildTabItem(page));
            }
        }

        /// <summary>
        /// 构建固定"外观"页面。结构硬编码在前端，值从后端获取。
        /// </summary>
        private SettingPage BuildAppearancePage()
        {
            var values = _backend.GetAppearanceSettings();

            object Get(string key, object fallback) =>
                values.TryGetValue(key, out var v) ? v : fallback;

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
                        Key = "fontScale", Name = "字号缩放",
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

        /// <summary>
        /// 将 SettingPage 转换为带 ScrollViewer 的 TabItem。
        /// </summary>
        private TabItem BuildTabItem(SettingPage page)
        {
            var tabItem = new TabItem
            {
                Header = page.Name,
                Tag = page.Key
            };

            var scrollViewer = new ScrollViewer
            {
                VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
                HorizontalScrollBarVisibility = ScrollBarVisibility.Disabled,
                Padding = new Thickness(0, 8, 0, 0)
            };
            // 启用平滑滚动行为（替代直接跳转，滚轮逐帧插值过渡）
            Behaviors.SmoothScrollBehavior.SetIsEnabled(scrollViewer, true);

            var stack = new StackPanel { Margin = new Thickness(0, 0, 0, 16) };

            foreach (var section in page.Sections)
            {
                stack.Children.Add(BuildSectionCard(section));
            }

            scrollViewer.Content = stack;
            tabItem.Content = scrollViewer;
            return tabItem;
        }

        // ===== 构建分区卡片 =====

        private Card BuildSectionCard(SettingSection section)
        {
            var card = new Card
            {
                Margin = new Thickness(0, 0, 0, 16),
                Padding = new Thickness(20, 16, 20, 16)
            };

            var stack = new StackPanel();

            // 分区标题
            var header = new TextBlock
            {
                Text = section.Name,
                FontSize = 16,
                FontWeight = FontWeights.SemiBold,
                Foreground = new SolidColorBrush(Colors.White),
                Margin = new Thickness(0, 0, 0, 12)
            };
            stack.Children.Add(header);

            // 分区内的设置项
            foreach (var setting in section.Settings)
            {
                var row = BuildSettingRow(setting);
                stack.Children.Add(row);
            }

            card.Content = stack;
            return card;
        }

        // ===== 构建设置项行（按 Editor 类型分发） =====

        private FrameworkElement BuildSettingRow(Setting setting)
        {
            switch (setting.Editor)
            {
                case SwitchEditor:
                    return BuildSwitchRow(setting);
                case SelectEditor:
                    return BuildSelectRow(setting);
                case SliderEditor:
                    return BuildSliderRow(setting);
                case TextEditor:
                    return BuildTextRow(setting);
                case ColorEditor:
                    return BuildColorRow(setting);
                default:
                    return new TextBlock { Text = $"未知编辑器类型: {setting.Editor.Type}" };
            }
        }

        // ---- Switch（ToggleSwitch） ----

        private FrameworkElement BuildSwitchRow(Setting setting)
        {
            var toggle = new ToggleSwitch
            {
                Tag = setting.Key,
                IsChecked = Convert.ToBoolean(setting.Value),
                VerticalAlignment = VerticalAlignment.Center,
                HorizontalAlignment = HorizontalAlignment.Right
            };
            _controlMap[setting.Key] = toggle;

            var dock = new DockPanel { Margin = new Thickness(0, 8, 0, 8) };
            var textStack = new StackPanel { VerticalAlignment = VerticalAlignment.Center };
            textStack.Children.Add(new TextBlock
            {
                Text = setting.DisplayName ? setting.Name : setting.Key,
                Foreground = new SolidColorBrush(Colors.White),
                FontSize = 14
            });
            if (setting.DisplayDescription && !string.IsNullOrEmpty(setting.Description))
            {
                textStack.Children.Add(new TextBlock
                {
                    Text = setting.Description,
                    Foreground = FindResource("TextFillColorSecondaryBrush") as Brush,
                    FontSize = 12,
                    Margin = new Thickness(0, 2, 0, 0),
                    TextWrapping = TextWrapping.Wrap
                });
            }
            dock.Children.Add(textStack);
            dock.Children.Add(toggle);
            return dock;
        }

        // ---- Select（ComboBox） ----

        private FrameworkElement BuildSelectRow(Setting setting)
        {
            var editor = (SelectEditor)setting.Editor;
            var combo = new ComboBox
            {
                Tag = setting.Key,
                Margin = new Thickness(0, 8, 0, 8),
                MinWidth = 160,
                HorizontalAlignment = HorizontalAlignment.Left
            };
            foreach (var opt in editor.Options)
            {
                var item = new ComboBoxItem { Content = opt.Text, Tag = opt.Value };
                combo.Items.Add(item);
                if (opt.Value != null && opt.Value.Equals(setting.Value))
                {
                    combo.SelectedItem = item;
                }
            }
            _controlMap[setting.Key] = combo;

            return BuildLabeledStack(setting, combo);
        }

        // ---- Slider ----

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
                Margin = new Thickness(0, 8, 0, 0)
            };
            _controlMap[setting.Key] = slider;

            // 顶部：标题 + 当前值
            var header = new DockPanel { Margin = new Thickness(0, 8, 0, 0) };
            var nameText = new TextBlock
            {
                Text = setting.DisplayName ? setting.Name : setting.Key,
                Foreground = new SolidColorBrush(Colors.White),
                FontSize = 14
            };
            var valueText = new TextBlock
            {
                Text = $"{slider.Value}{editor.Unit}",
                Foreground = FindResource("TextFillColorSecondaryBrush") as Brush,
                FontSize = 12,
                HorizontalAlignment = HorizontalAlignment.Right,
                Margin = new Thickness(16, 0, 0, 0)
            };
            header.Children.Add(nameText);
            header.Children.Add(valueText);

            slider.ValueChanged += (_, _) =>
            {
                valueText.Text = $"{slider.Value:0.#}{editor.Unit}";
            };

            var stack = new StackPanel { Margin = new Thickness(0, 8, 0, 8) };
            stack.Children.Add(header);
            stack.Children.Add(slider);
            return stack;
        }

        // ---- Text（TextBox） ----

        private FrameworkElement BuildTextRow(Setting setting)
        {
            var editor = (TextEditor)setting.Editor;
            var textBox = new TextBox
            {
                Tag = setting.Key,
                Text = setting.Value?.ToString() ?? string.Empty,
                IsReadOnly = editor.ReadOnly,
                Margin = new Thickness(0, 8, 0, 8),
                MinHeight = 32
            };
            if (editor.MultiLine)
            {
                textBox.AcceptsReturn = true;
                textBox.TextWrapping = TextWrapping.Wrap;
                textBox.Height = 80;
            }
            _controlMap[setting.Key] = textBox;

            var dock = new DockPanel { Margin = new Thickness(0, 8, 0, 8) };
            var content = new StackPanel();
            content.Children.Add(BuildLabel(setting));
            content.Children.Add(textBox);
            dock.Children.Add(content);

            // Action 按钮（如"浏览"）
            if (setting.Actions != null && setting.Actions.Count > 0)
            {
                var actionPanel = new StackPanel
                {
                    Orientation = Orientation.Horizontal,
                    Margin = new Thickness(0, 4, 0, 0)
                };
                foreach (var action in setting.Actions)
                {
                    var btn = new Wpf.Ui.Controls.Button
                    {
                        Content = action.Text,
                        Appearance = ControlAppearance.Secondary,
                        Margin = new Thickness(0, 0, 8, 0),
                        Tag = action
                    };
                    btn.Click += (_, _) => HandleAction(action, textBox);
                    actionPanel.Children.Add(btn);
                }
                content.Children.Add(actionPanel);
            }

            return dock;
        }

        // ---- Color（颜色选择器） ----

        private FrameworkElement BuildColorRow(Setting setting)
        {
            var editor = (ColorEditor)setting.Editor;
            _colorValues[setting.Key] = setting.Value?.ToString() ?? string.Empty;

            var grid = new UniformGrid
            {
                Columns = editor.Columns > 0 ? editor.Columns : 5,
                Margin = new Thickness(0, 8, 0, 8)
            };

            foreach (var colorHex in editor.Colors)
            {
                var btn = new Border
                {
                    Width = 36,
                    Height = 36,
                    CornerRadius = new CornerRadius(18),
                    Background = ParseColorBrush(colorHex),
                    Margin = new Thickness(4),
                    Cursor = System.Windows.Input.Cursors.Hand,
                    Tag = colorHex
                };
                btn.MouseLeftButtonUp += (_, _) =>
                {
                    _colorValues[setting.Key] = colorHex;
                    UpdateColorSelection(grid, colorHex);
                };
                grid.Children.Add(btn);
            }

            UpdateColorSelection(grid, _colorValues[setting.Key]);

            var stack = new StackPanel { Margin = new Thickness(0, 8, 0, 8) };
            stack.Children.Add(BuildLabel(setting));
            stack.Children.Add(grid);
            return stack;
        }

        // ===== 辅助方法 =====

        private FrameworkElement BuildLabel(Setting setting)
        {
            var stack = new StackPanel { Margin = new Thickness(0, 0, 0, 4) };
            stack.Children.Add(new TextBlock
            {
                Text = setting.DisplayName ? setting.Name : setting.Key,
                Foreground = new SolidColorBrush(Colors.White),
                FontSize = 14
            });
            if (setting.DisplayDescription && !string.IsNullOrEmpty(setting.Description))
            {
                stack.Children.Add(new TextBlock
                {
                    Text = setting.Description,
                    Foreground = FindResource("TextFillColorSecondaryBrush") as Brush,
                    FontSize = 12,
                    Margin = new Thickness(0, 2, 0, 0),
                    TextWrapping = TextWrapping.Wrap
                });
            }
            return stack;
        }

        private FrameworkElement BuildLabeledStack(Setting setting, FrameworkElement control)
        {
            var stack = new StackPanel { Margin = new Thickness(0, 8, 0, 8) };
            stack.Children.Add(BuildLabel(setting));
            stack.Children.Add(control);
            return stack;
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

        private void UpdateColorSelection(UniformGrid grid, string selectedHex)
        {
            foreach (Border child in grid.Children)
            {
                if (child.Tag is string hex && hex == selectedHex)
                {
                    child.BorderBrush = new SolidColorBrush(Colors.White);
                    child.BorderThickness = new Thickness(3);
                }
                else
                {
                    child.BorderThickness = new Thickness(0);
                }
            }
        }

        // ===== Action 处理（浏览目录/文件/重置） =====

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
                        {
                            target.Text = dialog.SelectedPath;
                        }
                    }
                    break;

                case ActionType.BrowseFile:
                    var openDlg = new Microsoft.Win32.OpenFileDialog();
                    if (openDlg.ShowDialog() == true)
                    {
                        target.Text = openDlg.FileName;
                    }
                    break;

                case ActionType.Reset:
                    var setting = _backend.GetSettingsPanel().Pages
                        .SelectMany(p => p.Sections)
                        .SelectMany(s => s.Settings)
                        .FirstOrDefault(st => st.Actions != null && st.Actions.Any(a => a.Key == action.Key));
                    if (setting != null)
                    {
                        target.Text = setting.DefaultValue?.ToString() ?? string.Empty;
                    }
                    break;
            }
        }

        // ===== 保存设置 =====

        private async void SaveSettings_Click(object sender, RoutedEventArgs e)
        {
            var values = new Dictionary<string, object>();

            foreach (var kv in _controlMap)
            {
                var key = kv.Key;
                var control = kv.Value;

                switch (control)
                {
                    case ToggleSwitch ts:
                        values[key] = ts.IsChecked == true;
                        break;
                    case ComboBox cb:
                        if (cb.SelectedItem is ComboBoxItem item && item.Tag != null)
                        {
                            values[key] = item.Tag;
                        }
                        break;
                    case Slider sl:
                        values[key] = sl.Value;
                        break;
                    case TextBox tb:
                        values[key] = tb.Text;
                        break;
                }
            }

            // 颜色值
            foreach (var kv in _colorValues)
            {
                values[kv.Key] = kv.Value;
            }

            _backend.SaveSettings(values);

            await new Wpf.Ui.Controls.MessageBox
            {
                Title = "设置已保存",
                Content = "所有设置项已成功保存到后端。",
                PrimaryButtonText = "确定"
            }.ShowDialogAsync();
        }

        private void TabControl_SelectionChanged(object sender, SelectionChangedEventArgs e)
        {
            // 选项卡切换时可在此触发动画或懒加载
        }
    }
}
