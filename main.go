package main

import (
	"CuckooInterface/core/constant"
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
