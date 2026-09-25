package dameon

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/logger"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
)

// 获取启动类型
var s = flag.String(constant.ArgTag, "", "启动类型")

func init() {
	flag.Parse()
	logger.Init(logger.LoggerConfig{
		Level:         "info",
		LogDir:        "./log/dameon",
		MaxSize:       100, // MB
		MaxBackups:    7,
		MaxAge:        30, // days
		Compress:      false,
		ConsoleOutput: true,
	})
}

func isDameon() bool {
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
func getBinaryPath() (string, error) {
	// 获取当前可执行文件的完整路径
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return execPath, err
}

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
	for {
		// 拉起NormMode主程序
		err := startNormMode()
		logger.GetLogger().Error(err.Error())
		// 若因为错误崩溃，重启
	}

}
