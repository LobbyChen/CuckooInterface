package CuckooInterface

import (
	"CuckooInterface/core/logger"
	"context"
	_ "embed"

	"github.com/getlantern/systray"
)

//go:embed icon.ico
var trayIcon []byte

// SetupAndLaunchTray 初始化系统托盘菜单
func SetupAndLaunchTray(ctx context.Context, onQuit, onOpenUI func()) {
	// 设置图标
	systray.SetIcon(trayIcon)
	systray.SetTooltip("CuckooInterface 守护进程")

	mOpenUI := systray.AddMenuItem("打开界面", "启动或唤醒 CuckooInterfaceUI")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "停止 Core 并退出守护进程")

	for {
		select {
		case <-ctx.Done():
			logger.GetLogger().Info("Tray listener goroutine exiting...")
			return

		case <-mOpenUI.ClickedCh:
			onOpenUI()
		case <-mQuit.ClickedCh:
			logger.GetLogger().Info("Quit clicked from tray.")
			if onQuit != nil {
				onQuit()
			}
			systray.Quit()
			return
		}
	}
}
