using System;
using System.Linq;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// ConfigPage.xaml 的交互逻辑。
    /// </summary>
    public partial class ConfigPage : AutoRefreshPage
    {
        private readonly CoreBackend _backend = CoreBackend.Instance;

        public ConfigPage()
        {
            InitializeComponent();
            Loaded += ConfigPage_Loaded;
            Unloaded += (_, _) => _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
        }

        // 每 2 秒自动刷新插件列表
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
        /// 加载插件列表。
        /// interactive=true：用户点击或连接恢复，错误时给出提示；
        /// interactive=false：2s 自动刷新的静默模式，保留当前选中的插件。
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
                var pluginList = plugins.ToList();

                // 记住当前选中项，避免自动刷新把用户正在查看的插件跳走
                var selectedId = (PluginListBox.SelectedItem as PluginInfo)?.Id;

                PluginListBox.ItemsSource = pluginList;

                if (selectedId != null)
                    PluginListBox.SelectedItem = pluginList.FirstOrDefault(p => p.Id == selectedId);

                if (pluginList.Count == 0)
                {
                    ClearPluginSelection("暂无可配置插件", "Core 当前没有返回任何已安装插件。");
                    return;
                }

                // 首次进入配置页时自动选择第一项，避免右侧出现空白标题区域。
                if (PluginListBox.SelectedIndex < 0)
                    PluginListBox.SelectedIndex = 0;
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
            PluginListBox.ItemsSource = null;
            PluginListBox.SelectedItem = null;
            ClearPluginSelection("Core 未连接", "等待后端恢复后会自动重新加载配置内容。");
        }

        private void ClearPluginSelection(string title, string description)
        {
            SelectedPluginName.Text = title;
            SelectedPluginDesc.Text = description;
        }

        private void PluginListBox_SelectionChanged(object sender, SelectionChangedEventArgs e)
        {
            if (PluginListBox.SelectedItem is PluginInfo plugin)
            {
                SelectedPluginName.Text = plugin.Name;
                SelectedPluginDesc.Text = string.IsNullOrWhiteSpace(plugin.Description)
                    ? "该插件未提供描述。"
                    : plugin.Description;
            }
        }
    }
}
