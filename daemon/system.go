package daemon

import (
	"CuckooInterface/core/constant"
	"os"
	"sync"
	"time"
)

// StatusManager 管理守护进程的系统状态
type StatusManager struct {
	mu         sync.RWMutex
	startTime  time.Time
	version    string
	launchType string
	uiProcess  *os.Process
	uiRunning  bool
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

// SetUIProcess 记录 UI 进程并标记为运行中
func (sm *StatusManager) SetUIProcess(p *os.Process) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.uiProcess = p
	sm.uiRunning = p != nil
}

// IsUIRunning 检查 UI 是否正在运行
func (sm *StatusManager) IsUIRunning() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.uiRunning
}

// GetUIProcess 获取当前 UI 进程句柄
func (sm *StatusManager) GetUIProcess() *os.Process {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.uiProcess
}

// ClearUIProcess 清除 UI 进程记录（进程退出后调用）
func (sm *StatusManager) ClearUIProcess() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.uiProcess = nil
	sm.uiRunning = false
}
