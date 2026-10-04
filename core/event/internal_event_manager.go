package event

import (
	"CuckooInterface/core/logger"
	"CuckooInterface/core/provider"
	"fmt"
	"sync"
	"time"
)

// stopTimeout 等待内部事件源停止的超时时间。
const stopTimeout = 5 * time.Second

// InternalEventManager 管理宿主内置的事件源。
type InternalEventManager struct {
	ebus           *EventBus
	log            *logger.Logger
	internalEvents map[string]provider.InternalEvent
	emitFunc       func(eventName string, payload string)

	lock sync.Mutex
}

func NewInternalEventManager(ebus *EventBus, log *logger.Logger) *InternalEventManager {
	return &InternalEventManager{
		ebus:           ebus,
		log:            log,
		emitFunc:       ebus.Emit,
		internalEvents: make(map[string]provider.InternalEvent),
	}
}

// AddInternalEvent 注册一个内部事件源：
func (iem *InternalEventManager) AddInternalEvent(event provider.InternalEvent) []error {
	if event == nil {
		return []error{fmt.Errorf("internal event is nil")}
	}

	iem.lock.Lock()
	defer iem.lock.Unlock()

	// 先注入依赖，再取元信息
	event.InjectAPI(iem.emitFunc, iem.log)
	id, names := event.GetMeta()
	if id == "" {
		return []error{fmt.Errorf("internal event provider id is empty")}
	}
	if _, exists := iem.internalEvents[id]; exists {
		return []error{fmt.Errorf("internal event provider %s already registered", id)}
	}

	var errS []error
	for _, name := range names {
		if err := iem.ebus.RegisterEvent(id, name); err != nil {
			errS = append(errS, fmt.Errorf("register event %s for provider %s: %w", name, id, err))
			continue
		}
	}

	iem.internalEvents[id] = event
	iem.logf("registered internal event provider %s with %d event(s)", id, len(names))
	return errS
}

// Start 启动所有已注册的内部事件源
func (iem *InternalEventManager) Start() {
	iem.lock.Lock()
	defer iem.lock.Unlock()
	for id, event := range iem.internalEvents {
		go func(id string, ev provider.InternalEvent) {
			// 单个事件源 panic 不应拖垮整个 Core
			defer func() {
				if r := recover(); r != nil {
					iem.logErrorf("internal event provider %s panicked: %v", id, r)
				}
			}()
			ev.StartService()
		}(id, event)
	}
}

// Stop 通知所有内部事件源停止，并同步等待它们退出。
func (iem *InternalEventManager) Stop() {
	iem.lock.Lock()
	var wg sync.WaitGroup
	for id, event := range iem.internalEvents {
		wg.Add(1)
		go func(id string, ev provider.InternalEvent) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					iem.logErrorf("internal event provider %s panicked on stop: %v", id, r)
				}
			}()
			ev.StopService()
		}(id, event)
	}
	// 先释放锁再等待，避免 StopService 期间阻塞其他查询
	iem.lock.Unlock()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(stopTimeout):
		iem.logErrorf("timed out waiting for internal event providers to stop")
	}
}

// logf 常规信息
func (iem *InternalEventManager) logf(format string, args ...any) {
	if iem.log != nil {
		iem.log.Infof(format, args...)
	}
}

// logErrorf 异常信息
func (iem *InternalEventManager) logErrorf(format string, args ...any) {
	if iem.log != nil {
		iem.log.Errorf(format, args...)
	}
}
