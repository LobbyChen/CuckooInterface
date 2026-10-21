// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package runtime_manager

import (
	"CuckooInterface/core/logger"
	"CuckooInterface/core/plugin_manager"
	"CuckooInterface/plugins"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// 运行时状态会触发 Kernel 控制面调用，短时间缓存避免 UI 高频轮询。
	runtimeListCacheTTL = 2 * time.Second
)

// RuntimeInfo 是下发给 UI 的单个 Kernel 运行时信息。
type RuntimeInfo struct {
	KernelID       string   `json:"kernelId"`
	Name           string   `json:"name"`
	KernelVersion  string   `json:"kernelVersion"`
	Runtime        string   `json:"runtime"`
	Status         string   `json:"status"` // running / starting / stopped / error
	Initialized    bool     `json:"initialized"`
	LoopStarted    bool     `json:"loopStarted"`
	RuntimeVersion string   `json:"runtimeVersion,omitempty"`
	Home           string   `json:"home,omitempty"`
	Source         string   `json:"source,omitempty"` // system / config / embedded
	Embedded       bool     `json:"embedded"`
	PluginCount    int      `json:"pluginCount"`
	Capabilities   []string `json:"capabilities,omitempty"`
	LastError      string   `json:"lastError,omitempty"`
}

// Manager 统一管理 Kernel 运行时的状态、控制面与运行环境选择。
type Manager struct {
	km  *plugin_manager.KernelManager
	pm  *plugin_manager.PluginManager
	log *logger.Logger

	cfgMu   sync.RWMutex
	cfg     Config
	cfgPath string

	installsMu sync.Mutex
	installs   []pythonInstall
	installsAt time.Time

	listMu sync.Mutex
	list   []RuntimeInfo
	listAt time.Time
}

// New 创建 RuntimeManager 并加载运行时配置。
func New(km *plugin_manager.KernelManager, pm *plugin_manager.PluginManager, log *logger.Logger, configDir string) *Manager {
	m := &Manager{
		km:      km,
		pm:      pm,
		log:     log,
		cfgPath: filepath.Join(configDir, ConfigFile),
	}
	if err := m.loadConfig(); err != nil && log != nil {
		log.Warnf("RuntimeManager: %v", err)
	}
	return m
}

func (m *Manager) invalidate() {
	m.installsMu.Lock()
	m.installs = nil
	m.installsAt = time.Time{}
	m.installsMu.Unlock()

	m.listMu.Lock()
	m.listAt = time.Time{}
	m.list = nil
	m.listMu.Unlock()
}

// List 返回全部 Kernel 运行时信息。
func (m *Manager) List() []RuntimeInfo {
	m.listMu.Lock()
	if time.Since(m.listAt) < runtimeListCacheTTL {
		ret := append([]RuntimeInfo(nil), m.list...)
		m.listMu.Unlock()
		return ret
	}
	m.listMu.Unlock()

	infos := m.buildList()

	m.listMu.Lock()
	m.list = append([]RuntimeInfo(nil), infos...)
	m.listAt = time.Now()
	m.listMu.Unlock()
	return infos
}

func (m *Manager) buildList() []RuntimeInfo {
	entries := m.km.ListKernelEntries()
	if len(entries) == 0 {
		return []RuntimeInfo{}
	}

	counts := m.pluginCounts()
	infos := make([]RuntimeInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.Kernel == nil {
			continue
		}
		kid, name, version, runtime := entry.Kernel.Meta()
		info := RuntimeInfo{
			KernelID:      kid,
			Name:          name,
			KernelVersion: version,
			Runtime:       runtime,
			Initialized:   entry.Initialized,
			LoopStarted:   entry.LoopStarted,
			LastError:     entry.InitError,
			PluginCount:   counts[runtime],
		}

		if entry.InitError != "" {
			info.Status = "error"
		} else if !entry.Initialized {
			info.Status = "stopped"
		} else if !entry.Kernel.IsAlive() {
			info.Status = "error"
			if info.LastError == "" {
				info.LastError = "kernel process is not alive"
			}
		} else if !entry.LoopStarted {
			info.Status = "starting"
			if entry.LoopError != "" {
				info.Status = "error"
				info.LastError = entry.LoopError
			}
		} else {
			info.Status = "running"
		}

		if entry.Initialized {
			raw, err := entry.Kernel.GetRuntimeState()
			if err != nil {
				if info.LastError == "" {
					info.LastError = err.Error()
				}
			} else if state, ok := parseKernelRuntimeState(raw); ok {
				info.RuntimeVersion = state.Version
				info.Home = state.Home
				info.Source = state.Source
				info.Embedded = state.Embedded
				info.Capabilities = []string{"state", "settings", "operations"}
				if state.LastError != "" && info.LastError == "" {
					info.LastError = state.LastError
				}
				if state.State == "error" {
					info.Status = "error"
				}
			}
		}

		// Kernel 未初始化或没有控制面时，用 RuntimeManager 的探测结果补全展示信息。
		if runtime == pythonRuntime && info.Home == "" {
			if install, source, ok := m.effectivePython(); ok {
				info.RuntimeVersion = install.Version
				info.Home = install.Home
				info.Source = source
				info.Embedded = false
			}
		}
		if runtime == "lua" && info.Home == "" {
			info.Home = filepath.Join(entry.Dir, "binary")
			info.Source = "embedded"
			info.Embedded = true
		}
		infos = append(infos, info)
	}
	return infos
}

type kernelRuntimeState struct {
	State       string `json:"state"`
	Runtime     string `json:"runtime"`
	Version     string `json:"version"`
	Home        string `json:"home"`
	Source      string `json:"source"`
	Embedded    bool   `json:"embedded"`
	PluginCount int    `json:"pluginCount"`
	LastError   string `json:"lastError"`
}

func parseKernelRuntimeState(raw string) (kernelRuntimeState, bool) {
	var state kernelRuntimeState
	if strings.TrimSpace(raw) == "" {
		return state, false
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return state, false
	}
	if state.State == "" {
		state.State = "ready"
	}
	return state, true
}

func (m *Manager) pluginCounts() map[string]int {
	counts := make(map[string]int)
	if m.pm == nil {
		return counts
	}
	for _, p := range m.pm.ListLoadedPlugins() {
		counts[p.Runtime]++
	}
	return counts
}

// GetRuntimeState 返回指定 Kernel 的原始运行时状态 JSON。
func (m *Manager) GetRuntimeState(kernelID string) (string, error) {
	k, err := m.findKernel(kernelID)
	if err != nil {
		return "", err
	}
	raw, err := k.GetRuntimeState()
	if err != nil {
		return "", err
	}
	if !json.Valid([]byte(raw)) {
		return "", fmt.Errorf("kernel %s returned invalid runtime state JSON", kernelID)
	}
	return raw, nil
}

// GetSettingsPanel 返回指定 Kernel 的动态设置面板 JSON。
func (m *Manager) GetSettingsPanel(kernelID string) (string, error) {
	k, err := m.findKernel(kernelID)
	if err != nil {
		return "", err
	}
	raw, err := k.GetSettingsPanel()
	if err != nil {
		return "", err
	}
	if !json.Valid([]byte(raw)) {
		return "", fmt.Errorf("kernel %s returned invalid settings panel JSON", kernelID)
	}
	return raw, nil
}

// SetSetting 修改指定 Kernel 的设置项，valueJSON 必须是合法 JSON 值。
func (m *Manager) SetSetting(kernelID, key, valueJSON string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("setting key is required")
	}
	if !json.Valid([]byte(valueJSON)) {
		return "", fmt.Errorf("setting value is not valid JSON")
	}
	k, err := m.findKernel(kernelID)
	if err != nil {
		return "", err
	}
	raw, err := k.SetSetting(key, valueJSON)
	if err != nil {
		return "", err
	}
	if !json.Valid([]byte(raw)) {
		return "", fmt.Errorf("kernel %s returned invalid setting response JSON", kernelID)
	}
	m.invalidate()
	return raw, nil
}

// GetOperations 返回指定 Kernel 声明的运行时操作。
func (m *Manager) GetOperations(kernelID string) (string, error) {
	k, err := m.findKernel(kernelID)
	if err != nil {
		return "", err
	}
	raw, err := k.GetOperations()
	if err != nil {
		return "", err
	}
	if !json.Valid([]byte(raw)) {
		return "", fmt.Errorf("kernel %s returned invalid operations JSON", kernelID)
	}
	return raw, nil
}

// InvokeOperation 执行指定 Kernel 的运行时操作。
func (m *Manager) InvokeOperation(kernelID, operationID, requestJSON string) (string, error) {
	if strings.TrimSpace(operationID) == "" {
		return "", fmt.Errorf("operation id is required")
	}
	if strings.TrimSpace(requestJSON) == "" {
		requestJSON = "{}"
	}
	if !json.Valid([]byte(requestJSON)) {
		return "", fmt.Errorf("operation request is not valid JSON")
	}
	k, err := m.findKernel(kernelID)
	if err != nil {
		return "", err
	}
	raw, err := k.InvokeOperation(operationID, requestJSON)
	if err != nil {
		return "", err
	}
	if !json.Valid([]byte(raw)) {
		return "", fmt.Errorf("kernel %s returned invalid operation response JSON", kernelID)
	}
	m.invalidate()
	return raw, nil
}

func (m *Manager) findKernel(kernelID string) (*plugins.Kernel, error) {
	if strings.TrimSpace(kernelID) == "" {
		return nil, fmt.Errorf("kernel id is required")
	}
	k, ok := m.km.FindKernel(kernelID)
	if !ok {
		return nil, fmt.Errorf("kernel %s not found", kernelID)
	}
	return k, nil
}

func (m *Manager) findKernelEntry(kernelID string) (plugin_manager.KernelRuntimeEntry, error) {
	for _, entry := range m.km.ListKernelEntries() {
		if entry.ID == kernelID {
			return entry, nil
		}
	}
	return plugin_manager.KernelRuntimeEntry{}, fmt.Errorf("kernel %s not found", kernelID)
}
