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
    public partial class LogsPage : Page
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

        private async Task LoadLogsFromBackendAsync()
        {
            if (!_backend.IsCoreConnected)
            {
                SetDisconnectedState();
                return;
            }

            try
            {
                RefreshLogsButton.IsEnabled = false;
                _allLogs = await _backend.GetLogsAsync();
                ApplyFilter();
            }
            catch (Exception ex)
            {
                if (ex is BackendConnectionException) SetDisconnectedState();
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

        private void ApplyFilter()
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
