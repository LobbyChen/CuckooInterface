package dameon

import (
	"github.com/getlantern/systray"
	"github.com/getlantern/systray/example/icon" // 引入图标资源
)

func runTray(onStop func()) {
	systray.Run(onReady, onStop)
}

func onReady() {
	systray.SetIcon(icon.Data)
	systray.SetTitle("CuckooInterface 守护进程")

	// 创建菜单项
	mQuit := systray.AddMenuItem("退出", "退出守护")

	// 监听点击事件
	go func() {
		for {
			select {
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}
