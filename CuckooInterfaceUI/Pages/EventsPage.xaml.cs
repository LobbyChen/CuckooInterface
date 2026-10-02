using System;
using System.Collections.Generic;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;
using Wpf.Ui.Controls;

namespace CuckooInterfaceUI.Pages
{
    public partial class EventsPage : Page
    {
        private readonly CoreBackend _backend = CoreBackend.Instance;
        private List<EventOption> _eventOptions = new();

        public EventsPage()
        {
            InitializeComponent();
            Loaded += EventsPage_Loaded;
            Unloaded += (_, _) => _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
        }

        private async void EventsPage_Loaded(object sender, RoutedEventArgs e)
        {
            _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
            _backend.ConnectionStateChanged += Backend_ConnectionStateChanged;
            await LoadDataAsync();
        }

        private void Backend_ConnectionStateChanged(object? sender, BackendConnectionStateChangedEventArgs e)
        {
            if (!e.IsCore || !IsVisible)
                return;

            _ = Dispatcher.InvokeAsync(async () =>
            {
                if (e.IsConnected)
                    await LoadDataAsync();
                else
                    SetDisconnectedState();
            });
        }

        private async Task LoadDataAsync()
        {
            if (!_backend.IsCoreConnected)
            {
                SetDisconnectedState();
                return;
            }

            try
            {
                RefreshEventsButton.IsEnabled = false;
                OpenRegisteredButton.IsEnabled = false;
                OpenPublishButton.IsEnabled = false;

                var recentTask = _backend.GetRecentEventsAsync();
                var registeredTask = _backend.GetRegisteredEventsAsync();
                var optionsTask = _backend.GetEventOptionsAsync();

                await Task.WhenAll(recentTask, registeredTask, optionsTask);

                var recent = await recentTask;
                var registered = await registeredTask;
                _eventOptions = await optionsTask;

                RecentEventsList.ItemsSource = recent;
                UpdateRecentEmptyState(false);
            }
            catch (Exception ex)
            {
                if (ex is BackendConnectionException)
                    SetDisconnectedState();

                await ShowBackendErrorAsync("加载事件失败", ex);
            }
            finally
            {
                var connected = _backend.IsCoreConnected;
                RefreshEventsButton.IsEnabled = connected;
                OpenRegisteredButton.IsEnabled = connected;
                OpenPublishButton.IsEnabled = connected;
            }
        }

        private async void RefreshEvents_Click(object sender, RoutedEventArgs e)
        {
            await LoadDataAsync();
        }

        private void UpdateRecentEmptyState(bool disconnected)
        {
            var hasItems = RecentEventsList.Items.Count > 0;
            EventsEmptyState.Visibility = hasItems
                ? Visibility.Collapsed
                : Visibility.Visible;

            EventsEmptyTitle.Text = disconnected ? "Core 未连接" : "暂无事件";
            EventsEmptyDetail.Text = disconnected
                ? "恢复连接后会自动刷新。"
                : "当前没有可显示的事件。";
        }

        private void SetDisconnectedState()
        {
            RecentEventsList.ItemsSource = null;
            UpdateRecentEmptyState(true);
            _eventOptions.Clear();

            RefreshEventsButton.IsEnabled = false;
            OpenRegisteredButton.IsEnabled = false;
            OpenPublishButton.IsEnabled = false;
        }

        private async void OpenPublishPanel_Click(object sender, RoutedEventArgs e)
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

            var eventNameComboBox = new ComboBox
            {
                ItemsSource = _eventOptions,
                DisplayMemberPath = nameof(EventOption.Name),
                MinWidth = 340,
                Margin = new Thickness(0, 0, 0, 18),
                Foreground = primaryBrush,
                Background = controlBrush
            };

            var payloadTextBox = new Wpf.Ui.Controls.TextBox
            {
                Height = 170,
                Foreground = primaryBrush,
                Background = controlBrush,
                AcceptsReturn = true,
                TextWrapping = TextWrapping.Wrap,
                VerticalScrollBarVisibility = ScrollBarVisibility.Auto,
                FontFamily = new System.Windows.Media.FontFamily("Consolas, Courier New, monospace"),
                FontSize = 12,
                PlaceholderText = "{}"
            };

            eventNameComboBox.SelectionChanged += (_, _) =>
            {
                if (eventNameComboBox.SelectedItem is EventOption option)
                    payloadTextBox.Text = option.DefaultPayload;
            };

            var content = new StackPanel
            {
                MinWidth = 420
            };

            if (primaryBrush != null)
                System.Windows.Documents.TextElement.SetForeground(content, primaryBrush);

            content.Children.Add(new Wpf.Ui.Controls.TextBlock
            {
                Text = "事件名称",
                FontSize = 12,
                Margin = new Thickness(0, 0, 0, 6)
            });
            content.Children.Add(eventNameComboBox);

            content.Children.Add(new Wpf.Ui.Controls.TextBlock
            {
                Text = "Payload",
                FontSize = 12,
                Margin = new Thickness(0, 0, 0, 6)
            });
            content.Children.Add(payloadTextBox);

            var dialog = new ContentDialog(host)
            {
                Title = "发布事件",
                Content = content,
                PrimaryButtonText = "发布",
                CloseButtonText = "取消",
                DefaultButton = ContentDialogButton.Primary,
                DialogWidth = 560,
                DialogMaxWidth = 620,
                DialogMaxHeight = 560,
                PrimaryButtonAppearance = ControlAppearance.Primary
            };

            var result = await dialog.ShowAsync();
            if (result != ContentDialogResult.Primary)
                return;

            if (eventNameComboBox.SelectedItem is not EventOption option)
            {
                (Window.GetWindow(this) as MainWindow)?.ShowToast(
                    "无法发布",
                    "请先选择事件名称。",
                    true);
                return;
            }

            var payload = string.IsNullOrWhiteSpace(payloadTextBox.Text)
                ? "{}"
                : payloadTextBox.Text.Trim();

            try
            {
                await _backend.PublishEventAsync(option.Name, payload);
                await LoadDataAsync();
                (Window.GetWindow(this) as MainWindow)?.ShowToast(
                    "事件已发布",
                    option.Name);
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("发布事件失败", ex);
            }
        }

        private async void OpenRegisteredPanel_Click(object sender, RoutedEventArgs e)
        {
            if (!_backend.IsCoreConnected)
            {
                (Window.GetWindow(this) as MainWindow)?.ShowBackendUnavailable();
                return;
            }

            var host = (Window.GetWindow(this) as MainWindow)?.RootContentDialogHost;
            if (host == null)
                return;

            try
            {
                var registered = await _backend.GetRegisteredEventsAsync();

                var grid = new System.Windows.Controls.DataGrid
                {
                    AutoGenerateColumns = false,
                    IsReadOnly = true,
                    CanUserResizeRows = false,
                    CanUserReorderColumns = false,
                    GridLinesVisibility = DataGridGridLinesVisibility.Horizontal,
                    HeadersVisibility = DataGridHeadersVisibility.Column,
                    Background = System.Windows.Media.Brushes.Transparent,
                    BorderThickness = new Thickness(0),
                    RowHeight = 36,
                    FontSize = 13,
                    MinHeight = 220,
                    MaxHeight = 460
                };

                grid.Columns.Add(new DataGridTextColumn
                {
                    Header = "事件名称",
                    Binding = new System.Windows.Data.Binding(nameof(RegisteredEvent.EventName)),
                    Width = new DataGridLength(1, DataGridLengthUnitType.Star)
                });
                grid.Columns.Add(new DataGridTextColumn
                {
                    Header = "提供者",
                    Binding = new System.Windows.Data.Binding(nameof(RegisteredEvent.Provider)),
                    Width = new DataGridLength(160)
                });
                grid.Columns.Add(new DataGridTextColumn
                {
                    Header = "监听器数",
                    Binding = new System.Windows.Data.Binding(nameof(RegisteredEvent.ListenerCount)),
                    Width = new DataGridLength(90)
                });

                grid.ItemsSource = registered;

                var dialog = new ContentDialog(host)
                {
                    Title = $"已注册事件（{registered.Count}）",
                    Content = grid,
                    CloseButtonText = "关闭",
                    DefaultButton = ContentDialogButton.Close,
                    DialogWidth = 700,
                    DialogMaxWidth = 760,
                    DialogMaxHeight = 560
                };

                await dialog.ShowAsync();
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("加载已注册事件失败", ex);
            }
        }

        private async void RepublishEvent_Click(object sender, RoutedEventArgs e)
        {
            if (sender is not FrameworkElement element ||
                element.Tag is not EventRecord evt)
            {
                return;
            }

            if (!_backend.IsCoreConnected)
            {
                (Window.GetWindow(this) as MainWindow)?.ShowBackendUnavailable();
                return;
            }

            try
            {
                await _backend.PublishEventAsync(evt.EventName, evt.Payload);
                await LoadDataAsync();
                (Window.GetWindow(this) as MainWindow)?.ShowToast(
                    "事件已重新发布",
                    evt.EventName);
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("重新发布失败", ex);
            }
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
