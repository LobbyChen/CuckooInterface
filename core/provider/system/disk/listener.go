package disk

import (
	"CuckooInterface/core/logger"
	"encoding/json"
	"sync"
	"time"
)

const pollInterval = 3 * time.Second

// 内部事件源标识
const providerID = "system.diskChecker"

// 事件索引常量
const (
	eventDiskPlugged = iota // system.disk.plugged
	eventDiskUnplug         // system.disk.unplug
)

// events 声明本事件源提供的全部事件名。
var events = []string{
	"system.disk.plugged",
	"system.disk.unplug",
}

// payload 磁盘变化事件的载荷。
type payload struct {
	AllDisks     []diskInfo `json:"all_disks"`
	ChangedDisks []diskInfo `json:"changed_disks"`
}

// DiskMonitor 磁盘插拔监控
type DiskMonitor struct {
	emit     func(eventName string, payload string)
	log      *logger.Logger
	stopchan chan struct{}
	stopOnce sync.Once
}

// InjectAPI
func (h *DiskMonitor) InjectAPI(Emit func(eventName string, payload string), logger *logger.Logger) {
	h.log = logger
	h.emit = Emit
	h.stopchan = make(chan struct{})
}

// StartService 以固定间隔轮询磁盘列表
func (h *DiskMonitor) StartService() {
	prevDisks := getDisks()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopchan:
			return
		case <-ticker.C:
			currDisks := getDisks()
			removed, added := getDiff(prevDisks, currDisks)
			h.emitChanged(eventDiskUnplug, currDisks, removed)
			h.emitChanged(eventDiskPlugged, currDisks, added)
			prevDisks = currDisks
		}
	}
}

// emitChanged 序列化并发送一次磁盘变化事件，无变化时静默返回。
func (h *DiskMonitor) emitChanged(eventIdx int, all, changed []diskInfo) {
	if h.emit == nil || len(changed) == 0 {
		return
	}
	b, err := json.Marshal(payload{
		AllDisks:     all,
		ChangedDisks: changed,
	})
	if err != nil {
		if h.log != nil {
			h.log.Errorf("disk monitor: marshal payload failed: %v", err)
		}
		return
	}
	h.emit(events[eventIdx], string(b))
}

// GetMeta 返回事件源标识与其提供的事件名列表。
func (h *DiskMonitor) GetMeta() (string, []string) {
	cp := make([]string, len(events))
	copy(cp, events)
	return providerID, cp
}

// StopService 通知轮询循环退出
func (h *DiskMonitor) StopService() {
	h.stopOnce.Do(func() {
		if h.stopchan != nil {
			close(h.stopchan)
		}
	})
}
