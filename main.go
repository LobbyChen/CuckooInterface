// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/service"
	"CuckooInterface/daemon"
	"fmt"
	"os"
)

func main() {
	startType := daemon.GetStartType()

	switch *startType {
	case constant.NormalModeArgData:
		RunMainLoop()

	case constant.DameonModeArgData, "":
		RunDaemonLoop()

	case constant.ServiceModeArgData:
		RunServiceLoop()

	case constant.ServiceInstallArgData:
		if err := service.Install(""); err != nil {
			fmt.Fprintf(os.Stderr, "install service failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)

	case constant.ServiceUninstallArgData:
		if err := service.Uninstall(); err != nil {
			fmt.Fprintf(os.Stderr, "uninstall service failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)

	default:
		// 处理未知的启动类型
		fmt.Fprintf(os.Stderr, "Error: Unknown launch_type '%s'\n", *startType)
		fmt.Fprintf(os.Stderr, "Usage: %s -%s [%s|%s]\n",
			os.Args[0],
			constant.ArgTag,
			constant.NormalModeArgData,
			constant.DameonModeArgData)
		os.Exit(1)
	}
}
