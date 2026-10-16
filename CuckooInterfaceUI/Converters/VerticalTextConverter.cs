// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
using System;
using System.Globalization;
using System.Linq;
using System.Windows.Data;

namespace CuckooInterfaceUI.Converters
{
    public sealed class VerticalTextConverter : IValueConverter
    {
        public object Convert(
            object value,
            Type targetType,
            object parameter,
            CultureInfo culture)
        {
            if (value == null)
                return string.Empty;

            var text = value.ToString() ?? string.Empty;

            if (string.IsNullOrWhiteSpace(text))
                return string.Empty;

            return string.Join(
                Environment.NewLine,
                text.Select(c => c.ToString()));
        }

        public object ConvertBack(
            object value,
            Type targetType,
            object parameter,
            CultureInfo culture)
        {
            throw new NotSupportedException();
        }
    }
}