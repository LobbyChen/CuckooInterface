package main

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/utils"
	"CuckooInterface/dameon"
	"flag"
	"fmt"
	"os"

	"github.com/getlantern/systray"
)

const (
	daemonPort int = 25567
	corePort   int = 25568
)

func main() {
	flag.Parse()

	mode := *dameon.GetStartType()
	if mode == "" {
		mode = constant.DameonModeArgData
	}

	switch mode {
	case constant.NormalModeArgData:
		utils.EnsureSingleInstance(corePort)
		runNormalMode()
	case constant.DameonModeArgData:
		utils.EnsureSingleInstance(daemonPort)
		runDaemonMode()
	default:
		utils.EnsureSingleInstance(daemonPort)
		runDaemonMode()
	}
}

// runNormalMode 运行纯核心模式
func runNormalMode() {
	coreApp := NewCoreApp()
	if err := coreApp.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init core: %v\n", err)
		os.Exit(1)
	}

	coreApp.Run()
}

// runDaemonMode 运行守护进程模式
func runDaemonMode() {
	coreApp := NewCoreApp()
	daemonApp := NewDaemonApp(coreApp)

	if err := daemonApp.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init daemon: %v\n", err)
		os.Exit(1)
	}

	systray.Run(func() {
		// onReady
		go daemonApp.Run()
	}, func() {
		// onExit
		daemonApp.Shutdown()
	})
}
