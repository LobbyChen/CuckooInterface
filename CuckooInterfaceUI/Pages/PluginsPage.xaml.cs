using System;
using System.Collections.Generic;
using System.Linq;
using System.Windows;
using System.Windows.Controls;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// PluginsPage.xaml 的交互逻辑
    /// </summary>
    public partial class PluginsPage : Page
    {
        private readonly MockBackend _backend = MockBackend.Instance;
        private List<PluginInfo> _allPlugins = new();

        public PluginsPage()
        {
            InitializeComponent();
            LoadPluginsFromBackend();
        }

        private void LoadPluginsFromBackend()
        {
            _allPlugins = _backend.GetPlugins().ToList();
            ApplyFilter();
        }

        // ===== 筛选与搜索 =====

        private void ApplyFilter()
        {
            string? typeFilter = null;
            if (TypeFilter.SelectedItem is ComboBoxItem item && item.Content is string type && type != "全部类型")
            {
                typeFilter = type;
            }

            string search = (SearchBox.Text ?? string.Empty).Trim().ToLower();

            var filtered = _allPlugins.Where(p =>
            {
                if (typeFilter != null && p.TypeText != typeFilter) return false;
                if (!string.IsNullOrEmpty(search) && !p.Name.ToLower().Contains(search)) return false;
                return true;
            }).ToList();

            PluginsList.ItemsSource = filtered;
        }

        private void TypeFilter_SelectionChanged(object sender, SelectionChangedEventArgs e)
        {
            ApplyFilter();
        }

        private void SearchBox_TextChanged(object sender, TextChangedEventArgs e)
        {
            ApplyFilter();
        }

        private void RefreshButton_Click(object sender, RoutedEventArgs e)
        {
            LoadPluginsFromBackend();
        }

        // ===== 安装插件（跳转到配置页设置插件目录） =====

        private void InstallPlugin_Click(object sender, RoutedEventArgs e)
        {
            NavigateTo(typeof(ConfigPage));
        }

        // ===== 插件启用/禁用 =====

        private void PluginToggle_Changed(object sender, RoutedEventArgs e)
        {
            if (sender is System.Windows.Controls.Primitives.ToggleButton toggle && toggle.Tag is PluginInfo plugin)
            {
                bool enabled = toggle.IsChecked == true;
                _backend.TogglePlugin(plugin.Id, enabled);
            }
        }

        // ===== 插件设置（跳转到配置页） =====

        private void PluginSettings_Click(object sender, RoutedEventArgs e)
        {
            NavigateTo(typeof(ConfigPage));
        }

        // ===== 卸载插件 =====

        private async void DeletePlugin_Click(object sender, RoutedEventArgs e)
        {
            if (sender is System.Windows.Controls.Button btn && btn.Tag is PluginInfo plugin)
            {
                var result = await new Wpf.Ui.Controls.MessageBox
                {
                    Title = "卸载插件",
                    Content = $"确定要卸载插件「{plugin.Name}」吗？此操作不可撤销。",
                    PrimaryButtonText = "卸载",
                    SecondaryButtonText = "取消"
                }.ShowDialogAsync();

                if (result == Wpf.Ui.Controls.MessageBoxResult.Primary)
                {
                    _backend.RemovePlugin(plugin.Id);
                    LoadPluginsFromBackend();
                }
            }
        }

        // ===== 导航辅助 =====

        private void NavigateTo(Type pageType)
        {
            if (Window.GetWindow(this) is MainWindow window)
            {
                window.RootNavigation.Navigate(pageType);
            }
        }
    }
}

