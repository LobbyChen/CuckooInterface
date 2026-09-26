package dameon

import (
	"CuckooInterface/core/constant"
	"sync"
	"time"
)

// StatusManager 管理守护进程的系统状态
type StatusManager struct {
	mu         sync.RWMutex
	startTime  time.Time
	version    string
	launchType string
}

var (
	statusManagerInstance *StatusManager
	statusManagerOnce     sync.Once
)

// GetStatusManager 获取单例 StatusManager
func GetStatusManager() *StatusManager {
	statusManagerOnce.Do(func() {
		sm := &StatusManager{
			startTime: time.Now(),
		}

		// 初始化启动类型
		if isDaemon() {
			sm.launchType = constant.DameonModeArgData
		} else {
			sm.launchType = constant.NormalModeArgData
		}

		statusManagerInstance = sm
	})
	return statusManagerInstance
}

// GetStartedTime 获取程序运行时长
func (sm *StatusManager) GetStartedTime() time.Duration {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return time.Since(sm.startTime)
}

// SetVersion 设置版本号
func (sm *StatusManager) SetVersion(v string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.version = v
}

// GetVersion 获取版本号
func (sm *StatusManager) GetVersion() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.version
}

// GetLaunchType 获取启动类型 (daemon/normal)
func (sm *StatusManager) GetLaunchType() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.launchType
}

// GetStatusMap 获取所有状态的快照，方便 IPC 或日志使用
func (sm *StatusManager) GetStatusMap() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return map[string]interface{}{
		"version":    sm.version,
		"uptime":     time.Since(sm.startTime).String(),
		"start_time": sm.startTime.Format(time.RFC3339),
		"mode":       sm.launchType,
	}
}
