package IPC

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/event"
	"CuckooInterface/core/logger"
	"CuckooInterface/core/plugin_manager"
	"CuckooInterface/core/utils"
	"CuckooInterface/plugins"
	"fmt"
	"sync"
	"time"

	pipe "gopkg.in/natefinch/npipe.v2"
)

// 与IPC的UI层接口定义

type CoreIpcInterface struct {
	km   *plugin_manager.KernelManager
	pfm  *plugin_manager.PluginFileManager
	pm   *plugin_manager.PluginManager
	ebus *event.EventBus
	once sync.Once

	logger   *logger.Logger
	listener *pipe.PipeListener
}

type singlePlugin struct {
	meta     plugins.PluginMetaData
	plugType constant.PlugType
	enable   bool
}

type singleEvent struct {
	name      string
	payload   string
	timestamp int64
}

func (ipc *CoreIpcInterface) init(km *plugin_manager.KernelManager, pfm *plugin_manager.PluginFileManager, pm *plugin_manager.PluginManager, ebus *event.EventBus, logger *logger.Logger) {
	ipc.once.Do(func() {
		ipc.km = km
		ipc.pfm = pfm
		ipc.pm = pm
		ipc.logger = logger
		ipc.ebus = ebus
	})
}

func (ipc *CoreIpcInterface) check() bool {
	if ipc.km == nil || ipc.pfm == nil || ipc.pm == nil {
		return false
	}
	return true
}

func (ipc *CoreIpcInterface) CreatePipe() error {
	l, err := pipe.Listen(constant.NamePipe)
	if err != nil {
		return err
	}
	ipc.listener = l
	return nil
}

func (ipc *CoreIpcInterface) DestroyPipe() error {
	return ipc.listener.Close()
}

func (ipc *CoreIpcInterface) waitForSentData(data []byte) error {
	conn, err := ipc.listener.Accept()
	if err != nil {
		return err
	}
	err = conn.SetWriteDeadline(time.Now().Add(time.Second * 1))
	if err != nil {
		return err
	}
	sentData := GeneralPkg(data)
	cnt, err := conn.Write(sentData)
	if err != nil {
		return err
	}
	if cnt != len(sentData) {
		return fmt.Errorf("expected to write %d bytes, wrote %d", len(data), cnt)
	}
	return nil
}

func (ipc *CoreIpcInterface) waitForReceivedData() ([]byte, error) {
	conn, err := ipc.listener.Accept()
	if err != nil {
		return nil, err
	}
	// 读取包
	err = conn.SetReadDeadline(time.Now().Add(time.Second * 1))
	if err != nil {
		return nil, err
	}
	recvData, err := ReadSinglePacket(conn)
	if err != nil {
		return nil, err
	}
	return recvData, nil
}

func (ipc *CoreIpcInterface) getAllPlugins() ([]singlePlugin, error) {
	allFiles := ipc.pfm.GetAllPlugins()
	ret := make([]singlePlugin, 0, len(allFiles))
	rMap := make(map[string]*singlePlugin, len(allFiles))

	// 映射slice和map
	for _, file := range allFiles {
		plugin := singlePlugin{
			meta:     file.Meta,
			plugType: file.Typo,
			enable:   false,
		}
		ret = append(ret, plugin)
		rMap[file.Meta.ID] = &ret[len(ret)-1]
	}

	// 获取所有已经启用的插件
	enabledKernel := ipc.km.GetInitKernelPlugins()
	enabledPlugin := ipc.pm.ListLoadedPlugins()

	// 筛选并标记
	for _, p := range enabledKernel {
		id, _, _, _ := p.Meta()
		if plugin, ok := rMap[id]; ok {
			plugin.enable = true
		}
	}

	for _, p := range enabledPlugin {
		id := p.PluginID
		if plugin, ok := rMap[id]; ok {
			plugin.enable = true
		}
	}

	return ret, nil
}

func (ipc *CoreIpcInterface) getLog() []logger.SingleLogRecord {
	return ipc.logger.GetCachedLogs()
}

func (ipc *CoreIpcInterface) getCachedEvents() []singleEvent {
	events := ipc.ebus.GetAllBufferedEvent()
	ret := make([]singleEvent, 0, len(events))
	for _, e := range events {
		ret = append(ret, singleEvent{
			name:      e.GetName(),
			payload:   e.GetName(),
			timestamp: e.GetTimeStamp(),
		})
	}
	return ret
}

func (ipc *CoreIpcInterface) NewPlugin(pluginType constant.PlugType, fp string) error {
	// 检查文件是否存在
	if !utils.IsFileExist(fp) {
		return fmt.Errorf("file %s not exist", fp)
	}
	// 加载
	return ipc.pfm.ExtractAndLoadExternalPlugin(fp, pluginType)
}

func (ipc *CoreIpcInterface) LoadPlugin(plugType constant.PlugType, id string) error {
	// 加载插件
	if plugType != constant.ACTIVE || plugType != constant.BASE || plugType != constant.KERNEL {
		return fmt.Errorf("unknown plugin type %v", plugType)
	}
	files := ipc.pfm.GetAllPlugins()
	var file *plugin_manager.SinglePluginFile
	// 遍历筛选
	for i := range files {
		if files[i].Meta.ID == id {
			file = files[i]
		}
	}
	if file == nil {
		return fmt.Errorf("plugin not found")
	}
	return ipc.pm.LoadPlugin(file)
}

func (ipc *CoreIpcInterface) UnloadPlugin(plugType constant.PlugType, id string) error {
	// 卸载插件
	if plugType != constant.ACTIVE || plugType != constant.BASE || plugType != constant.KERNEL {
		return fmt.Errorf("unknown plugin type %v", plugType)
	}
	files := ipc.pfm.GetAllPlugins()
	var file *plugin_manager.SinglePluginFile
	// 遍历筛选
	for i := range files {
		if files[i].Meta.ID == id {
			file = files[i]
		}
	}
	if file == nil {
		return fmt.Errorf("plugin not found")
	}
	// 卸载
	return ipc.pm.UnloadPlugin(file)
}
