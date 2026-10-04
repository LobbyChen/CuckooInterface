package event

import (
	"CuckooInterface/core/utils"
	"CuckooInterface/plugins"
	"errors"
	"fmt"
	"sync"
	"time"
)

type receiverEntry struct {
	record plugins.ListenerRecord
	kernel *plugins.Kernel
	handle plugins.PluginHandle // 对应 C 侧的 CuckooPluginHandle
}

// event 代表一个已注册的事件通道
type event struct {
	name       string
	providerID string
	receivers  []receiverEntry
	lock       sync.RWMutex
}

// EventBus 核心事件总线
type EventBus struct {
	events     map[string]*event
	globalLock sync.Mutex
	once       sync.Once
	// 事件缓冲区
	eventQueue *utils.RingQueue[*EmitTask]
	// Worker 池
	taskCh      chan EmitTask
	workerCount int

	// 今日事件计数
	statsMu         sync.Mutex
	statsDay        string
	todayEventCount int64
}

// EmitTask 表示一次事件触发任务
type EmitTask struct {
	rec       receiverEntry
	eventName string
	payload   string
	timestamp time.Time
}

func (e EmitTask) GetName() string {
	return e.eventName
}
func (e EmitTask) GetTimeStamp() int64 {
	return e.timestamp.Unix()
}
func (e EmitTask) GetPayLoad() string {
	return e.payload
}

// 默认 Worker 数量与任务队列容量
const (
	defaultWorkerCount = 16
	defaultTaskQueue   = 1024
)

func NewEventBus() *EventBus {
	bus := &EventBus{
		events:      make(map[string]*event),
		taskCh:      make(chan EmitTask, defaultTaskQueue),
		workerCount: defaultWorkerCount,
		eventQueue:  utils.NewRingQueue[*EmitTask](defaultTaskQueue * defaultWorkerCount),
	}
	// 启动固定数量的 Worker Goroutine 消费事件任务
	for i := 0; i < bus.workerCount; i++ {
		go bus.eventWorker()
	}
	return bus
}

// eventWorker 消费事件任务队列
func (bus *EventBus) eventWorker() {
	for task := range bus.taskCh {
		func() {
			defer func() {
				if err := recover(); err != nil {
					fmt.Printf("[EventBus] Panic in listener for event %s: %v\n", task.eventName, err)
				}
			}()
			task.rec.kernel.TriggerCallback(task.rec.handle, &task.rec.record, task.payload)
		}()
	}
}

// RegisterEvent 由插件提供者声明一个事件
func (bus *EventBus) RegisterEvent(providerID string, name string) error {
	if providerID == "" || name == "" {
		return errors.New("provider ID or event name is empty")
	}
	bus.globalLock.Lock()
	defer bus.globalLock.Unlock()
	if _, exists := bus.events[name]; exists {
		return fmt.Errorf("event %s already registered by provider %s", name, providerID)
	}
	bus.events[name] = &event{
		name:       name,
		providerID: providerID,
		receivers:  make([]receiverEntry, 0),
	}
	return nil
}

// RegisterListener 注册监听器并绑定到具体的 Kernel 和 Plugin Handle
func (bus *EventBus) RegisterListener(receiver plugins.ListenerRecord, kernel *plugins.Kernel, handle plugins.PluginHandle) error {
	if kernel == nil {
		return errors.New("kernel cannot be nil")
	}
	bus.globalLock.Lock()
	evt, exists := bus.events[receiver.EventName]
	bus.globalLock.Unlock()
	if !exists {
		return fmt.Errorf("event %s has not been registered yet", receiver.EventName)
	}
	evt.lock.Lock()
	defer evt.lock.Unlock()
	// 去重检查
	for _, r := range evt.receivers {
		if r.record.Function == receiver.Function && r.kernel == kernel {
			return nil // 已经注册过
		}
	}
	evt.receivers = append(evt.receivers, receiverEntry{
		record: receiver,
		kernel: kernel,
		handle: handle,
	})
	return nil
}

// UnregisterPlugin 移除指定 Kernel + PluginHandle 关联的所有监听器。
func (bus *EventBus) UnregisterPlugin(kernel *plugins.Kernel, handle plugins.PluginHandle) {
	bus.globalLock.Lock()
	events := make([]*event, 0, len(bus.events))
	for _, evt := range bus.events {
		events = append(events, evt)
	}
	bus.globalLock.Unlock()
	for _, evt := range events {
		evt.lock.Lock()
		filtered := evt.receivers[:0]
		for _, r := range evt.receivers {
			// 保留不属于该插件的监听器
			if !(r.kernel == kernel && r.handle == handle) {
				filtered = append(filtered, r)
			}
		}
		evt.receivers = filtered
		evt.lock.Unlock()
	}
}

// UnregisterPluginEvents 移除指定 provider 注册的所有事件。
// Base 插件卸载或加载失败回滚时必须调用
func (bus *EventBus) UnregisterPluginEvents(providerID string) {
	bus.globalLock.Lock()
	defer bus.globalLock.Unlock()
	for name, evt := range bus.events {
		if evt.providerID == providerID {
			delete(bus.events, name)
		}
	}
}

// Emit 触发事件 通知所有订阅者
func (bus *EventBus) Emit(eventName string, payload string) {
	bus.globalLock.Lock()
	evt, exists := bus.events[eventName]
	bus.globalLock.Unlock()
	if !exists {
		return
	}
	// 累加后端维护的今日事件计数（仅统计有效投递的已注册事件）
	bus.countTodayEvent()
	evt.lock.RLock()
	receivers := make([]receiverEntry, len(evt.receivers))
	copy(receivers, evt.receivers)
	// 投递到buffer
	bus.eventQueue.Push(&EmitTask{rec: receiverEntry{}, eventName: eventName, payload: payload, timestamp: time.Now()})
	evt.lock.RUnlock()
	// 将每个接收者的触发任务投递到 Worker 队列
	for _, rec := range receivers {
		task := EmitTask{eventName: eventName, rec: rec, payload: payload}
		select {
		case bus.taskCh <- task:
		default:
			// 队列已满，打印警告并丢弃该次触发，避免阻塞事件源
			fmt.Printf("[EventBus] Task queue full, dropping event %s for one listener\n", eventName)
		}
	}
}

// GetEventNames 获取所有已注册的事件名
func (bus *EventBus) GetEventNames() []string {
	bus.globalLock.Lock()
	defer bus.globalLock.Unlock()
	names := make([]string, 0, len(bus.events))
	for name := range bus.events {
		names = append(names, name)
	}
	return names
}

// RegisteredEventInfo 描述一个已注册事件的概要信息（供 IPC 返回给前端）
type RegisteredEventInfo struct {
	Name          string
	ProviderID    string
	ListenerCount int
}

// GetRegisteredEventsInfo 返回所有已注册事件的名称、提供者和监听者数量
func (bus *EventBus) GetRegisteredEventsInfo() []RegisteredEventInfo {
	bus.globalLock.Lock()
	events := make([]*event, 0, len(bus.events))
	for _, evt := range bus.events {
		events = append(events, evt)
	}
	bus.globalLock.Unlock()

	infos := make([]RegisteredEventInfo, 0, len(events))
	for _, evt := range events {
		evt.lock.RLock()
		infos = append(infos, RegisteredEventInfo{
			Name:          evt.name,
			ProviderID:    evt.providerID,
			ListenerCount: len(evt.receivers),
		})
		evt.lock.RUnlock()
	}
	return infos
}

// GetAllBufferedEvent 获取所有已经缓冲的事件
func (bus *EventBus) GetAllBufferedEvent() []*EmitTask {
	if bus.eventQueue.Len() != 0 {
		return bus.eventQueue.GetAll()
	}
	return nil
}

// countTodayEvent 累加今日事件计数，跨天自动清零。
func (bus *EventBus) countTodayEvent() {
	today := time.Now().Format("2006-01-02")
	bus.statsMu.Lock()
	defer bus.statsMu.Unlock()
	if bus.statsDay != today {
		bus.statsDay = today
		bus.todayEventCount = 0
	}
	bus.todayEventCount++
}

// GetTodayEventCount 返回后端维护的今日事件计数（跨天自动清零）。
// 该计数独立于事件环形缓冲区，不受其容量与覆盖策略影响。
func (bus *EventBus) GetTodayEventCount() int64 {
	today := time.Now().Format("2006-01-02")
	bus.statsMu.Lock()
	defer bus.statsMu.Unlock()
	if bus.statsDay != today {
		bus.statsDay = today
		bus.todayEventCount = 0
	}
	return bus.todayEventCount
}
