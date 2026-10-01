package IPC

import "encoding/json"

// 请求/响应协议定义
//
// 采用类 JSON-RPC 协议，通过命名管道传输。
// 请求包载荷: {"id": <int>, "method": "<string>", "params": <object>}
// 响应包载荷: {"id": <int>, "success": <bool>, "data": <any>, "error": "<string>"}

// IpcRequest 表示前端发来的请求
type IpcRequest struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// IpcResponse 表示返回前端的响应
type IpcResponse struct {
	ID      int64           `json:"id"`
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// 方法名常量
const (
	// 概览
	MethodGetOverview = "getOverview"

	// 插件
	MethodGetPlugins    = "getPlugins"
	MethodTogglePlugin  = "togglePlugin"
	MethodRemovePlugin  = "removePlugin"
	MethodLoadPlugin    = "loadPlugin"
	MethodUnloadPlugin  = "unloadPlugin"
	MethodInstallPlugin = "installPlugin"

	// 事件
	MethodGetRecentEvents     = "getRecentEvents"
	MethodGetRegisteredEvents = "getRegisteredEvents"
	MethodGetEventOptions     = "getEventOptions"
	MethodPublishEvent        = "publishEvent"

	// 日志
	MethodGetLogs   = "getLogs"
	MethodClearLogs = "clearLogs"

	// 设置
	MethodGetSettingsPanel      = "getSettingsPanel"
	MethodGetAppearanceSettings = "getAppearanceSettings"
	MethodSaveSettings          = "saveSettings"
)

// Daemon IPC 方法名常量
// Daemon IPC 负责 Core 进程的生命周期控制（启动/停止/重启）
const (
	MethodDaemonPing        = "ping"
	MethodDaemonStartCore   = "startCore"
	MethodDaemonStopCore    = "stopCore"
	MethodDaemonRestartCore = "restartCore"
)

// 请求参数类型

type TogglePluginParams struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

type RemovePluginParams struct {
	ID string `json:"id"`
}

type LoadPluginParams struct {
	Type string `json:"type"` // "kernel" | "base" | "active"
	ID   string `json:"id"`
}

type InstallPluginParams struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

type PublishEventParams struct {
	Name    string `json:"name"`
	Payload string `json:"payload"`
}

// 响应数据类型

// PluginDto 前端插件展示模型
type PluginDto struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Version        string   `json:"version"`
	Description    string   `json:"description"`
	Type           string   `json:"type"` // "kernel" | "base" | "active"
	IsEnabled      bool     `json:"isEnabled"`
	IsLoaded       bool     `json:"isLoaded"`
	Author         string   `json:"author"`
	RuntimeType    string   `json:"runtimeType"`
	ProvidedEvents []string `json:"providedEvents"`
	Listeners      []string `json:"listeners"`
}

// EventRecordDto 前端事件记录模型
type EventRecordDto struct {
	Timestamp int64  `json:"timestamp"` // Unix 秒
	EventName string `json:"eventName"`
	Payload   string `json:"payload"`
	Level     string `json:"level"` // "normal" | "warning" | "error"
}

// RegisteredEventDto 已注册事件模型
type RegisteredEventDto struct {
	EventName     string `json:"eventName"`
	Provider      string `json:"provider"`
	ListenerCount int    `json:"listenerCount"`
}

// EventOptionDto 可发布事件选项
type EventOptionDto struct {
	Name           string `json:"name"`
	DefaultPayload string `json:"defaultPayload"`
}

// LogEntryDto 日志条目模型
type LogEntryDto struct {
	Timestamp int64  `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Logger    string `json:"logger"`
}

// SystemOverviewDto 系统概览模型
type SystemOverviewDto struct {
	TotalPlugins    int     `json:"totalPlugins"`
	ActivePlugins   int     `json:"activePlugins"`
	TotalKernels    int     `json:"totalKernels"`
	ActiveKernels   int     `json:"activeKernels"`
	EventCountToday int     `json:"eventCountToday"`
	ErrorCountToday int     `json:"errorCountToday"`
	CpuUsage        float64 `json:"cpuUsage"`
	MemoryUsageMB   float64 `json:"memoryUsageMB"`
	Uptime          string  `json:"uptime"`
}
