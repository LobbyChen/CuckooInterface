using System;
using System.Windows.Media;
using Brush = System.Windows.Media.Brush;
using Color = System.Windows.Media.Color;

namespace CuckooInterfaceUI.Models
{
    /// <summary>
    /// 事件级别
    /// </summary>
    public enum EventLevel
    {
        Normal,
        Warning,
        Error
    }

    /// <summary>
    /// 历史事件记录
    /// </summary>
    public class EventRecord
    {
        public DateTime Timestamp { get; set; } = DateTime.Now;
        public string EventName { get; set; } = string.Empty;
        public string Payload { get; set; } = "{}";
        public EventLevel Level { get; set; } = EventLevel.Normal;

        public string Time => Timestamp.ToString("HH:mm:ss");

        public string LevelText => Level switch
        {
            EventLevel.Normal => "正常",
            EventLevel.Warning => "警告",
            EventLevel.Error => "错误",
            _ => "未知"
        };

        public Brush LevelColor => Level switch
        {
            EventLevel.Normal => new SolidColorBrush(Color.FromRgb(0x63, 0xD3, 0x8B)),
            EventLevel.Warning => new SolidColorBrush(Color.FromRgb(0xF2, 0xC3, 0x5E)),
            EventLevel.Error => new SolidColorBrush(Color.FromRgb(0xFF, 0x72, 0x72)),
            _ => new SolidColorBrush(Color.FromRgb(0x9A, 0xA0, 0xA6))
        };
    }

    /// <summary>
    /// 已注册事件
    /// </summary>
    public class RegisteredEvent
    {
        public string EventName { get; set; } = string.Empty;
        public string Provider { get; set; } = string.Empty;
        public int ListenerCount { get; set; } = 0;
    }

    /// <summary>
    /// 可发布事件选项
    /// </summary>
    public class EventOption
    {
        public string Name { get; set; } = string.Empty;
        public string DefaultPayload { get; set; } = "{}";
    }
}


