using System.Windows;
using System.Windows.Controls;
using System.Windows.Controls.Primitives;
using System.Windows.Input;
using System.Windows.Media;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// SettingsPage.xaml 的交互逻辑
    /// </summary>
    public partial class SettingsPage : Page
    {
        /// <summary>
        /// 当前选中的强调色
        /// </summary>
        private string _currentAccentColor = "#0078D4";

        public SettingsPage()
        {
            InitializeComponent();
            ApplyInitialAccentHighlight();
        }

        // ===== 强调色选择 =====

        /// <summary>
        /// 初始化时为默认强调色添加选择边框
        /// </summary>
        private void ApplyInitialAccentHighlight()
        {
            foreach (var child in ((UniformGrid)FindName("AccentColorContainer")).Children)
            {
                if (child is Border border && border.Tag is string color && color == _currentAccentColor)
                {
                    border.BorderBrush = new SolidColorBrush(Colors.White);
                    border.BorderThickness = new Thickness(2);
                }
            }
        }

        private void AccentColor_Click(object sender, MouseButtonEventArgs e)
        {
            if (sender is Border border && border.Tag is string color)
            {
                _currentAccentColor = color;

                // 清除所有边框
                var container = (UniformGrid)border.Parent;
                foreach (var child in container.Children)
                {
                    if (child is Border b)
                    {
                        b.BorderBrush = null;
                        b.BorderThickness = new Thickness(0);
                    }
                }

                // 高亮当前选中
                border.BorderBrush = new SolidColorBrush(Colors.White);
                border.BorderThickness = new Thickness(2);
            }
        }

        // ===== 字号缩放 =====

        private void FontScaleSlider_ValueChanged(object sender, RoutedPropertyChangedEventArgs<double> e)
        {
            if (FontScaleValue != null)
            {
                FontScaleValue.Text = $"{(int)e.NewValue}%";
            }
        }

        // ===== 日志单文件大小 =====

        private void LogMaxSizeSlider_ValueChanged(object sender, RoutedPropertyChangedEventArgs<double> e)
        {
            if (LogMaxSizeValue != null)
            {
                LogMaxSizeValue.Text = $"{(int)e.NewValue} MB";
            }
        }

        // ===== 高级选项显隐控制 =====

        private void ShowAdvancedToggle_Changed(object sender, RoutedEventArgs e)
        {
            if (ShowAdvancedToggle.IsChecked == true)
            {
                AdvancedSettingsPanel.Visibility = Visibility.Visible;
            }
            else
            {
                AdvancedSettingsPanel.Visibility = Visibility.Collapsed;
            }
        }

        // ===== Kernel 加载超时 =====

        private void KernelTimeoutSlider_ValueChanged(object sender, RoutedPropertyChangedEventArgs<double> e)
        {
            if (KernelTimeoutValue != null)
            {
                KernelTimeoutValue.Text = $"{(int)e.NewValue} s";
            }
        }

        // ===== 事件 Worker 数量 =====

        private void WorkerCountSlider_ValueChanged(object sender, RoutedPropertyChangedEventArgs<double> e)
        {
            if (WorkerCountValue != null)
            {
                WorkerCountValue.Text = ((int)e.NewValue).ToString();
            }
        }
    }
}
