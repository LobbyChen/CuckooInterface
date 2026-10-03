package dameon

import (
	"CuckooInterface/core/constant"
	"flag"
)

// 获取启动类型
var s = flag.String(constant.ArgTag, "", "启动类型")

func GetStartType() *string {
	return s
}

func init() {
	flag.Parse()
	// 注意：不在此初始化全局 logger。
	// logger.Init 使用 sync.Once，若此处先用相对路径 "./log/dameon" 初始化，
	// 会导致 main 中 RunDaemonLoop/RunMainLoop 显式指定的绝对路径
	//（<exeDir>/logs/daemon、<exeDir>/logs）全部失效。
	// 全局 logger 的初始化交由各模式入口函数负责。
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
