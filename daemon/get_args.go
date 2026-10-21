// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package daemon

import (
	"CuckooInterface/core/constant"
	"flag"
)

// 获取启动类型
var s = flag.String(constant.ArgTag, "", "启动类型")

// noUI 为 true 时，Daemon 启动后不自动打开 UI（用于静默开机自启）。
var noUI = flag.Bool("no_ui", false, "启动守护进程时不自动打开 UI")

func GetStartType() *string {
	return s
}

// ShouldOpenUI 返回本次启动是否应自动显示 UI。
func ShouldOpenUI() bool {
	return !*noUI
}

func init() {
	flag.Parse()
}

func isDaemon() bool {
	switch *s {
	case constant.DameonModeArgData:
		return true
	case constant.NormalModeArgData:
		return true
	case "":
		// 默认采用dameon模式启动
		return true
	}
	return true
}
