package constant

const (
	INFO string = "INFO"
)

type PlugType byte

const (
	KERNEL PlugType = 0x0A
	ACTIVE PlugType = 0x1E
	BASE   PlugType = 0x2A
)

// String 返回插件类型的可读名称，用于日志与事件载荷
func (t PlugType) String() string {
	switch t {
	case KERNEL:
		return "kernel"
	case ACTIVE:
		return "active"
	case BASE:
		return "base"
	default:
		return "unknown"
	}
}

// 内部框架事件名（foundation.* 命名空间，见 core/provider/events.md）
const (
	EventFrameworkStarted  string = "foundation.framework.started"
	EventFrameworkStopping string = "foundation.framework.stopping"
	EventPluginLoaded      string = "foundation.plugin.loaded"
	EventPluginUnloaded    string = "foundation.plugin.unloaded"
)

// 项目所需的所有核心目录定义（面向代码阅读者的说明）：
//   - logs:        系统运行日志
//   - runtime:     Kernel 插件以及程序的运行时缓存
//   - plugin_temp: 插件解压与编译的临时工作区
//   - user:        用户数据根目录
//   - user/Active: 应用层插件
//   - user/Base:   基础层插件
//   - user/Kernel: 内核层插件
//   - plugins:     运行时维护的插件文件夹
//   - configs:     全局配置文件及插件配置备份
var Folders = []string{
	"logs",
	"user",
	"user/Active",
	"user/Base",
	"user/Kernel",
	"plugins",
	"configs",
}
var LogFolder = Folders[0]
var UserFolder = Folders[1]
var ActivePluginFolder = Folders[2]
var BasePluginFolder = Folders[3]
var KernelPluginFolder = Folders[4]
var RuntimePluginFolder = Folders[5]
var ConfigFolder = Folders[6]

// 插件内文件定义
const (
	MetaFile       string = "META-INF.json"
	KernelEntrance string = "binary/export.dll"
)

// dameon参数定义
const ArgTag = "launch_type"

const (
	NormalModeArgData = "normal"
	DameonModeArgData = "dameon"
)

// namepipe定义
const CoreNamePipe string = "\\\\.\\pipe\\CuckooCoreInterfaceBridge"
const DaemonNamePipe string = "\\\\.\\pipe\\CuckooDaemonInterfaceBridge"

// 错误消息定义
const ErrorMsgTitle string = "CuckooInterface遇到问题"
