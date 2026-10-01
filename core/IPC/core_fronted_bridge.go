package IPC

import (
	"CuckooInterface/core/config"
	"CuckooInterface/core/constant"
	"CuckooInterface/core/event"
	"CuckooInterface/core/logger"
	"CuckooInterface/core/plugin_manager"
	"CuckooInterface/core/utils"
	"encoding/json"
	"fmt"
	"net"
	"runtime"
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

	logger    *logger.Logger
	listener  *pipe.PipeListener
	startTime time.Time
}

// Init 注入核心组件依赖（仅生效一次）
func (ipc *CoreIpcInterface) Init(km *plugin_manager.KernelManager, pfm *plugin_manager.PluginFileManager, pm *plugin_manager.PluginManager, ebus *event.EventBus, logger *logger.Logger) {
	ipc.once.Do(func() {
		ipc.km = km
		ipc.pfm = pfm
		ipc.pm = pm
		ipc.logger = logger
		ipc.ebus = ebus
		ipc.startTime = time.Now()
	})
}

func (ipc *CoreIpcInterface) check() bool {
	if ipc.km == nil || ipc.pfm == nil || ipc.pm == nil {
		return false
	}
	return true
}

func (ipc *CoreIpcInterface) CreatePipe() error {
	l, err := pipe.Listen(constant.CoreNamePipe)
	if err != nil {
		return err
	}
	ipc.listener = l
	return nil
}

func (ipc *CoreIpcInterface) DestroyPipe() error {
	if ipc.listener == nil {
		return nil
	}
	return ipc.listener.Close()
}

func (ipc *CoreIpcInterface) waitForSentData(data []byte) error {
	conn, err := ipc.listener.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()
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
	defer conn.Close()
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

// ServeLoop 持续接受前端连接，读取请求并返回响应。
// 每个连接处理一次请求-响应后关闭，由前端重新连接发起下一次调用。
func (ipc *CoreIpcInterface) ServeLoop() {
	for {
		conn, err := ipc.listener.Accept()
		if err != nil {
			ipc.logger.Errorf("IPC accept failed: %v", err)
			return
		}
		ipc.handleConnection(conn)
	}
}

// handleConnection 在单个连接上完成一次请求-响应
func (ipc *CoreIpcInterface) handleConnection(conn net.Conn) {
	defer conn.Close()

	// 读取请求
	_ = conn.SetReadDeadline(time.Now().Add(time.Second * 5))
	reqData, err := ReadSinglePacket(conn)
	if err != nil {
		ipc.logger.Warnf("IPC read failed: %v", err)
		return
	}

	// 分发处理
	respData := ipc.HandleRequest(reqData)

	// 写入响应
	_ = conn.SetWriteDeadline(time.Now().Add(time.Second * 5))
	sentData := GeneralPkg(respData)
	if _, err := conn.Write(sentData); err != nil {
		ipc.logger.Warnf("IPC write failed: %v", err)
	}
}

// 请求分发

// HandleRequest 解析请求并分发到对应处理函数，返回序列化后的响应
func (ipc *CoreIpcInterface) HandleRequest(data []byte) []byte {
	var req IpcRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return ipc.errorResp(0, "invalid request: "+err.Error())
	}

	var (
		result []byte
		err    error
	)

	switch req.Method {
	case MethodGetOverview:
		result, err = ipc.handleGetOverview()
	case MethodGetPlugins:
		result, err = ipc.handleGetPlugins()
	case MethodTogglePlugin:
		result, err = ipc.handleTogglePlugin(req.Params)
	case MethodRemovePlugin:
		result, err = ipc.handleRemovePlugin(req.Params)
	case MethodLoadPlugin:
		result, err = ipc.handleLoadPlugin(req.Params)
	case MethodUnloadPlugin:
		result, err = ipc.handleUnloadPlugin(req.Params)
	case MethodInstallPlugin:
		result, err = ipc.handleInstallPlugin(req.Params)
	case MethodGetRecentEvents:
		result, err = ipc.handleGetRecentEvents()
	case MethodGetRegisteredEvents:
		result, err = ipc.handleGetRegisteredEvents()
	case MethodGetEventOptions:
		result, err = ipc.handleGetEventOptions()
	case MethodPublishEvent:
		result, err = ipc.handlePublishEvent(req.Params)
	case MethodGetLogs:
		result, err = ipc.handleGetLogs()
	case MethodClearLogs:
		result, err = ipc.handleClearLogs()
	case MethodGetSettingsPanel:
		result, err = ipc.handleGetSettingsPanel()
	case MethodGetAppearanceSettings:
		result, err = ipc.handleGetAppearanceSettings()
	case MethodSaveSettings:
		result, err = ipc.handleSaveSettings(req.Params)
	default:
		return ipc.errorResp(req.ID, "unknown method: "+req.Method)
	}

	if err != nil {
		return ipc.errorResp(req.ID, err.Error())
	}
	return ipc.successResp(req.ID, result)
}

// 响应辅助

func (ipc *CoreIpcInterface) successResp(id int64, data json.RawMessage) []byte {
	resp := IpcResponse{ID: id, Success: true, Data: data}
	b, _ := json.Marshal(resp)
	return b
}

func (ipc *CoreIpcInterface) errorResp(id int64, errMsg string) []byte {
	resp := IpcResponse{ID: id, Success: false, Error: errMsg}
	b, _ := json.Marshal(resp)
	return b
}

// 插件类型转换

func plugTypeToString(t constant.PlugType) string {
	switch t {
	case constant.KERNEL:
		return "kernel"
	case constant.BASE:
		return "base"
	case constant.ACTIVE:
		return "active"
	default:
		return "unknown"
	}
}

func stringToPlugType(s string) (constant.PlugType, error) {
	switch s {
	case "kernel":
		return constant.KERNEL, nil
	case "base":
		return constant.BASE, nil
	case "active":
		return constant.ACTIVE, nil
	default:
		return 0, fmt.Errorf("unknown plugin type: %s", s)
	}
}

// 概览

func (ipc *CoreIpcInterface) handleGetOverview() (json.RawMessage, error) {
	allPlugins := ipc.pfm.GetAllPlugins()
	var total, active, kernels, activeKernels int
	for _, f := range allPlugins {
		total++
		if f.Typo == constant.KERNEL {
			kernels++
		}
	}

	// 已加载的插件
	loaded := ipc.pm.ListLoadedPlugins()
	loadedSet := make(map[string]bool, len(loaded))
	for _, p := range loaded {
		loadedSet[p.PluginID] = true
		if p.Type == constant.KERNEL {
			activeKernels++
		}
	}
	active = len(loaded)

	// 事件统计
	events := ipc.ebus.GetAllBufferedEvent()
	errorCount := 0 // 事件总线当前无级别概念，暂为 0

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memMB := float64(m.Alloc) / 1024 / 1024

	uptime := formatUptime(time.Since(ipc.startTime))

	dto := SystemOverviewDto{
		TotalPlugins:    total,
		ActivePlugins:   active,
		TotalKernels:    kernels,
		ActiveKernels:   activeKernels,
		EventCountToday: len(events),
		ErrorCountToday: errorCount,
		CpuUsage:        0, // 暂不采集 CPU
		MemoryUsageMB:   memMB,
		Uptime:          uptime,
	}
	return json.Marshal(dto)
}

func formatUptime(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm %ds", m, int(d.Seconds())%60)
}

// 插件

func (ipc *CoreIpcInterface) handleGetPlugins() (json.RawMessage, error) {
	allFiles := ipc.pfm.GetAllPlugins()

	// 已加载插件集合
	loaded := ipc.pm.ListLoadedPlugins()
	loadedSet := make(map[string]plugin_manager.PluginSummary, len(loaded))
	for _, p := range loaded {
		loadedSet[p.PluginID] = p
	}

	// 内核启用状态
	enabledKernelIDs := make(map[string]bool)
	for _, k := range ipc.km.GetInitKernelPlugins() {
		id, _, _, _ := k.Meta()
		enabledKernelIDs[id] = true
	}

	dtos := make([]PluginDto, 0, len(allFiles))
	for _, f := range allFiles {
		dto := PluginDto{
			ID:          f.Meta.ID,
			Name:        f.Meta.Name,
			Version:     f.Meta.Version,
			Description: f.Meta.Description,
			Type:        plugTypeToString(f.Typo),
			RuntimeType: f.Meta.RuntimeType,
			IsEnabled:   enabledKernelIDs[f.Meta.ID],
			IsLoaded:    false,
		}
		if sum, ok := loadedSet[f.Meta.ID]; ok {
			dto.IsLoaded = true
			dto.IsEnabled = true
			dto.ProvidedEvents = sum.Events
			dto.Listeners = sum.Listeners
		}
		dtos = append(dtos, dto)
	}
	return json.Marshal(dtos)
}

func (ipc *CoreIpcInterface) handleTogglePlugin(params json.RawMessage) (json.RawMessage, error) {
	var p TogglePluginParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	file := ipc.findPluginFile(p.ID)
	if file == nil {
		return nil, fmt.Errorf("plugin %s not found", p.ID)
	}

	if p.Enabled {
		if err := ipc.pm.LoadPlugin(file); err != nil {
			return nil, fmt.Errorf("load plugin failed: %w", err)
		}
	} else {
		if err := ipc.pm.UnloadPlugin(file); err != nil {
			return nil, fmt.Errorf("unload plugin failed: %w", err)
		}
	}
	return json.Marshal(true)
}

func (ipc *CoreIpcInterface) handleRemovePlugin(params json.RawMessage) (json.RawMessage, error) {
	var p RemovePluginParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	file := ipc.findPluginFile(p.ID)
	if file == nil {
		return nil, fmt.Errorf("plugin %s not found", p.ID)
	}

	// 先卸载（若已加载）
	_ = ipc.pm.UnloadPlugin(file)
	// 从文件管理器移除
	if err := ipc.pfm.RemovePlugin(p.ID); err != nil {
		return nil, fmt.Errorf("remove plugin file failed: %w", err)
	}
	ipc.logger.Infof("插件 [%s] 已卸载", file.Meta.Name)
	return json.Marshal(true)
}

func (ipc *CoreIpcInterface) handleLoadPlugin(params json.RawMessage) (json.RawMessage, error) {
	var p LoadPluginParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if _, err := stringToPlugType(p.Type); err != nil {
		return nil, err
	}
	file := ipc.findPluginFile(p.ID)
	if file == nil {
		return nil, fmt.Errorf("plugin %s not found", p.ID)
	}
	if err := ipc.pm.LoadPlugin(file); err != nil {
		return nil, err
	}
	return json.Marshal(true)
}

func (ipc *CoreIpcInterface) handleUnloadPlugin(params json.RawMessage) (json.RawMessage, error) {
	var p LoadPluginParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	file := ipc.findPluginFile(p.ID)
	if file == nil {
		return nil, fmt.Errorf("plugin %s not found", p.ID)
	}
	if err := ipc.pm.UnloadPlugin(file); err != nil {
		return nil, err
	}
	return json.Marshal(true)
}

func (ipc *CoreIpcInterface) handleInstallPlugin(params json.RawMessage) (json.RawMessage, error) {
	var p InstallPluginParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	plugType, err := stringToPlugType(p.Type)
	if err != nil {
		return nil, err
	}
	if !utils.IsFileExist(p.Path) {
		return nil, fmt.Errorf("file %s not exist", p.Path)
	}
	if err := ipc.pfm.ExtractAndLoadExternalPlugin(p.Path, plugType); err != nil {
		return nil, fmt.Errorf("install plugin failed: %w", err)
	}
	return json.Marshal(true)
}

func (ipc *CoreIpcInterface) findPluginFile(id string) *plugin_manager.SinglePluginFile {
	files := ipc.pfm.GetAllPlugins()
	for i := range files {
		if files[i].Meta.ID == id {
			return files[i]
		}
	}
	return nil
}

// 事件

func (ipc *CoreIpcInterface) handleGetRecentEvents() (json.RawMessage, error) {
	events := ipc.ebus.GetAllBufferedEvent()
	dtos := make([]EventRecordDto, 0, len(events))
	for _, e := range events {
		dtos = append(dtos, EventRecordDto{
			Timestamp: e.GetTimeStamp(),
			EventName: e.GetName(),
			Payload:   e.GetPayLoad(), // 修复：之前错误地使用了 GetName()
			Level:     "normal",
		})
	}
	return json.Marshal(dtos)
}

func (ipc *CoreIpcInterface) handleGetRegisteredEvents() (json.RawMessage, error) {
	infos := ipc.ebus.GetRegisteredEventsInfo()
	dtos := make([]RegisteredEventDto, 0, len(infos))
	for _, info := range infos {
		dtos = append(dtos, RegisteredEventDto{
			EventName:     info.Name,
			Provider:      info.ProviderID,
			ListenerCount: info.ListenerCount,
		})
	}
	return json.Marshal(dtos)
}

func (ipc *CoreIpcInterface) handleGetEventOptions() (json.RawMessage, error) {
	names := ipc.ebus.GetEventNames()
	dtos := make([]EventOptionDto, 0, len(names))
	for _, name := range names {
		dtos = append(dtos, EventOptionDto{
			Name:           name,
			DefaultPayload: "{}",
		})
	}
	return json.Marshal(dtos)
}

func (ipc *CoreIpcInterface) handlePublishEvent(params json.RawMessage) (json.RawMessage, error) {
	var p PublishEventParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("event name is required")
	}
	payload := p.Payload
	if payload == "" {
		payload = "{}"
	}
	ipc.ebus.Emit(p.Name, payload)

	dto := EventRecordDto{
		Timestamp: time.Now().Unix(),
		EventName: p.Name,
		Payload:   payload,
		Level:     "normal",
	}
	return json.Marshal(dto)
}

// 日志

func (ipc *CoreIpcInterface) handleGetLogs() (json.RawMessage, error) {
	logs := ipc.logger.GetCachedLogs()
	dtos := make([]LogEntryDto, 0, len(logs))
	for _, l := range logs {
		dtos = append(dtos, LogEntryDto{
			Timestamp: l.Timestamp.Unix(),
			Level:     l.Level,
			Message:   l.Message,
			Logger:    l.Caller,
		})
	}
	return json.Marshal(dtos)
}

func (ipc *CoreIpcInterface) handleClearLogs() (json.RawMessage, error) {
	ipc.logger.ClearCachedLogs()
	return json.Marshal(true)
}

// 设置

func (ipc *CoreIpcInterface) handleGetSettingsPanel() (json.RawMessage, error) {
	return json.Marshal(config.BuildSettingsPanel())
}

func (ipc *CoreIpcInterface) handleGetAppearanceSettings() (json.RawMessage, error) {
	return json.Marshal(config.GetAppearanceSettings())
}

func (ipc *CoreIpcInterface) handleSaveSettings(params json.RawMessage) (json.RawMessage, error) {
	var values map[string]interface{}
	if err := json.Unmarshal(params, &values); err != nil {
		return nil, fmt.Errorf("invalid settings payload: %w", err)
	}
	config.SaveSettings(values)
	return json.Marshal(true)
}
