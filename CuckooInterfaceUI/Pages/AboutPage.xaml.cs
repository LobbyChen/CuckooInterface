// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
using System;
using System.Diagnostics;
using System.Windows;
using System.Windows.Controls;

namespace CuckooInterfaceUI.Pages
{
    /// <summary>
    /// AboutPage.xaml 的交互逻辑。
    /// </summary>
    public partial class AboutPage : Page
    {
        private const string GitHubUrl = "https://github.com/LobbyChen/CuckooInterface";

        public AboutPage()
        {
            InitializeComponent();
        }

        private void OpenGitHub_Click(object sender, RoutedEventArgs e)
        {
            try
            {
                Process.Start(new ProcessStartInfo(GitHubUrl)
                {
                    UseShellExecute = true
                });
            }
            catch (Exception ex)
            {
                (Window.GetWindow(this) as MainWindow)?.ShowToast("无法打开 GitHub", ex.Message, true);
            }
        }
    }
}
