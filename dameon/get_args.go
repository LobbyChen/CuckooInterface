package dameon

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/logger"
	"flag"
	"os"
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
func getBinaryPath() (string, error) {
	// 获取当前可执行文件的完整路径
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return execPath, err
}
