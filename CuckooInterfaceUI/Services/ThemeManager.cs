using System;
using System.Windows;
using System.Windows.Media;
using Microsoft.Win32;
using Wpf.Ui.Appearance;

namespace CuckooInterfaceUI.Services
{
    /// <summary>
    /// 统一管理应用主题（浅色 / 深色 / 跟随系统）与强调色。
    /// 通过 WPF-UI 的 ApplicationThemeManager / ApplicationAccentColorManager 切换资源字典。
    /// </summary>
    public static class ThemeManager
    {
        private const string RegistryThemePath =
            @"Software\Microsoft\Windows\CurrentVersion\Themes\Personalize";

        private const string RegistryLightThemeValue = "AppsUseLightTheme";

        /// <summary>
        /// 根据字符串值应用主题。
        /// </summary>
        /// <param name="themeValue">"light" / "dark" / "system"</param>
        public static void ApplyTheme(string themeValue)
        {
            var theme = ResolveTheme(themeValue);
            ApplicationThemeManager.Apply(theme);
        }

        /// <summary>
        /// 应用强调色。
        /// </summary>
        /// <param name="accentColorHex">十六进制颜色字符串，如 "#0078D4"</param>
        public static void ApplyAccent(string accentColorHex)
        {
            if (string.IsNullOrWhiteSpace(accentColorHex)) return;

            try
            {
                var color = (Color)ColorConverter.ConvertFromString(accentColorHex);
                ApplicationAccentColorManager.Apply(color, ApplicationThemeManager.GetAppTheme(), true);
            }
            catch
            {
                // 颜色解析失败时忽略，保持当前强调色。
            }
        }

        /// <summary>
        /// 将后端的字符串主题值解析为 WPF-UI 的 ApplicationTheme 枚举。
        /// "system" 会读取系统注册表判断当前 Windows 主题。
        /// </summary>
        private static ApplicationTheme ResolveTheme(string themeValue)
        {
            return (themeValue ?? string.Empty).ToLowerInvariant() switch
            {
                "light" => ApplicationTheme.Light,
                "dark" => ApplicationTheme.Dark,
                _ => GetSystemTheme()
            };
        }

        /// <summary>
        /// 读取 Windows 注册表判断系统当前是浅色还是深色主题。
        /// 读取失败时默认返回 Dark，与 App.xaml 初始主题保持一致。
        /// </summary>
        private static ApplicationTheme GetSystemTheme()
        {
            try
            {
                using var key = Registry.CurrentUser.OpenSubKey(RegistryThemePath);
                if (key?.GetValue(RegistryLightThemeValue) is int value)
                    return value == 0 ? ApplicationTheme.Dark : ApplicationTheme.Light;
            }
            catch
            {
                // 忽略注册表读取异常。
            }

            return ApplicationTheme.Dark;
        }
    }
}
