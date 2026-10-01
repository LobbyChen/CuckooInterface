using System;
using System.Linq;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;
using System.Windows.Media.Animation;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// EventsPage.xaml 的交互逻辑
    /// </summary>
    public partial class EventsPage : Page
    {
        private readonly MockBackend _backend = MockBackend.Instance;

        public EventsPage()
        {
            InitializeComponent();
            LoadDataFromBackend();
        }

        // ===== 从后端加载数据 =====

        private void LoadDataFromBackend()
        {
            EventNameComboBox.ItemsSource = _backend.GetEventOptions();
            RecentEventsList.ItemsSource = _backend.GetRecentEvents();
            RegisteredEventsGrid.ItemsSource = _backend.GetRegisteredEvents();
            RegisteredCountText.Text = $"共 {_backend.GetRegisteredEvents().Count} 个事件";
        }

        // ===== 叠加层控制 =====

        private void OpenPublishOverlay_Click(object sender, RoutedEventArgs e)
        {
            EventNameComboBox.SelectedIndex = -1;
            PayloadTextBox.Text = string.Empty;
            MainContent.Visibility = Visibility.Collapsed;
            ShowOverlay(PublishOverlay, PublishOverlayBorder);
        }

        private void ClosePublishOverlay_Click(object sender, RoutedEventArgs e)
        {
            HideOverlay(PublishOverlay, PublishOverlayBorder);
        }

        private void OpenRegisteredOverlay_Click(object sender, RoutedEventArgs e)
        {
            MainContent.Visibility = Visibility.Collapsed;
            ShowOverlay(RegisteredOverlay, RegisteredOverlayBorder);
        }

        private void CloseRegisteredOverlay_Click(object sender, RoutedEventArgs e)
        {
            HideOverlay(RegisteredOverlay, RegisteredOverlayBorder);
        }

        private void ShowOverlay(Grid overlay, FrameworkElement border)
        {
            overlay.Visibility = Visibility.Visible;
            border.Opacity = 0;
            var showAnim = (Storyboard)FindResource("OverlayShowAnimation");
            Storyboard.SetTarget(showAnim, border);
            showAnim.Begin();
        }

        private void HideOverlay(Grid overlay, FrameworkElement border)
        {
            var hideAnim = (Storyboard)FindResource("OverlayHideAnimation");
            Storyboard.SetTarget(hideAnim, border);
            hideAnim.Completed += (s, args) =>
            {
                overlay.Visibility = Visibility.Collapsed;
                MainContent.Visibility = Visibility.Visible;
            };
            hideAnim.Begin();
        }

        // ===== 事件发布逻辑 =====

        private void EventNameComboBox_SelectionChanged(object sender, SelectionChangedEventArgs e)
        {
            if (EventNameComboBox.SelectedItem is EventOption option)
            {
                PayloadTextBox.Text = option.DefaultPayload;
            }
        }

        private async void PublishEvent_Click(object sender, RoutedEventArgs e)
        {
            if (EventNameComboBox.SelectedItem is not EventOption option)
            {
                await new Wpf.Ui.Controls.MessageBox
                {
                    Title = "提示",
                    Content = "请先选择要发布的事件名称",
                    PrimaryButtonText = "确定"
                }.ShowDialogAsync();
                return;
            }

            var payload = string.IsNullOrWhiteSpace(PayloadTextBox.Text) ? "{}" : PayloadTextBox.Text.Trim();

            // 调用后端发布事件
            _backend.PublishEvent(option.Name, payload);

            // 刷新列表（ObservableCollection 会自动通知 UI 更新）
            RecentEventsList.ItemsSource = null;
            RecentEventsList.ItemsSource = _backend.GetRecentEvents();

            HideOverlay(PublishOverlay, PublishOverlayBorder);
        }

        private void RepublishEvent_Click(object sender, RoutedEventArgs e)
        {
            if (sender is Button btn && btn.Tag is EventRecord evt)
            {
                // 调用后端重发事件
                _backend.PublishEvent(evt.EventName, evt.Payload);

                RecentEventsList.ItemsSource = null;
                RecentEventsList.ItemsSource = _backend.GetRecentEvents();
            }
        }
    }
}
