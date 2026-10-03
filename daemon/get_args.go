package daemon

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
