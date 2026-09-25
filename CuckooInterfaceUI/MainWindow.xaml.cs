using Wpf.Ui.Controls;

namespace CuckooInterfaceUI
{
    /// <summary>
    /// MainWindow.xaml 的交互逻辑
    /// </summary>

    public partial class MainWindow : FluentWindow
    {
        public MainWindow()
        {
            InitializeComponent();
            Loaded += (_, _) => RootNavigation.Navigate(typeof(Pages.HomePage));
        }
    }
}
