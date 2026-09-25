package plugin_manager

import (
	constantPkg "CuckooInterface/core/constant"
	"CuckooInterface/core/event"
	"CuckooInterface/core/logger"
	"CuckooInterface/plugins"
	"fmt"
)

type pluginDescriptor struct {
	p      *plugins.PluginDescriptor
	kernel *plugins.Kernel
	typo   constantPkg.PlugType
}

// PluginManager 上层插件管理器
type PluginManager struct {
	fileManager   *PluginFileManager
	kernelManager *KernelManager
	eventBus      *event.EventBus
	// Key: PluginID, Value: PluginHandle
	loadedPlugins map[string]pluginDescriptor
	logger        *logger.Logger
}

// NewPluginManager 创建插件管理器实例
func NewPluginManager(logger *logger.Logger, km *KernelManager, pfm *PluginFileManager, bus *event.EventBus) *PluginManager {
	return &PluginManager{
		kernelManager: km,
		fileManager:   pfm,
		eventBus:      bus,
		loadedPlugins: make(map[string]pluginDescriptor),
		logger:        logger,
	}
}
func (pm *PluginManager) checkEnv() bool {
	if pm.kernelManager == nil || pm.fileManager == nil || pm.eventBus == nil {
		return false
	}
	return true
}
func (pm *PluginManager) handleActive(plug pluginDescriptor, name string) error {
	// 注册事件监听。监听是 Active 插件的核心依赖，任意一个注册失败都应中止加载。
	var firstErr error
	for i := range plug.p.Listeners {
		err := pm.eventBus.RegisterListener(plug.p.Listeners[i], plug.kernel, plug.p.Handle())
		if err != nil {
			pm.logger.Error("plugin manager failed to register listener for plugin %s: %s", name, err.Error())
			if firstErr == nil {
				firstErr = fmt.Errorf("plugin %s failed to register listener %q: %w", name, plug.p.Listeners[i].EventName, err)
			}
		}
	}
	return firstErr
}
func (pm *PluginManager) handleBasePlugin(plug pluginDescriptor) error {
	// 注册事件产生。事件声明是 Base 插件的核心依赖
	var firstErr error
	for i := range plug.p.ProvidedEvents {
		err := pm.eventBus.RegisterEvent(plug.p.PluginID, plug.p.ProvidedEvents[i].EventName)
		if err != nil {
			pm.logger.Error("plugin manager failed to register event for plugin %s: %s", plug.p.PluginID, err.Error())
			if firstErr == nil {
				firstErr = fmt.Errorf("plugin %s failed to register event %q: %w", plug.p.PluginID, plug.p.ProvidedEvents[i].EventName, err)
			}
		}
	}
	return firstErr
}
func (pm *PluginManager) LoadPlugin(plugin *SinglePluginFile) error {
	// 检查前置依赖
	if !pm.checkEnv() {
		return fmt.Errorf("plugin manager is not initialized")
	}
	// 检测这个plugin是否已经被解压
	pluginFiles := append([]*SinglePluginFile{}, pm.fileManager.GetBasePlugins()...)
	pluginFiles = append(pluginFiles, pm.fileManager.GetActivePlugins()...)
	available := false
	for _, p := range pluginFiles {
		if p == plugin {
			available = true
		}
	}
	if !available {
		return fmt.Errorf("plugin %s not found in plugin manager", plugin.Name())
	}
	// 利用Km加载plugin
	kernel, plug, err := pm.kernelManager.LoadSinglePluginWithKernel(plugin)
	if err != nil {
		return err
	}
	descriptor := pluginDescriptor{
		p:      plug,
		kernel: kernel,
		typo:   plugin.Typo,
	}
	pm.loadedPlugins[plugin.Name()] = descriptor
	// 注册事件
	var regErr error
	switch plugin.Typo {
	case constantPkg.ACTIVE:
		regErr = pm.handleActive(descriptor, plugin.Name())
	case constantPkg.BASE:
		regErr = pm.handleBasePlugin(descriptor)
	default:
		regErr = fmt.Errorf("unknown plugin type: %d", plugin.Typo)
	}
	// 核心事件注册失败：回滚加载，从已加载列表移除并通知 Kernel 卸载
	if regErr != nil {
		// 先注销已注册的监听器/事件，避免 EventBus 持有悬空指针或永久占用事件名
		pm.eventBus.UnregisterPlugin(kernel, plug.Handle())
		pm.eventBus.UnregisterPluginEvents(plug.PluginID)
		delete(pm.loadedPlugins, plugin.Name())
		kernel.UnloadPlugin(plug.Handle())
		return regErr
	}
	return nil
}
func (pm *PluginManager) UnloadPlugin(plugin *SinglePluginFile) error {
	if !pm.checkEnv() {
		return fmt.Errorf("plugin manager is not initialized")
	}
	currentPlugin, ok := pm.loadedPlugins[plugin.Name()]
	if !ok {
		pm.logger.Error("plugin manager failed to unload plugin %s: not loaded", plugin.Name())
		return fmt.Errorf("plugin %s is not loaded", plugin.Name())
	}
	// 先从 EventBus 注销该插件的所有监听器和事件，避免持有已失效的 C 函数指针
	pm.eventBus.UnregisterPlugin(currentPlugin.kernel, currentPlugin.p.Handle())
	pm.eventBus.UnregisterPluginEvents(currentPlugin.p.PluginID)
	// 利用 kernel 释放插件实例
	currentPlugin.kernel.UnloadPlugin(currentPlugin.p.Handle())
	// 移除 map
	delete(pm.loadedPlugins, plugin.Name())
	return nil
}

// PluginSummary 插件摘要，供外部（如 IPC 查询）只读使用
type PluginSummary struct {
	Name      string               `json:"name"`
	PluginID  string               `json:"plugin_id"`
	Runtime   string               `json:"runtime"`
	Type      constantPkg.PlugType `json:"type"`
	Listeners []string             `json:"listeners"`
	Events    []string             `json:"provided_events"`
}

// ListLoadedPlugins 返回当前已加载插件的只读摘要列表
func (pm *PluginManager) ListLoadedPlugins() []PluginSummary {
	sums := make([]PluginSummary, 0, len(pm.loadedPlugins))
	for _, d := range pm.loadedPlugins {
		s := PluginSummary{
			Name:     d.p.PluginID,
			PluginID: d.p.PluginID,
			Runtime:  d.p.LanguageRuntime,
			Type:     d.typo,
		}
		for _, l := range d.p.Listeners {
			s.Listeners = append(s.Listeners, l.EventName)
		}
		for _, e := range d.p.ProvidedEvents {
			s.Events = append(s.Events, e.EventName)
		}
		sums = append(sums, s)
	}
	return sums
}

// GetEventNames 返回事件总线上已注册的全部事件名
func (pm *PluginManager) GetEventNames() []string {
	if pm.eventBus == nil {
		return nil
	}
	return pm.eventBus.GetEventNames()
}
