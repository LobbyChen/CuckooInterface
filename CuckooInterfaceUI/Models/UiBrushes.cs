// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
using System.Windows.Media;

namespace CuckooInterfaceUI.Models
{
    /// <summary>
    /// 模型层共用的状态颜色，避免每次读取属性都新建 <see cref="SolidColorBrush"/>。
    /// </summary>
    internal static class UiBrushes
    {
        public static readonly Brush Success = Create(0x63, 0xD3, 0x8B);
        public static readonly Brush Warning = Create(0xF2, 0xC3, 0x5E);
        public static readonly Brush Starting = Create(0xFF, 0xB9, 0x00);
        public static readonly Brush Danger = Create(0xFF, 0x72, 0x72);
        public static readonly Brush Neutral = Create(0x9A, 0xA0, 0xA6);

        private static Brush Create(byte r, byte g, byte b) =>
            new SolidColorBrush(Color.FromRgb(r, g, b));
    }
}
