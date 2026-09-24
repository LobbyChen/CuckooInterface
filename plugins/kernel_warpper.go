package plugins

/*
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <stdbool.h>
#include "cuckoo_kernel.h"
#include "cuckoo_plugin.h"
#include "sandbox_host.h"
// 前置声明 Go 导出函数
extern void go_emit_event_gateway(char* eventName, char* jsonPayload);
extern void go_log_info_gateway(char* msg);
extern void go_log_error_gateway(char* msg);
extern char* go_get_plugin_config_gateway(char* plug_name);
extern void go_into_loop_report_gateway(char* kernel_id);
extern void go_panic_gateway(char* reason);
// 桥接函数
static void c_emit_event_bridge(const char* eventName, const char* jsonPayload) {
go_emit_event_gateway((char*)eventName, (char*)jsonPayload);
}
static void c_log_info_bridge(const char* msg) {
go_log_info_gateway((char*)msg);
}
static void c_log_error_bridge(const char* msg) {
go_log_error_gateway((char*)msg);
}
static const char* c_get_plugin_config_bridge(const char* plug_name) {
return (const char*)go_get_plugin_config_gateway((char*)plug_name);
}
static void c_into_loop_report_bridge(const char* kernel_id) {
go_into_loop_report_gateway((char*)kernel_id);
}
static void c_panic_bridge(const char* reason) {
go_panic_gateway((char*)reason);
}
// Getter 函数
typedef void (*emit_event_fn)(const char*, const char*);
typedef void (*log_fn)(const char*);
typedef const char* (*get_config_fn)(const char*);
typedef void (*loop_report_fn)(const char*);
typedef void (*panic_fn)(const char*);
static emit_event_fn get_emit_event_bridge(void)       { return c_emit_event_bridge; }
static log_fn        get_log_info_bridge(void)          { return c_log_info_bridge; }
static log_fn        get_log_error_bridge(void)         { return c_log_error_bridge; }
static get_config_fn get_get_plugin_config_bridge(void) { return c_get_plugin_config_bridge; }
static loop_report_fn get_into_loop_report_bridge(void) { return c_into_loop_report_bridge; }
static panic_fn      get_panic_bridge(void)             { return c_panic_bridge; }
*/
import "C"
import (
	"CuckooInterface/core/logger"
	"fmt"
	"sync"
	"time"
	"unsafe"
)

// Go 类型定义

type PluginHandle unsafe.Pointer
type InstanceID uint64
type EventRecord struct {
	EventName string
}
type ListenerRecord struct {
	EventName string
	Function  unsafe.Pointer
}
type PluginDescriptor struct {
	InstanceID        InstanceID
	PluginID          string
	LanguageRuntime   string
	Listeners         []ListenerRecord
	ProvidedEvents    []EventRecord
	InternalObjectPtr unsafe.Pointer
}

// HostAPI 修正：移除 GetConfig，增加 IntoLoopReport 和 Panic
type HostAPI struct {
	EmitEvent       func(eventName, payload string)
	LogInfo         func(msg string)
	LogError        func(msg string)
	GetPluginConfig func(plugin_name string) string
	IntoLoopReport  func(kernel_id string)
	Panic           func(reason string)
}
type Kernel struct {
	sandboxHandle unsafe.Pointer
	hostAPI       *C.CuckooHostAPI
	mu            sync.Mutex
	closed        bool
	loopStartedCh chan struct{} // 用于同步等待进入循环
}

var globalHostAPI *HostAPI
var hostApiOnce sync.Once

func InjectHostAPI(api *HostAPI) {
	hostApiOnce.Do(func() {
		globalHostAPI = api
	})
}

// C 字符串环形缓冲区
const cStringRingSize = 64

var (
	cStringRing    [cStringRingSize]*C.char
	cStringRingIdx int
	cStringRingMu  sync.Mutex
)

// allocReturnedCString 分配一个供 C 侧读取的 C 字符串，托管其生命周期。
func allocReturnedCString(val string) *C.char {
	cStringRingMu.Lock()
	defer cStringRingMu.Unlock()
	// 释放当前槽位的旧字符串
	if cStringRing[cStringRingIdx] != nil {
		C.free(unsafe.Pointer(cStringRing[cStringRingIdx]))
		cStringRing[cStringRingIdx] = nil
	}
	cs := C.CString(val)
	cStringRing[cStringRingIdx] = cs
	cStringRingIdx = (cStringRingIdx + 1) % cStringRingSize
	return cs
}

// 导出网关函数

//export go_emit_event_gateway
func go_emit_event_gateway(eventName *C.char, jsonPayload *C.char) {
	if globalHostAPI != nil && globalHostAPI.EmitEvent != nil {
		globalHostAPI.EmitEvent(C.GoString(eventName), C.GoString(jsonPayload))
	}
}

//export go_log_info_gateway
func go_log_info_gateway(msg *C.char) {
	if globalHostAPI != nil && globalHostAPI.LogInfo != nil {
		globalHostAPI.LogInfo(C.GoString(msg))
	}
}

//export go_log_error_gateway
func go_log_error_gateway(msg *C.char) {
	if globalHostAPI != nil && globalHostAPI.LogError != nil {
		globalHostAPI.LogError(C.GoString(msg))
	}
}

//export go_get_plugin_config_gateway
func go_get_plugin_config_gateway(plugname *C.char) *C.char {
	if globalHostAPI != nil && globalHostAPI.GetPluginConfig != nil {
		val := globalHostAPI.GetPluginConfig(C.GoString(plugname))
		return allocReturnedCString(val)
	}
	return nil
}

//export go_into_loop_report_gateway
func go_into_loop_report_gateway(kernelID *C.char) {
	id := C.GoString(kernelID)
	// 调用上层业务逻辑
	if globalHostAPI != nil && globalHostAPI.IntoLoopReport != nil {
		globalHostAPI.IntoLoopReport(id)
	}
}

//export go_panic_gateway
func go_panic_gateway(reason *C.char) {
	if globalHostAPI != nil && globalHostAPI.Panic != nil {
		globalHostAPI.Panic(C.GoString(reason))
	}
}

// 核心 API

func (k *Kernel) Meta() (kernelID, kernelName, version string, runtime string) {
	cID := make([]byte, 64)
	cName := make([]byte, 64)
	cVer := make([]byte, 32)
	cRt := make([]byte, 32)
	C.sandbox_get_meta(k.sandboxHandle,
		(*C.char)(unsafe.Pointer(&cID[0])),
		(*C.char)(unsafe.Pointer(&cName[0])),
		(*C.char)(unsafe.Pointer(&cVer[0])),
		(*C.char)(unsafe.Pointer(&cRt[0])))
	return C.GoString((*C.char)(unsafe.Pointer(&cID[0]))),
		C.GoString((*C.char)(unsafe.Pointer(&cName[0]))),
		C.GoString((*C.char)(unsafe.Pointer(&cVer[0]))),
		C.GoString((*C.char)(unsafe.Pointer(&cRt[0])))
}
func (k *Kernel) InitRuntime(api *HostAPI) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.closed {
		return fmt.Errorf("cuckoo: kernel already closed")
	}
	if k.hostAPI != nil {
		return fmt.Errorf("cuckoo: kernel already initialized")
	}
	cAPI := (*C.CuckooHostAPI)(C.malloc(C.sizeof_CuckooHostAPI))
	if cAPI == nil {
		return fmt.Errorf("cuckoo: failed to allocate host API table")
	}
	C.memset(unsafe.Pointer(cAPI), 0, C.sizeof_CuckooHostAPI)
	// 修正绑定：移除 get_config，绑定 into_loop_report 和 panic
	cAPI.emit_event = (*[0]byte)(unsafe.Pointer(C.get_emit_event_bridge()))
	cAPI.log_info = (*[0]byte)(unsafe.Pointer(C.get_log_info_bridge()))
	cAPI.log_error = (*[0]byte)(unsafe.Pointer(C.get_log_error_bridge()))
	cAPI.get_plugin_config = (*[0]byte)(unsafe.Pointer(C.get_get_plugin_config_bridge()))
	cAPI.into_loop_report = (*[0]byte)(unsafe.Pointer(C.get_into_loop_report_bridge()))
	cAPI.panic = (*[0]byte)(unsafe.Pointer(C.get_panic_bridge()))
	rc := C.sandbox_init_runtime(k.sandboxHandle, cAPI)
	if rc != 0 {
		C.free(unsafe.Pointer(cAPI))
		k.handleCrashIfNeeded()
		return fmt.Errorf("cuckoo: init_runtime failed with code %d", int(rc))
	}
	k.hostAPI = cAPI
	return nil
}
func (k *Kernel) LoadPlugin(pluginPath, manifestJSON string) (*PluginDescriptor, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.closed {
		return nil, fmt.Errorf("cuckoo: kernel already closed")
	}
	cPath := C.CString(pluginPath)
	defer C.free(unsafe.Pointer(cPath))
	cManifest := C.CString(manifestJSON)
	defer C.free(unsafe.Pointer(cManifest))
	var cDesc C.CuckooPluginDescriptor
	cDesc.listener_count = 0
	rc := C.sandbox_load_plugin(k.sandboxHandle, cPath, cManifest, &cDesc)
	if rc != 0 {
		k.handleCrashIfNeeded() // load_plugin 失败可能由 Kernel 崩溃导致
		return nil, fmt.Errorf("cuckoo: load_plugin failed with code %d", int(rc))
	}
	return convertDescriptor(&cDesc), nil
}
func (k *Kernel) TriggerCallback(handle PluginHandle, listener *ListenerRecord, jsonPayload string) {
	// 事件 Worker 池异步调用
	if k.sandboxHandle == nil {
		return
	}
	cPayload := C.CString(jsonPayload)
	defer C.free(unsafe.Pointer(cPayload))
	var cListener C.CuckooListenerRecord
	cEventName := C.CString(listener.EventName)
	defer C.free(unsafe.Pointer(cEventName))
	C.strncpy((*C.char)(unsafe.Pointer(&cListener.event_name[0])), cEventName, 127)
	cListener.event_name[127] = 0
	cListener.function = listener.Function
	C.sandbox_trigger_callback(k.sandboxHandle, C.CuckooPluginHandle(handle), &cListener, cPayload)
}
func (k *Kernel) UnloadPlugin(handle PluginHandle) {
	C.sandbox_unload_plugin(k.sandboxHandle, C.CuckooPluginHandle(handle))
}
func (k *Kernel) GetChan() chan struct{} {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.loopStartedCh
}

// StartLoop 同步启动 Kernel 循环
func (k *Kernel) StartLoop() error {
	crashCh := make(chan struct{})
	go func() {
		C.sandbox_wait_for_crash(k.sandboxHandle)
		close(crashCh)
	}()
	// 独立调用 C 层的异步启动函数
	go func() {
		C.sandbox_start_loop_async(k.sandboxHandle, k.hostAPI)
	}()
	// 等待 into_loop_report 被调用，崩溃事件触发，或者超时
	select {
	case <-k.loopStartedCh:
		_, name, _, _ := k.Meta()
		logger.GetLogger().Infof("kernel %s entered loop", name)
		return nil // 成功进入循环
	case <-crashCh:
		reason := k.getCrashReason()
		if globalHostAPI != nil && globalHostAPI.Panic != nil {
			globalHostAPI.Panic(reason)
		}
		return fmt.Errorf("cuckoo: kernel crashed before entering loop: %s", reason)
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timeout waiting for kernel to enter loop")
	}
}
func (k *Kernel) ShutdownRuntime() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.closed {
		return
	}
	C.sandbox_shutdown(k.sandboxHandle)
	if k.hostAPI != nil {
		C.free(unsafe.Pointer(k.hostAPI))
		k.hostAPI = nil
	}

	k.closed = true
}
func (k *Kernel) IsAlive() bool {
	return C.is_sandbox_alive(k.sandboxHandle) == 1
}

// getCrashReason 从 C 侧读取 VEH 记录的崩溃原因
func (k *Kernel) getCrashReason() string {
	cbuf := make([]byte, 128)
	C.sandbox_get_crash_reason(k.sandboxHandle, (*C.char)(unsafe.Pointer(&cbuf[0])), C.int(len(cbuf)))
	return C.GoString((*C.char)(unsafe.Pointer(&cbuf[0])))
}

// handleCrashIfNeeded 在 Go 上下文中检查并处理崩溃
func (k *Kernel) handleCrashIfNeeded() {
	if k.IsAlive() {
		return
	}
	reason := k.getCrashReason()
	if reason == "" {
		reason = "kernel crashed (unknown reason)"
	}
	if globalHostAPI != nil && globalHostAPI.Panic != nil {
		globalHostAPI.Panic(reason)
	}
}
func (k *Kernel) Destroy() {
	if k.sandboxHandle != nil {
		C.destroy_sandbox(k.sandboxHandle)
		k.sandboxHandle = nil
	}
}
func (d *PluginDescriptor) Handle() PluginHandle {
	return PluginHandle(d.InternalObjectPtr)
}

// cbufStr 将定长 C char 数组转换为 Go string
func cbufStr(p *C.char, maxLen C.size_t) string {
	n := C.strnlen(p, maxLen)
	return C.GoStringN(p, C.int(n))
}
func convertDescriptor(c *C.CuckooPluginDescriptor) *PluginDescriptor {
	desc := &PluginDescriptor{
		InstanceID:        InstanceID(c.instance_id),
		PluginID:          cbufStr((*C.char)(unsafe.Pointer(&c.plugin_id[0])), 64),
		LanguageRuntime:   cbufStr((*C.char)(unsafe.Pointer(&c.language_runtime[0])), 32),
		InternalObjectPtr: c.internal_object_ptr,
	}
	n := int(c.listener_count)
	if n < 0 || n > C.MAX_LISTENERS_PER_PLUGIN {
		// C 端返回越界 count，防御性截断并留空，避免越界访问 listeners 数组
		n = 0
	}
	if n > 0 {
		desc.Listeners = make([]ListenerRecord, n)
		for i := 0; i < n; i++ {
			l := &c.listeners[i]
			desc.Listeners[i] = ListenerRecord{
				EventName: cbufStr((*C.char)(unsafe.Pointer(&l.event_name[0])), 128),
				Function:  l.function,
			}
		}
	}
	en := int(c.event_count)
	if en < 0 || en > C.MAX_EVENTS_PER_PLUGIN {
		en = 0
	}
	if en > 0 {
		desc.ProvidedEvents = make([]EventRecord, en)
		for i := 0; i < en; i++ {
			e := &c.provided_events[i]
			desc.ProvidedEvents[i] = EventRecord{
				EventName: cbufStr((*C.char)(unsafe.Pointer(&e.event_name[0])), 128),
			}
		}
	}
	return desc
}

// NewKernelWithDLL 沙箱模式加载dll
func NewKernelWithDLL(path string) (*Kernel, error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	handle := C.create_sandbox(cPath)
	if handle == nil {
		return nil, fmt.Errorf("failed to create sandbox for DLL: %s", path)
	}
	return &Kernel{sandboxHandle: handle, loopStartedCh: make(chan struct{})}, nil
}
