using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;
using Wpf.Ui.Controls;

namespace CuckooInterfaceUI.Pages
{
    public partial class EventsPage : AutoRefreshPage
    {
        private const int PageSize = 10;

        private readonly CoreBackend _backend = CoreBackend.Instance;
        private List<EventOption> _eventOptions = new();
        private List<EventRecord> _allEvents = new();
        private int _currentPage = 1;

        public EventsPage()
        {
            InitializeComponent();
            Loaded += EventsPage_Loaded;
            Unloaded += (_, _) => _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
        }

        // 每 2 秒自动刷新事件数据（静默模式：保留页码，不打断翻页）
        protected override Task OnAutoRefreshAsync() => LoadDataAsync(interactive: false);

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

        /// <summary>
        /// 加载事件数据。
        /// interactive=true：用户点击或连接恢复，重置到首页并给出完整反馈；
        /// interactive=false：2s 自动刷新的静默模式，保留当前页码，数据未变化时不重渲染。
        /// </summary>
        private async Task LoadDataAsync(bool interactive = true)
        {
            if (!_backend.IsCoreConnected)
            {
                SetDisconnectedState();
                return;
            }

            try
            {
                if (interactive)
                {
                    RefreshEventsButton.IsEnabled = false;
                    OpenRegisteredButton.IsEnabled = false;
                    OpenPublishButton.IsEnabled = false;
                }

                var recentTask = _backend.GetRecentEventsAsync();
                var registeredTask = _backend.GetRegisteredEventsAsync();
                var optionsTask = _backend.GetEventOptionsAsync();

                await Task.WhenAll(recentTask, registeredTask, optionsTask);

                var recent = await recentTask;
                var registered = await registeredTask;
                _eventOptions = await optionsTask;

                // 数据未变化时跳过重渲染，避免自动刷新打断分页与滚动位置
                var unchanged = EventsUnchanged(recent);

                _allEvents = recent;
                if (interactive)
                    _currentPage = 1;

                if (interactive || !unchanged)
                    RenderCurrentPage();
            }
            catch (Exception ex)
            {
                if (ex is BackendConnectionException)
                    SetDisconnectedState();

                if (interactive)
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

        /// <summary>
        /// 判断新拉取的事件列表与当前列表是否等价：条数一致且最新一条（列表按新→旧排列）
        /// 的时间戳与事件名相同。用于避免自动刷新时的无效重渲染。
        /// </summary>
        private bool EventsUnchanged(List<EventRecord> recent)
        {
            if (recent.Count != _allEvents.Count)
                return false;
            if (recent.Count == 0 || _allEvents.Count == 0)
                return false;

            var latest = recent[0];
            var current = _allEvents[0];
            return latest.Timestamp == current.Timestamp &&
                   latest.EventName == current.EventName &&
                   latest.Payload == current.Payload;
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

        /// <summary>
        /// 仅渲染当前页的事件，避免一次渲染过多条目。
        /// </summary>
        private void RenderCurrentPage()
        {
            var totalPages = Math.Max(1, (int)Math.Ceiling(_allEvents.Count / (double)PageSize));
            if (_currentPage > totalPages) _currentPage = totalPages;

            var skip = (_currentPage - 1) * PageSize;
            var pageItems = _allEvents.Skip(skip).Take(PageSize).ToList();

            RecentEventsList.ItemsSource = pageItems;
            UpdateRecentEmptyState(false);
            UpdateEventsPagination(totalPages);
        }

        private void UpdateEventsPagination(int totalPages)
        {
            EventsPageIndicator.Text = $"第 {_currentPage} / {totalPages} 页";
            EventsPrevPageButton.IsEnabled = _currentPage > 1;
            EventsNextPageButton.IsEnabled = _currentPage < totalPages;
        }

        private void EventsPrevPage_Click(object sender, RoutedEventArgs e)
        {
            if (_currentPage > 1)
            {
                _currentPage--;
                RenderCurrentPage();
            }
        }

        private void EventsNextPage_Click(object sender, RoutedEventArgs e)
        {
            var totalPages = (int)Math.Ceiling(_allEvents.Count / (double)PageSize);
            if (_currentPage < totalPages)
            {
                _currentPage++;
                RenderCurrentPage();
            }
        }

        private void SetDisconnectedState()
        {
            _allEvents.Clear();
            _currentPage = 1;
            RecentEventsList.ItemsSource = null;
            UpdateRecentEmptyState(true);
            UpdateEventsPagination(1);
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
