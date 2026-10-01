using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Windows;
using System.Windows.Controls;
using CuckooInterfaceUI.Models;
using CuckooInterfaceUI.Services;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// LogsPage.xaml 的交互逻辑
    /// </summary>
    public partial class LogsPage : Page
    {
        private readonly MockBackend _backend = MockBackend.Instance;
        private List<LogEntry> _allLogs = new();

        public LogsPage()
        {
            InitializeComponent();
            LoadLogsFromBackend();
        }

        private void LoadLogsFromBackend()
        {
            _allLogs = _backend.GetLogs().ToList();
            ApplyFilter();
        }

        // ===== 筛选与搜索 =====

        private void ApplyFilter()
        {
            // InitializeComponent 期间控件可能尚未全部创建（ComboBox 的 SelectedIndex
            // 会在 XAML 解析阶段触发 SelectionChanged），此时直接访问会 NRE。
            if (LogsDataGrid == null || SearchBox == null || LevelFilter == null)
                return;

            string? levelFilter = null;
            if (LevelFilter.SelectedItem is ComboBoxItem item && item.Content is string level && level != "所有级别")
            {
                levelFilter = level;
            }

            string search = (SearchBox.Text ?? string.Empty).Trim().ToLower();

            var filtered = _allLogs.Where(log =>
            {
                if (levelFilter != null && log.LevelText != levelFilter) return false;
                if (!string.IsNullOrEmpty(search))
                {
                    return log.Message.ToLower().Contains(search) ||
                           log.Logger.ToLower().Contains(search);
                }
                return true;
            }).ToList();

            LogsDataGrid.ItemsSource = filtered;
        }

        private void SearchBox_TextChanged(object sender, TextChangedEventArgs e)
        {
            ApplyFilter();
        }

        private void LevelFilter_SelectionChanged(object sender, SelectionChangedEventArgs e)
        {
            ApplyFilter();
        }

        // ===== 操作 =====

        private async void ClearLogs_Click(object sender, RoutedEventArgs e)
        {
            var result = await new Wpf.Ui.Controls.MessageBox
            {
                Title = "确认",
                Content = "确定要清空所有日志吗？",
                PrimaryButtonText = "是",
                SecondaryButtonText = "否"
            }.ShowDialogAsync();

            if (result == Wpf.Ui.Controls.MessageBoxResult.Primary)
            {
                _backend.GetLogs().Clear();
                LoadLogsFromBackend();
            }
        }

        private async void ExportLogs_Click(object sender, RoutedEventArgs e)
        {
            var dialog = new Microsoft.Win32.SaveFileDialog
            {
                Filter = "CSV 文件 (*.csv)|*.csv|文本文件 (*.txt)|*.txt",
                FileName = $"cuckoo_logs_{System.DateTime.Now:yyyyMMdd_HHmmss}"
            };

            if (dialog.ShowDialog() == true)
            {
                var lines = _allLogs.Select(log =>
                    $"{log.Time},{log.LevelText},{log.Logger},{log.Message}");
                File.WriteAllLines(dialog.FileName, lines);
                await new Wpf.Ui.Controls.MessageBox
                {
                    Title = "导出成功",
                    Content = $"日志已导出到：\n{dialog.FileName}",
                    PrimaryButtonText = "确定"
                }.ShowDialogAsync();
            }
        }
    }
}
