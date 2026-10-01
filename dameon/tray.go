package dameon

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/logger"
	"CuckooInterface/core/utils"
	"fmt"

	"github.com/getlantern/systray"
)

// SetupTray 初始化系统托盘菜单
func SetupTray() {
	systray.SetTitle("CuckooInterface 守护进程")

	mOpenUI := systray.AddMenuItem("打开界面", "启动 CuckooInterfaceUI")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出", "退出守护进程")

	// 监听点击事件
	go func() {
		for {
			select {
			case <-mOpenUI.ClickedCh:
				if err := StartUI(); err != nil {
					logger.GetLogger().Errorf("Failed to start UI: %v", err)
					_ = utils.ShowErrorBox(
						constant.ErrorMsgTitle,
						fmt.Sprintf("无法启动界面程序:\n%v", err),
					)
				}

			// 退出守护进程
			case <-mQuit.ClickedCh:
				// 退出前先关闭 UI
				if err := StopUI(); err != nil {
					logger.GetLogger().Warnf("Failed to stop UI on quit: %v", err)
				}
				systray.Quit()
				return
			}
		}
	}()
}
