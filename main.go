package main

import (
	"CuckooInterface/core/constant"
	"context"
)

func main() {
	switch launchType {
	case constant.DameonModeArgData, "daemon":
		MainDaemonLoop()
	case constant.NormalModeArgData:
		MainCoreLoop(context.Background())
	default:
		MainDaemonLoop()
	}
}
