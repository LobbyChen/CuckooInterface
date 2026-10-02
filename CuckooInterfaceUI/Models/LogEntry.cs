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
            LogLevel.Debug => new SolidColorBrush(Color.FromRgb(0x9A, 0xA0, 0xA6)),
            LogLevel.Info => new SolidColorBrush(Color.FromRgb(0x63, 0xD3, 0x8B)),
            LogLevel.Warn => new SolidColorBrush(Color.FromRgb(0xF2, 0xC3, 0x5E)),
            LogLevel.Error => new SolidColorBrush(Color.FromRgb(0xFF, 0x72, 0x72)),
            _ => new SolidColorBrush(Color.FromRgb(0x9A, 0xA0, 0xA6))
        };
    }
}


