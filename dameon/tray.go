package dameon

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/logger"
	"CuckooInterface/core/utils"
	"context"
	_ "embed"
	"fmt"

	"github.com/getlantern/systray"
)

//go:embed icon.ico
var trayIcon []byte

// SetupTray 初始化系统托盘菜单
func SetupTray(ctx context.Context) {
	// 设置图标
	systray.SetIcon(trayIcon)
	systray.SetTooltip("CuckooInterface 守护进程")

	mOpenUI := systray.AddMenuItem("打开界面", "启动或唤醒 CuckooInterfaceUI")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "停止 Core 并退出守护进程")

	go func() {
		for {
			select {
			case <-ctx.Done():
				// systray 退出或主进程关闭时，安全退出 Goroutine
				logger.GetLogger().Info("Tray listener goroutine exiting...")
				return

			case <-mOpenUI.ClickedCh:
				handleOpenUI()

			case <-mQuit.ClickedCh:
				logger.GetLogger().Info("Quit clicked from tray.")
				systray.Quit()
				return
			}
		}
	}()
}

// handleOpenUI 处理打开 UI 的逻辑
func handleOpenUI() {
	sm := GetStatusManager()
	if sm.IsUIRunning() {
		logger.GetLogger().Info("UI is already running, attempting to bring to front (Not implemented yet)...")
		// utils.BringWindowToFront("CuckooInterface")
		return
	}

	if err := StartUI(); err != nil {
		logger.GetLogger().Errorf("Failed to start UI: %v", err)
		_ = utils.ShowErrorBox(
			constant.ErrorMsgTitle,
			fmt.Sprintf("无法启动界面程序:\n%v", err),
		)
	}
}
