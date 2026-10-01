using System;
using System.Windows.Media;
using Brush = System.Windows.Media.Brush;
using Color = System.Windows.Media.Color;

namespace CuckooInterfaceUI.Models
{
    /// <summary>
    /// 日志级别
    /// </summary>
    public enum LogLevel
    {
        Debug,
        Info,
        Warn,
        Error
    }

    /// <summary>
    /// 日志条目
    /// </summary>
    public class LogEntry
    {
        public DateTime Timestamp { get; set; } = DateTime.Now;
        public LogLevel Level { get; set; } = LogLevel.Info;
        public string Message { get; set; } = string.Empty;
        public string Logger { get; set; } = "core";

        public string Time => Timestamp.ToString("yyyy-MM-dd HH:mm:ss.fff");

        public string LevelText => Level.ToString().ToUpper();

        public Brush LevelColor => Level switch
        {
            LogLevel.Debug => new SolidColorBrush(Color.FromRgb(0x8A, 0x8A, 0x8A)),
            LogLevel.Info => new SolidColorBrush(Color.FromRgb(0x43, 0xB5, 0x81)),
            LogLevel.Warn => new SolidColorBrush(Color.FromRgb(0xF5, 0xA6, 0x23)),
            LogLevel.Error => new SolidColorBrush(Color.FromRgb(0xE5, 0x48, 0x4D)),
            _ => Brushes.Gray
        };
    }
}
