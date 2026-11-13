// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
using System;
using System.Windows;
using CuckooInterfaceUI.Services;

namespace CuckooInterfaceUI
{
    /// <summary>
    /// Interaction logic for App.xaml
    /// </summary>
    public partial class App : System.Windows.Application
    {
        protected override void OnStartup(StartupEventArgs e)
        {
            // 支持 -corepipe / -daemonpipe 指定命名管道，参数可为完整管道路径或裸管道名。
            CoreBackend.Instance.ConfigurePipeNames(
                GetArgumentValue(e.Args, "-corepipe"),
                GetArgumentValue(e.Args, "-daemonpipe"));

            base.OnStartup(e);
        }

        // 支持 "-name value" 与 "-name=value" 两种写法。
        private static string? GetArgumentValue(string[] args, string name)
        {
            for (var i = 0; i < args.Length; i++)
            {
                var arg = args[i];
                if (arg.Equals(name, StringComparison.OrdinalIgnoreCase))
                    return i + 1 < args.Length ? args[i + 1] : null;

                var prefix = name + "=";
                if (arg.StartsWith(prefix, StringComparison.OrdinalIgnoreCase))
                    return arg[prefix.Length..];
            }
            return null;
        }
    }

}


