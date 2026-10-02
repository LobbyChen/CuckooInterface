using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;
using Microsoft.Win32;
using Wpf.Ui.Controls;

namespace CuckooInterfaceUI.Pages
{
    public partial class PluginsPage : Page
    {
        private readonly CoreBackend _backend = CoreBackend.Instance;
        private List<PluginInfo> _allPlugins = new();

        public PluginsPage()
        {
            InitializeComponent();
            Loaded += PluginsPage_Loaded;
            Unloaded += (_, _) => _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
        }

        private async void PluginsPage_Loaded(object sender, RoutedEventArgs e)
        {
            _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
            _backend.ConnectionStateChanged += Backend_ConnectionStateChanged;
            await LoadPluginsFromBackendAsync();
        }

        private void Backend_ConnectionStateChanged(object? sender, BackendConnectionStateChangedEventArgs e)
        {
            if (!e.IsCore || !IsVisible)
                return;

            _ = Dispatcher.InvokeAsync(async () =>
            {
                if (e.IsConnected)
                    await LoadPluginsFromBackendAsync();
                else
                    SetDisconnectedState();
            });
        }

        private async Task LoadPluginsFromBackendAsync()
        {
            if (!_backend.IsCoreConnected)
            {
                SetDisconnectedState();
                return;
            }

            try
            {
                RefreshPluginsButton.IsEnabled = false;
                InstallPluginButton.IsEnabled = false;

                _allPlugins = await _backend.GetPluginsAsync();
                ApplyFilter();
            }
            catch (Exception ex)
            {
                if (ex is BackendConnectionException)
                    SetDisconnectedState();

                await ShowBackendErrorAsync("加载插件失败", ex);
            }
            finally
            {
                var connected = _backend.IsCoreConnected;
                RefreshPluginsButton.IsEnabled = connected;
                InstallPluginButton.IsEnabled = connected;
            }
        }

        private void ApplyFilter()
        {
            if (PluginsList == null || SearchBox == null || TypeFilter == null)
                return;

            string? typeFilter = null;
            if (TypeFilter.SelectedItem is ComboBoxItem item &&
                item.Tag is string tag &&
                !string.IsNullOrWhiteSpace(tag))
            {
                typeFilter = tag;
            }

            var search = (SearchBox.Text ?? string.Empty).Trim();

            var filtered = _allPlugins.Where(plugin =>
            {
                if (typeFilter != null && TypeToString(plugin.Type) != typeFilter)
                    return false;

                if (!string.IsNullOrWhiteSpace(search) &&
                    !plugin.Name.Contains(search, StringComparison.OrdinalIgnoreCase) &&
                    !plugin.Id.Contains(search, StringComparison.OrdinalIgnoreCase))
                {
                    return false;
                }

                return true;
            }).ToList();

            PluginsList.ItemsSource = filtered;
            UpdateEmptyState(false);
        }

        private static string TypeToString(PluginType type) => type switch
        {
            PluginType.Kernel => "kernel",
            PluginType.Base => "base",
            PluginType.Active => "active",
            _ => string.Empty
        };

        private void TypeFilter_SelectionChanged(object sender, SelectionChangedEventArgs e)
        {
            ApplyFilter();
        }

        private void SearchBox_TextChanged(object sender, TextChangedEventArgs e)
        {
            ApplyFilter();
        }

        private async void RefreshButton_Click(object sender, RoutedEventArgs e)
        {
            await LoadPluginsFromBackendAsync();
        }

        private async void PluginToggle_Changed(object sender, RoutedEventArgs e)
        {
            if (sender is not ToggleSwitch toggle ||
                toggle.Tag is not PluginInfo plugin)
            {
                return;
            }

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

                await LoadPluginsFromBackendAsync();

                (Window.GetWindow(this) as MainWindow)?.ShowToast(
                    targetEnabled ? "插件已启用" : "插件已禁用",
                    plugin.Name);
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("更新插件状态失败", ex);
                await LoadPluginsFromBackendAsync();
            }
            finally
            {
                toggle.IsEnabled = _backend.IsCoreConnected;
            }
        }

        private void PluginSettings_Click(object sender, RoutedEventArgs e)
        {
            NavigateTo(typeof(ConfigPage));
        }

        private async void DeletePlugin_Click(object sender, RoutedEventArgs e)
        {
            if (sender is not FrameworkElement element ||
                element.Tag is not PluginInfo plugin)
            {
                return;
            }

            if (Window.GetWindow(this) is not MainWindow window)
                return;

            var confirmed = await window.ShowConfirmDialogAsync(
                "卸载插件",
                $"确定要卸载“{plugin.Name}”吗？此操作不可撤销。",
                "卸载",
                danger: true);

            if (!confirmed)
                return;

            try
            {
                var ok = await _backend.RemovePluginAsync(plugin.Id);
                if (!ok)
                    throw new InvalidOperationException("Core 未确认插件卸载。");

                await LoadPluginsFromBackendAsync();

                (Window.GetWindow(this) as MainWindow)?.ShowToast(
                    "插件已卸载",
                    plugin.Name);
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("卸载插件失败", ex);
            }
        }

        private async void OpenInstallPanel_Click(object sender, RoutedEventArgs e)
        {
            if (!_backend.IsCoreConnected)
            {
                (Window.GetWindow(this) as MainWindow)?.ShowBackendUnavailable();
                return;
            }

            var host = (Window.GetWindow(this) as MainWindow)?.RootContentDialogHost;
            if (host == null)
                return;

            var primaryBrush = FindResource("TextFillColorPrimaryBrush") as System.Windows.Media.Brush;
            var controlBrush = FindResource("ControlFillColorDefaultBrush") as System.Windows.Media.Brush;

            var typeComboBox = new ComboBox
            {
                SelectedIndex = 0,
                MinWidth = 340,
                Margin = new Thickness(0, 0, 0, 18),
                Foreground = primaryBrush,
                Background = controlBrush
            };

            typeComboBox.Items.Add(new ComboBoxItem
            {
                Content = "内核",
                Tag = "kernel",
                Foreground = primaryBrush,
                Background = controlBrush
            });
            typeComboBox.Items.Add(new ComboBoxItem
            {
                Content = "基础",
                Tag = "base",
                Foreground = primaryBrush,
                Background = controlBrush
            });
            typeComboBox.Items.Add(new ComboBoxItem
            {
                Content = "应用",
                Tag = "active",
                Foreground = primaryBrush,
                Background = controlBrush
            });

            var pathBox = new Wpf.Ui.Controls.TextBox
            {
                PlaceholderText = "选择 .zip 插件包…",
                MinWidth = 260,
                Foreground = primaryBrush,
                Background = controlBrush
            };

            var browseButton = new Wpf.Ui.Controls.Button
            {
                Content = "浏览",
                Appearance = ControlAppearance.Secondary,
                Margin = new Thickness(10, 0, 0, 0)
            };

            browseButton.Click += (_, _) =>
            {
                var fileDialog = new OpenFileDialog
                {
                    Filter = "插件包 (*.zip)|*.zip|所有文件 (*.*)|*.*",
                    CheckFileExists = true,
                    Multiselect = false
                };

                if (fileDialog.ShowDialog() == true)
                    pathBox.Text = fileDialog.FileName;
            };

            var pathGrid = new Grid
            {
                Margin = new Thickness(0, 0, 0, 8)
            };
            pathGrid.ColumnDefinitions.Add(new ColumnDefinition
            {
                Width = new GridLength(1, GridUnitType.Star)
            });
            pathGrid.ColumnDefinitions.Add(new ColumnDefinition
            {
                Width = GridLength.Auto
            });

            Grid.SetColumn(pathBox, 0);
            Grid.SetColumn(browseButton, 1);
            pathGrid.Children.Add(pathBox);
            pathGrid.Children.Add(browseButton);

            var content = new StackPanel
            {
                MinWidth = 420
            };

            if (primaryBrush != null)
                System.Windows.Documents.TextElement.SetForeground(content, primaryBrush);

            content.Children.Add(new Wpf.Ui.Controls.TextBlock
            {
                Text = "插件类型",
                FontSize = 12,
                Margin = new Thickness(0, 0, 0, 6)
            });
            content.Children.Add(typeComboBox);

            content.Children.Add(new Wpf.Ui.Controls.TextBlock
            {
                Text = "安装包",
                FontSize = 12,
                Margin = new Thickness(0, 0, 0, 6)
            });
            content.Children.Add(pathGrid);

            var dialog = new ContentDialog(host)
            {
                Title = "安装插件",
                Content = content,
                PrimaryButtonText = "安装",
                CloseButtonText = "取消",
                DefaultButton = ContentDialogButton.Primary,
                DialogWidth = 520,
                DialogMaxWidth = 560,
                DialogMaxHeight = 520,
                PrimaryButtonAppearance = ControlAppearance.Primary
            };

            var result = await dialog.ShowAsync();
            if (result != ContentDialogResult.Primary)
                return;

            if (typeComboBox.SelectedItem is not ComboBoxItem typeItem ||
                typeItem.Tag is not string type)
            {
                (Window.GetWindow(this) as MainWindow)?.ShowToast(
                    "无法安装",
                    "请先选择插件类型。",
                    true);
                return;
            }

            var path = (pathBox.Text ?? string.Empty).Trim();
            if (string.IsNullOrWhiteSpace(path) || !File.Exists(path))
            {
                (Window.GetWindow(this) as MainWindow)?.ShowToast(
                    "无法安装",
                    "请选择有效的插件包文件。",
                    true);
                return;
            }

            try
            {
                InstallPluginButton.IsEnabled = false;

                var ok = await _backend.InstallPluginAsync(type, path);
                if (!ok)
                    throw new InvalidOperationException("Core 未确认插件安装成功。");

                await LoadPluginsFromBackendAsync();

                (Window.GetWindow(this) as MainWindow)?.ShowToast(
                    "插件已安装",
                    Path.GetFileName(path));
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("安装插件失败", ex);
            }
            finally
            {
                InstallPluginButton.IsEnabled = _backend.IsCoreConnected;
            }
        }

        private void UpdateEmptyState(bool disconnected)
        {
            var hasItems = PluginsList.Items.Count > 0;

            PluginsEmptyState.Visibility = hasItems
                ? Visibility.Collapsed
                : Visibility.Visible;

            if (hasItems)
                return;

            var filterActive =
                !string.IsNullOrWhiteSpace(SearchBox?.Text) ||
                TypeFilter?.SelectedIndex > 0;

            PluginsEmptyTitle.Text = disconnected
                ? "Core 未连接"
                : filterActive
                    ? "没有匹配的插件"
                    : "暂无插件";

            PluginsEmptyDetail.Text = disconnected
                ? "恢复连接后会自动刷新。"
                : filterActive
                    ? "可以清空搜索条件或切换类型。"
                    : "可以从右上角安装插件。";
        }

        private void SetDisconnectedState()
        {
            _allPlugins.Clear();
            PluginsList.ItemsSource = null;
            UpdateEmptyState(true);

            RefreshPluginsButton.IsEnabled = false;
            InstallPluginButton.IsEnabled = false;
        }

        private System.Threading.Tasks.Task ShowBackendErrorAsync(string title, Exception ex)
        {
            if (ex is BackendConnectionException)
            {
                (Window.GetWindow(this) as MainWindow)?.ShowBackendUnavailable();
                return System.Threading.Tasks.Task.CompletedTask;
            }

            (Window.GetWindow(this) as MainWindow)?.ShowToast(
                title,
                ex.Message,
                true);

            return System.Threading.Tasks.Task.CompletedTask;
        }

        private void NavigateTo(Type pageType)
        {
            if (Window.GetWindow(this) is MainWindow window)
                window.RootNavigation.Navigate(pageType);
        }
    }
}
