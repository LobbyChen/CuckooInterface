using System.Linq;
using System.Windows.Controls;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// ConfigPage.xaml 的交互逻辑
    /// </summary>
    public partial class ConfigPage : Page
    {
        private readonly MockBackend _backend = MockBackend.Instance;

        public ConfigPage()
        {
            InitializeComponent();
            LoadPluginsFromBackend();
        }

        private void LoadPluginsFromBackend()
        {
            PluginListBox.ItemsSource = _backend.GetPlugins().ToList();
        }

        private void PluginListBox_SelectionChanged(object sender, SelectionChangedEventArgs e)
        {
            if (PluginListBox.SelectedItem is PluginInfo plugin)
            {
                SelectedPluginName.Text = plugin.Name;
                SelectedPluginDesc.Text = plugin.Description;
            }
        }
    }
}
