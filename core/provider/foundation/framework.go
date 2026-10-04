// Package foundation 提供与插件框架自身生命周期相关的内部事件。
//
// 与 system.*（硬件/系统状态）不同，foundation.* 描述的是宿主的运行阶段与
// 插件装载情况。这些事件对插件而言和 Base 插件发布的事件完全等价，Active 插件
// 可以直接订阅 foundation.plugin.loaded 来感知其他插件的上线。
package foundation

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/logger"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// 内部事件源标识
const providerID = "foundation.framework"

// events 本事件源提供的全部事件名
var events = []string{
	constant.EventFrameworkStarted,
	constant.EventFrameworkStopping,
	constant.EventPluginLoaded,
	constant.EventPluginUnloaded,
}

// frameworkPayload framework.started / framework.stopping 的载荷
type frameworkPayload struct {
	StartedAt     int64   `json:"started_at"`
	UptimeSeconds float64 `json:"uptime_seconds"`
	PID           int     `json:"pid"`
}

// pluginPayload plugin.loaded / plugin.unloaded 的载荷
type pluginPayload struct {
	PluginID   string `json:"plugin_id"`
	PluginName string `json:"plugin_name"`
	PluginType string `json:"plugin_type"`
}

// FrameworkMonitor 框架生命周期事件源。
//
// 它自身只负责发出 started / stopping 两个事件；plugin.loaded 与
// plugin.unloaded 由外部（宿主在插件装载完成后）通过 NotifyPluginLoaded /
// NotifyPluginUnloaded 主动触发，因为插件装载发生在 PluginManager 里，
// 事件源本身无从感知。
type FrameworkMonitor struct {
	emit     func(eventName string, payload string)
	log      *logger.Logger
	start    time.Time
	stopchan chan struct{}
	stopOnce sync.Once
	mu       sync.Mutex
}

// InjectAPI 由 InternalEventManager 调用，注入发送能力与日志器。
func (f *FrameworkMonitor) InjectAPI(Emit func(eventName string, payload string), logger *logger.Logger) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = logger
	f.emit = Emit
	f.start = time.Now()
	f.stopchan = make(chan struct{})
}

// StartService 发出 framework.started 后阻塞，直到 StopService 被调用。
func (f *FrameworkMonitor) StartService() {
	f.emitFrameworkEvent(constant.EventFrameworkStarted)
	<-f.stopchan
}

// StopService 发出 framework.stopping 并结束 StartService 的阻塞。
// 可安全重复调用。
func (f *FrameworkMonitor) StopService() {
	f.stopOnce.Do(func() {
		f.emitFrameworkEvent(constant.EventFrameworkStopping)
		if f.stopchan != nil {
			close(f.stopchan)
		}
	})
}

// GetMeta 返回事件源标识与事件名列表（返回副本，避免调用方误改）。
func (f *FrameworkMonitor) GetMeta() (string, []string) {
	cp := make([]string, len(events))
	copy(cp, events)
	return providerID, cp
}

// NotifyPluginLoaded 供宿主在插件装载成功后调用，发出 plugin.loaded。
func (f *FrameworkMonitor) NotifyPluginLoaded(id, name string, typo constant.PlugType) {
	f.emitPluginEvent(constant.EventPluginLoaded, id, name, typo)
}

// NotifyPluginUnloaded 供宿主在插件卸载成功后调用，发出 plugin.unloaded。
func (f *FrameworkMonitor) NotifyPluginUnloaded(id, name string, typo constant.PlugType) {
	f.emitPluginEvent(constant.EventPluginUnloaded, id, name, typo)
}

// emitFrameworkEvent 序列化并发送框架级事件
func (f *FrameworkMonitor) emitFrameworkEvent(name string) {
	f.mu.Lock()
	emit, log, start := f.emit, f.log, f.start
	f.mu.Unlock()

	if emit == nil {
		return
	}
	p := frameworkPayload{
		StartedAt:     start.Unix(),
		UptimeSeconds: time.Since(start).Seconds(),
		PID:           os.Getpid(),
	}
	b, err := json.Marshal(p)
	if err != nil {
		if log != nil {
			log.Errorf("foundation: marshal %s payload failed: %v", name, err)
		}
		return
	}
	emit(name, string(b))
}

// emitPluginEvent 序列化并发送插件级事件
func (f *FrameworkMonitor) emitPluginEvent(name, id, pluginName string, typo constant.PlugType) {
	f.mu.Lock()
	emit, log := f.emit, f.log
	f.mu.Unlock()

	if emit == nil {
		return
	}
	b, err := json.Marshal(pluginPayload{
		PluginID:   id,
		PluginName: pluginName,
		PluginType: typo.String(),
	})
	if err != nil {
		if log != nil {
			log.Errorf("foundation: marshal %s payload failed: %v", name, err)
		}
		return
	}
	emit(name, string(b))
}
