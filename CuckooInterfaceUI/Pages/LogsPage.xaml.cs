using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Threading.Tasks;
using System.Windows;
using System.Windows.Controls;
using Microsoft.Win32;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;

namespace CuckooInterfaceUI.Pages
{
    public partial class LogsPage : AutoRefreshPage
    {
        private const int PageSize = 10;

        private readonly CoreBackend _backend = CoreBackend.Instance;
        private List<LogEntry> _allLogs = new();
        private List<LogEntry> _filteredLogs = new();
        private int _currentPage = 1;

        public LogsPage()
        {
            InitializeComponent();
            Loaded += LogsPage_Loaded;
            Unloaded += (_, _) => _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
        }

        // 每 2 秒自动刷新日志（静默模式：保留页码，不打断浏览）
        protected override Task OnAutoRefreshAsync() => LoadLogsFromBackendAsync(interactive: false);

        private async void LogsPage_Loaded(object sender, RoutedEventArgs e)
        {
            _backend.ConnectionStateChanged -= Backend_ConnectionStateChanged;
            _backend.ConnectionStateChanged += Backend_ConnectionStateChanged;
            await LoadLogsFromBackendAsync();
        }

        private void Backend_ConnectionStateChanged(object? sender, BackendConnectionStateChangedEventArgs e)
        {
            if (!e.IsCore || !IsVisible) return;
            _ = Dispatcher.InvokeAsync(async () =>
            {
                if (e.IsConnected) await LoadLogsFromBackendAsync();
                else SetDisconnectedState();
            });
        }

        /// <summary>
        /// 加载日志数据。
        /// interactive=true：用户点击或连接恢复，回到首页并给出完整反馈；
        /// interactive=false：2s 自动刷新的静默模式，保留页码，数据未变化时不重渲染。
        /// </summary>
        private async Task LoadLogsFromBackendAsync(bool interactive = true)
        {
            if (!_backend.IsCoreConnected)
            {
                SetDisconnectedState();
                return;
            }

            try
            {
                if (interactive)
                    RefreshLogsButton.IsEnabled = false;

                var logs = await _backend.GetLogsAsync();

                // 数据未变化时跳过重建，避免自动刷新打断滚动与页码
                if (LogsSignature(logs) == LogsSignature(_allLogs))
                    return;

                _allLogs = logs;
                ApplyFilter(resetPage: interactive);
            }
            catch (Exception ex)
            {
                if (ex is BackendConnectionException) SetDisconnectedState();
                if (interactive)
                    await ShowBackendErrorAsync("加载日志失败", ex);
            }
            finally
            {
                var connected = _backend.IsCoreConnected;
                RefreshLogsButton.IsEnabled = connected;
                ExportLogsButton.IsEnabled = connected && LogsList.Items.Count > 0;
                ClearLogsButton.IsEnabled = connected && _allLogs.Count > 0;
            }
        }

        /// <summary>日志列表状态签名：条数 + 首末条时间与消息，用于判断自动刷新时数据是否真的变化。</summary>
        private static string LogsSignature(List<LogEntry> logs)
        {
            if (logs.Count == 0)
                return "0";

            var first = logs[0];
            var last = logs[logs.Count - 1];
            return $"{logs.Count}:{first.Timestamp:O}:{first.Message}:{last.Timestamp:O}:{last.Message}";
        }

        private void ApplyFilter(bool resetPage = true)
        {
            if (LogsList == null || SearchBox == null || LevelFilter == null) return;

            string? levelFilter = null;
            if (LevelFilter.SelectedItem is ComboBoxItem item && item.Content is string level && level != "所有级别")
                levelFilter = level;

            var search = (SearchBox.Text ?? string.Empty).Trim();
            _filteredLogs = _allLogs.Where(log =>
            {
                if (levelFilter != null && log.LevelText != levelFilter) return false;
                if (!string.IsNullOrWhiteSpace(search))
                {
                    return log.Message.Contains(search, StringComparison.OrdinalIgnoreCase) ||
                           log.Logger.Contains(search, StringComparison.OrdinalIgnoreCase);
                }
                return true;
            }).ToList();

            // 自动刷新时保留当前页码，用户手动改筛选或点击刷新时回到首页
            if (resetPage)
                _currentPage = 1;
            RenderCurrentPage();
        }

        /// <summary>
        /// 仅渲染当前页的日志，避免一次渲染过多条目。
        /// </summary>
        private void RenderCurrentPage()
        {
            var totalPages = Math.Max(1, (int)Math.Ceiling(_filteredLogs.Count / (double)PageSize));
            if (_currentPage > totalPages) _currentPage = totalPages;

            var skip = (_currentPage - 1) * PageSize;
            var display = _filteredLogs.Skip(skip).Take(PageSize).ToList();

            LogsList.ItemsSource = display;
            LogCountText.Text = $"{_filteredLogs.Count} 条";
            UpdateEmptyState();
            UpdateLogsPagination(totalPages);
            ExportLogsButton.IsEnabled = _backend.IsCoreConnected && display.Count > 0;
            ClearLogsButton.IsEnabled = _backend.IsCoreConnected && _allLogs.Count > 0;
        }

        private void UpdateLogsPagination(int totalPages)
        {
            LogsPageIndicator.Text = $"第 {_currentPage} / {totalPages} 页";
            LogsPrevPageButton.IsEnabled = _currentPage > 1;
            LogsNextPageButton.IsEnabled = _currentPage < totalPages;
        }

        private void LogsPrevPage_Click(object sender, RoutedEventArgs e)
        {
            if (_currentPage > 1)
            {
                _currentPage--;
                RenderCurrentPage();
            }
        }

        private void LogsNextPage_Click(object sender, RoutedEventArgs e)
        {
            var totalPages = (int)Math.Ceiling(_filteredLogs.Count / (double)PageSize);
            if (_currentPage < totalPages)
            {
                _currentPage++;
                RenderCurrentPage();
            }
        }

        private void UpdateEmptyState()
        {
            var hasItems = LogsList.Items.Count > 0;
            LogsEmptyState.Visibility = hasItems ? Visibility.Collapsed : Visibility.Visible;
            if (hasItems) return;

            var hasFilter = !string.IsNullOrWhiteSpace(SearchBox.Text) || LevelFilter.SelectedIndex > 0;
            LogsEmptyTitle.Text = hasFilter ? "没有匹配日志" : "暂无日志";
            LogsEmptyDetail.Text = hasFilter
                ? "清空搜索条件或切换日志级别。"
                : "Core 当前没有返回可显示的日志。";
        }

        private void SetDisconnectedState()
        {
            _allLogs.Clear();
            _filteredLogs.Clear();
            _currentPage = 1;
            LogsList.ItemsSource = null;
            LogCountText.Text = "—";
            UpdateLogsPagination(1);
            LogsEmptyState.Visibility = Visibility.Visible;
            LogsEmptyTitle.Text = "Core 未连接";
            LogsEmptyDetail.Text = "恢复连接后会自动刷新。";
            RefreshLogsButton.IsEnabled = false;
            ExportLogsButton.IsEnabled = false;
            ClearLogsButton.IsEnabled = false;
        }

        private void SearchBox_TextChanged(object sender, TextChangedEventArgs e) => ApplyFilter();

        private void LevelFilter_SelectionChanged(object sender, SelectionChangedEventArgs e) => ApplyFilter();

        private async void RefreshLogs_Click(object sender, RoutedEventArgs e) => await LoadLogsFromBackendAsync();

        private async void ClearLogs_Click(object sender, RoutedEventArgs e)
        {
            if (!_backend.IsCoreConnected)
            {
                (Window.GetWindow(this) as MainWindow)?.ShowBackendUnavailable();
                return;
            }

            if (Window.GetWindow(this) is not MainWindow window) return;
            var confirmed = await window.ShowConfirmDialogAsync("清空日志", "确定清空 Core 当前日志缓存吗？", "清空", danger: true);
            if (!confirmed) return;

            try
            {
                ClearLogsButton.IsEnabled = false;
                var ok = await _backend.ClearLogsAsync();
                if (!ok) throw new InvalidOperationException("Core 未确认日志清空。");
                await LoadLogsFromBackendAsync();
                (Window.GetWindow(this) as MainWindow)?.ShowToast("日志已清空", "Core 日志缓存已经清空。");
            }
            catch (Exception ex)
            {
                await ShowBackendErrorAsync("清空日志失败", ex);
            }
        }

        private void ExportLogs_Click(object sender, RoutedEventArgs e)
        {
            var items = LogsList.Items.Cast<object>().OfType<LogEntry>().ToList();
            if (items.Count == 0) return;

            var dialog = new SaveFileDialog
            {
                Filter = "CSV 文件 (*.csv)|*.csv|文本文件 (*.txt)|*.txt",
                FileName = $"cuckoo_logs_{DateTime.Now:yyyyMMdd_HHmmss}"
            };
            if (dialog.ShowDialog() != true) return;

            try
            {
                ExportLogsButton.IsEnabled = false;
                var lines = items.Select(log =>
                    $"{EscapeCsv(log.Time)},{EscapeCsv(log.LevelText)},{EscapeCsv(log.Logger)},{EscapeCsv(log.Message)}");
                File.WriteAllLines(dialog.FileName, lines);
                (Window.GetWindow(this) as MainWindow)?.ShowToast("导出成功", $"已导出 {items.Count} 条日志。");
            }
            catch (Exception ex)
            {
                (Window.GetWindow(this) as MainWindow)?.ShowToast("导出失败", ex.Message, true);
            }
            finally
            {
                ExportLogsButton.IsEnabled = _backend.IsCoreConnected && items.Count > 0;
            }
        }

        private static string EscapeCsv(string value)
        {
            if (value.IndexOfAny(new[] { ',', '"', '\r', '\n' }) < 0) return value;
            return $"\"{value.Replace("\"", "\"\"")}\"";
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
