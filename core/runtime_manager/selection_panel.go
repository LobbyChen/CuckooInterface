// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package runtime_manager

import (
	"CuckooInterface/core/config"
	"CuckooInterface/core/plugin_manager"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RuntimeSelectionResult 是选择运行时后返回给 UI 的结果。
type RuntimeSelectionResult struct {
	OK               bool   `json:"ok"`
	RequiresRestart  bool   `json:"requiresRestart"`
	SelectedHome     string `json:"selectedHome"`
	EffectiveHome    string `json:"effectiveHome,omitempty"`
	EffectiveVersion string `json:"effectiveVersion,omitempty"`
	EffectiveSource  string `json:"effectiveSource,omitempty"`
}

// SelectionPanel 动态生成运行时选择面板。
// UI 与设置页一样按 SettingPanel 描述渲染，不在前端写死控件。
func (m *Manager) SelectionPanel(kernelID string, refresh bool) (config.SettingPanel, error) {
	entry, err := m.findKernelEntry(kernelID)
	if err != nil {
		return config.SettingPanel{}, err
	}

	runtime := ""
	if entry.Kernel != nil {
		_, _, _, runtime = entry.Kernel.Meta()
	}
	if runtime != pythonRuntime {
		return m.buildBuiltinRuntimePanel(entry, runtime), nil
	}

	selected := m.selectedHome(pythonRuntime)
	installs := m.pythonInstallations(refresh)
	if selected != "" && !containsInstallHome(installs, selected) {
		if install, ok := probePythonHome(selected, "custom"); ok {
			installs = append([]pythonInstall{install}, installs...)
		}
	}

	effective, effectiveSource, hasEffective := pickEffectivePython(selected, installs)

	autoText := "自动探测"
	if hasEffective {
		autoText += fmt.Sprintf("（当前：Python %s · %s）", effective.Version, effective.Architecture)
	}

	options := []config.TextOption{{Value: "", Text: autoText}}
	for _, install := range installs {
		if !install.Compatible {
			continue
		}
		options = append(options, config.TextOption{
			Value: install.Home,
			Text: fmt.Sprintf("Python %s (%s) · %s",
				install.Version, install.Architecture, install.Home),
		})
	}
	if selected != "" && !optionsContainValue(options, selected) {
		options = append(options, config.TextOption{
			Value: selected,
			Text:  "当前选择（不可用）：" + selected,
		})
	}

	selectSetting := config.NewSetting(
		config.SettingDefinition{
			Key:                "runtime." + pythonRuntime + ".home",
			Name:               "Python 安装",
			Description:        "选择 Python Kernel 使用的 64 位 CPython。留空自动探测；修改后需重启 Core 生效。",
			DisplayName:        true,
			DisplayDescription: true,
			Editor:             config.SelectEditor{Options: options},
			Actions: []config.SettingAction{
				{Key: "runtime.refresh", Type: config.ActionRefreshRuntime, Text: "刷新检测"},
				{Key: "runtime.browse", Type: config.ActionBrowseRuntime, Text: "手动指定目录"},
				{Key: "runtime.apply", Type: config.ActionApplyRuntime, Text: "应用并重启 Core"},
			},
		},
		selected,
		"",
	)

	effectiveText := "未检测到可用的 64 位 Python。"
	if hasEffective {
		source := "系统安装"
		if effectiveSource == "config" {
			source = "手动选择"
		}
		effectiveText = fmt.Sprintf("Python %s (%s) · %s · %s",
			effective.Version, effective.Architecture, source, effective.Home)
	}

	installedText := "未在 PATH、py 启动器或注册表中发现 Python。"
	if len(installs) > 0 {
		lines := make([]string, 0, len(installs))
		for _, install := range installs {
			lines = append(lines, formatPythonInstall(install))
		}
		installedText = strings.Join(lines, "\n")
	}

	panel := config.SettingPanel{
		Pages: []config.SettingPage{
			{
				Key:  "runtime-selection",
				Name: "Python 运行时选择",
				Sections: []config.SettingSection{
					{
						Key:      "selection",
						Name:     "选择运行时",
						Settings: []config.Setting{selectSetting},
					},
					{
						Key:  "probe",
						Name: "探测结果",
						Settings: []config.Setting{
							config.NewSetting(
								config.SettingDefinition{
									Key:                "runtime.effective",
									Name:               "当前有效运行时",
									Description:        "重启 Core 后将使用这里显示的 Python。",
									DisplayName:        true,
									DisplayDescription: true,
									Editor:             config.TextEditor{ReadOnly: true},
								},
								effectiveText,
								"",
							),
							config.NewSetting(
								config.SettingDefinition{
									Key:                "runtime.installed",
									Name:               "已检测到的 Python",
									Description:        "内核不会随包分发 Python；目标机器需安装 64 位 CPython 3.8+。",
									DisplayName:        true,
									DisplayDescription: true,
									Editor:             config.TextEditor{ReadOnly: true, MultiLine: true},
								},
								installedText,
								"",
							),
						},
					},
				},
			},
		},
	}
	return panel, nil
}

func (m *Manager) buildBuiltinRuntimePanel(entry plugin_manager.KernelRuntimeEntry, runtime string) config.SettingPanel {
	version := ""
	home := builtinRuntimeHome(entry.Dir)
	if entry.Kernel != nil && entry.Initialized {
		if raw, err := entry.Kernel.GetRuntimeState(); err == nil {
			if state, ok := parseKernelRuntimeState(raw); ok {
				version = state.Version
				if state.Home != "" {
					home = state.Home
				}
			}
		}
	}
	if version == "" {
		version = "内置"
	}

	return config.SettingPanel{
		Pages: []config.SettingPage{
			{
				Key:  "runtime-builtin",
				Name: fmt.Sprintf("%s 运行时", runtime),
				Sections: []config.SettingSection{
					{
						Key:  "builtin",
						Name: "内置运行时",
						Settings: []config.Setting{
							config.NewSetting(
								config.SettingDefinition{
									Key:                "runtime.builtin.source",
									Name:               "运行时来源",
									Description:        "该 Kernel 自带运行时，无需选择外部安装。",
									DisplayName:        true,
									DisplayDescription: true,
									Editor:             config.TextEditor{ReadOnly: true},
								},
								"内置运行时（随 Kernel 分发）",
								"",
							),
							config.NewSetting(
								config.SettingDefinition{
									Key:                "runtime.builtin.version",
									Name:               "运行时版本",
									DisplayName:        true,
									DisplayDescription: false,
									Editor:             config.TextEditor{ReadOnly: true},
								},
								version,
								"",
							),
							config.NewSetting(
								config.SettingDefinition{
									Key:                "runtime.builtin.home",
									Name:               "Kernel 目录",
									DisplayName:        true,
									DisplayDescription: false,
									Editor:             config.TextEditor{ReadOnly: true},
								},
								home,
								"",
							),
						},
					},
				},
			},
		},
	}
}

// SelectRuntime 保存用户为指定 Kernel 选择的运行时。
// home 为空表示恢复自动探测。
func (m *Manager) SelectRuntime(kernelID, home string) (RuntimeSelectionResult, error) {
	entry, err := m.findKernelEntry(kernelID)
	if err != nil {
		return RuntimeSelectionResult{}, err
	}

	runtime := ""
	if entry.Kernel != nil {
		_, _, _, runtime = entry.Kernel.Meta()
	}
	if runtime != pythonRuntime {
		return RuntimeSelectionResult{}, fmt.Errorf("kernel %s uses a built-in runtime and cannot be changed", kernelID)
	}

	currentHome := ""
	if kernel, err := m.findKernel(kernelID); err == nil {
		if raw, err := kernel.GetRuntimeState(); err == nil {
			if state, ok := parseKernelRuntimeState(raw); ok {
				currentHome = state.Home
			}
		}
	}

	home = strings.TrimSpace(home)
	resolvedHome := ""
	if home != "" {
		install, ok := probePythonHome(home, "custom")
		if !ok {
			return RuntimeSelectionResult{}, fmt.Errorf("no runnable python.exe found in %s", home)
		}
		if !install.Compatible {
			switch {
			case install.Architecture != "x64":
				return RuntimeSelectionResult{}, fmt.Errorf("only 64-bit CPython is supported (found %s)", install.Architecture)
			case !versionAtLeast38(install.Version):
				return RuntimeSelectionResult{}, fmt.Errorf("CPython 3.8 or newer is required (found %s)", install.Version)
			default:
				return RuntimeSelectionResult{}, fmt.Errorf("selected Python installation is incomplete: %s", install.Home)
			}
		}
		resolvedHome = install.Home
	}

	if err := m.setSelectedHome(pythonRuntime, resolvedHome); err != nil {
		return RuntimeSelectionResult{}, err
	}

	m.invalidate()

	// auto 模式会把实际选中的 Python 写入环境变量；用它与正在运行的
	// Kernel 对比，才能正确判断是否需要重启。
	nextHome := strings.TrimSpace(os.Getenv(envPythonHome))
	result := RuntimeSelectionResult{
		OK:              true,
		RequiresRestart: !strings.EqualFold(currentHome, nextHome),
		SelectedHome:    resolvedHome,
	}
	if install, source, ok := m.effectivePython(); ok {
		result.EffectiveHome = install.Home
		result.EffectiveVersion = install.Version
		result.EffectiveSource = source
	}
	return result, nil
}

func pickEffectivePython(selected string, installs []pythonInstall) (pythonInstall, string, bool) {
	if selected != "" {
		for _, install := range installs {
			if strings.EqualFold(install.Home, selected) && install.Compatible {
				return install, "config", true
			}
		}
	}
	for _, install := range installs {
		if install.Compatible {
			return install, "system", true
		}
	}
	return pythonInstall{}, "", false
}

func containsInstallHome(installs []pythonInstall, home string) bool {
	for _, install := range installs {
		if strings.EqualFold(install.Home, home) {
			return true
		}
	}
	return false
}

func optionsContainValue(options []config.TextOption, value string) bool {
	for _, option := range options {
		if text, ok := option.Value.(string); ok && strings.EqualFold(text, value) {
			return true
		}
	}
	return false
}

// builtinRuntimeHome 仅用于展示内置 Kernel 的目录。
func builtinRuntimeHome(kernelDir string) string {
	if kernelDir == "" {
		return ""
	}
	return filepath.Join(kernelDir, "binary")
}
