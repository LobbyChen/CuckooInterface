package dameon

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/logger"
	"CuckooInterface/core/utils"
	"fmt"
	"log"
	"os"
	"os/exec"
)

func startNormMode() error {
	// 检查是否以dameon启动
	if !isDameon() {
		return fmt.Errorf("Program is not started in dameon")
	}
	// 使用go程序以创建包
	binaryPath, err := getBinaryPath()
	if err != nil {
		return err
	}
	cmd := exec.Command(binaryPath, fmt.Sprintf("--%s", constant.ArgTag), constant.NormalModeArgData)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Start() // 启动子进程
	if err != nil {
		log.Fatal(err)
	}
	return cmd.Wait() // 等待子进程结束
}

func DameonLoop() error {
	stopCh := make(chan struct{})
	// 运行托盘
	go runTray(func() {
		close(stopCh)
	})
	for {
		select {
		case <-stopCh:
			// 利用taskill im结束后台进程
		default:
			// 拉起NormMode主程序
			err := startNormMode()
			if err != nil {
				logger.GetLogger().Error(err.Error())
			}
			// 若因为错误崩溃，显示msg
			err = utils.ShowErrorBox(constant.ErrorMsgTitle, fmt.Sprint(err))
			if err != nil {
				logger.GetLogger().Error(err.Error())
			}
		}
	}
}
