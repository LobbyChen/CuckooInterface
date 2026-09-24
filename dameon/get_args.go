package dameon

import (
	"CuckooInterface/core/constant"
	"flag"
)

// 获取启动类型
var s = flag.String(constant.ArgTag, "", "启动类型")

func init() {
	flag.Parse()
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
