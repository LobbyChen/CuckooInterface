// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
using System;
using System.Windows.Media;
using Brush = System.Windows.Media.Brush;

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
            LogLevel.Debug => UiBrushes.Neutral,
            LogLevel.Info => UiBrushes.Success,
            LogLevel.Warn => UiBrushes.Warning,
            LogLevel.Error => UiBrushes.Danger,
            _ => UiBrushes.Neutral
        };
    }
}


