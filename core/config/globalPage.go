// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package config

import (
	"sync"
)

// 外观设置为前端固定页面，其值由后端单独存储
var (
	appearanceMu     sync.RWMutex
	appearanceValues = map[string]interface{}{
		"theme":       "system",
		"accentColor": "#0078D4",
		"fontScale":   100.0,
	}

	// appearanceKeys 用于 SaveSettings 时区分存储目标
	appearanceKeys = map[string]struct{}{
		"theme":       {},
		"accentColor": {},
		"fontScale":   {},
	}

	settingsMu    sync.RWMutex
	settingValues = map[string]interface{}{}
)

// GetAppearanceSettings 返回外观设置的当前值（返回副本，避免外部修改内部状态）。
func GetAppearanceSettings() map[string]interface{} {
	appearanceMu.RLock()
	defer appearanceMu.RUnlock()
	out := make(map[string]interface{}, len(appearanceValues))
	for k, v := range appearanceValues {
		out[k] = v
	}
	return out
}

// syncSettingsToCoreConfig 把本次更新的设置值同步进全局 CoreConfig 结构体。
// LoadCoreConfig 之前（coreCfg 为 nil）不做任何事。
func syncSettingsToCoreConfig(values map[string]interface{}) {
	coreCfgMu.Lock()
	defer coreCfgMu.Unlock()
	if coreCfg == nil {
		return
	}
	for k, v := range values {
		switch k {
		case "autoStart":
			coreCfg.AutoStart = toBool(v, coreCfg.AutoStart)
		case "singleInstance":
			coreCfg.SingleInstance = toBool(v, coreCfg.SingleInstance)
		case "serviceMode":
			coreCfg.ServiceMode = toBool(v, coreCfg.ServiceMode)
		case "serviceUserSid":
			coreCfg.ServiceUserSID = toString(v, coreCfg.ServiceUserSID)
		case "logLevel":
			coreCfg.LogLevel = toString(v, coreCfg.LogLevel)
		case "logMaxSize":
			coreCfg.LogMaxSize = toFloat(v, coreCfg.LogMaxSize)
		case "logCompress":
			coreCfg.LogCompress = toBool(v, coreCfg.LogCompress)
		case "autoLoadPlugins":
			coreCfg.AutoLoadPlugins = toBool(v, coreCfg.AutoLoadPlugins)
		case "continueOnError":
			coreCfg.ContinueOnError = toBool(v, coreCfg.ContinueOnError)
		case "pluginDir":
			coreCfg.PluginDir = toString(v, coreCfg.PluginDir)
		case "theme":
			coreCfg.Appearance.Theme = toString(v, coreCfg.Appearance.Theme)
		case "accentColor":
			coreCfg.Appearance.AccentColor = toString(v, coreCfg.Appearance.AccentColor)
		case "fontScale":
			coreCfg.Appearance.FontScale = toFloat(v, coreCfg.Appearance.FontScale)
		}
	}
}

func toBool(v interface{}, fallback bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return fallback
}

func toString(v interface{}, fallback string) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fallback
}

func toFloat(v interface{}, fallback float64) float64 {
	switch f := v.(type) {
	case float64:
		return f
	case float32:
		return float64(f)
	case int:
		return float64(f)
	case int64:
		return float64(f)
	}
	return fallback
}

// setSettingValue 在 SettingPanel 中递归查找并更新指定 key 的 Setting 值。
func setSettingValue(panel SettingPanel, key string, value interface{}) {
	for i := range panel.Pages {
		for j := range panel.Pages[i].Sections {
			for k := range panel.Pages[i].Sections[j].Settings {
				s := &panel.Pages[i].Sections[j].Settings[k]
				if (*s).Key() == key {
					_ = (*s).SetValue(value)
					return
				}
			}
		}
	}
}

func BuildSettingsPanel() SettingPanel {
	panel := SettingPanel{
		Pages: []SettingPage{
			// 1. 通用 (General)
			{
				Key:  "general",
				Name: "通用",
				Sections: []SettingSection{
					{
						Key:  "startup",
						Name: "启动",
						Settings: []Setting{
							NewSetting(
								SettingDefinition{
									Key:                "autoStart",
									Name:               "开机自启动",
									Description:        "登录系统后静默启动守护进程和托盘，不自动显示界面",
									DisplayName:        true,
									DisplayDescription: true,
									Editor:             SwitchEditor{},
								},
								false, // default value
								false,
							),
						},
					},
					{
						Key:  "instance",
						Name: "单实例",
						Settings: []Setting{
							NewSetting(
								SettingDefinition{
									Key:                "singleInstance",
									Name:               "仅允许运行一个实例",
									Description:        "新启动的实例将通知旧实例退出（重启 Core 后生效）",
									DisplayName:        true,
									DisplayDescription: true,
									Editor:             SwitchEditor{},
								},
								true,
								true,
							),
						},
					},
					{
						Key:  "service",
						Name: "系统服务",
						Settings: []Setting{
							NewSetting(
								SettingDefinition{
									Key:                "serviceMode",
									Name:               "将 Core 注册为系统服务并开机自启",
									Description:        "需要管理员权限；注册后将在下次开机自动启动 Core（系统账户后台运行，不显示界面），当前 Core 不受影响",
									DisplayName:        true,
									DisplayDescription: true,
									Editor:             SwitchEditor{},
								},
								false,
								false,
							),
						},
					},
				},
			},

			// 2. 日志 (Logging)
			{
				Key:  "logging",
				Name: "日志",
				Sections: []SettingSection{
					{
						Key:  "level",
						Name: "日志级别",
						Settings: []Setting{
							NewSetting(
								SettingDefinition{
									Key:                "logLevel",
									Name:               "日志级别",
									Description:        "控制日志输出的详细程度（立即生效）",
									DisplayName:        true,
									DisplayDescription: true,
									Editor: SelectEditor{
										Options: []TextOption{
											{Value: "debug", Text: "Debug"},
											{Value: "info", Text: "Info"},
											{Value: "warn", Text: "Warn"},
											{Value: "error", Text: "Error"},
										},
									},
								},
								"info",
								"info",
							),
						},
					},
					{
						Key:  "maxSize",
						Name: "文件大小限制",
						Settings: []Setting{
							NewSetting(
								SettingDefinition{
									Key:                "logMaxSize",
									Name:               "单文件最大大小",
									Description:        "当日志文件超过此大小时进行轮转（立即生效）",
									DisplayName:        true,
									DisplayDescription: false,
									Editor: SliderEditor{
										Min:  10,
										Max:  1024,
										Step: 10,
										Unit: "MB",
									},
								},
								100.0,
								100.0,
							),
						},
					},
					{
						Key:  "compression",
						Name: "压缩策略",
						Settings: []Setting{
							NewSetting(
								SettingDefinition{
									Key:                "logCompress",
									Name:               "压缩旧日志",
									Description:        "对超过保留天数的日志文件进行压缩以节省空间（立即生效）",
									DisplayName:        true,
									DisplayDescription: true,
									Editor:             SwitchEditor{},
								},
								true,
								true,
							),
						},
					},
				},
			},

			// 3. 插件 (Plugins)
			{
				Key:  "plugins",
				Name: "插件",
				Sections: []SettingSection{
					{
						Key:  "loading",
						Name: "加载行为",
						Settings: []Setting{
							NewSetting(
								SettingDefinition{
									Key:                "autoLoadPlugins",
									Name:               "启动时自动加载插件",
									Description:        "启动时自动加载 Base/Active 插件（重启 Core 后生效）",
									DisplayName:        true,
									DisplayDescription: true,
									Editor:             SwitchEditor{},
								},
								true,
								true,
							),
							NewSetting(
								SettingDefinition{
									Key:                "continueOnError",
									Name:               "加载失败时继续",
									Description:        "某个插件加载失败时不中断整体启动流程（重启 Core 后生效）",
									DisplayName:        true,
									DisplayDescription: true,
									Editor:             SwitchEditor{},
								},
								true,
								true,
							),
						},
					},
					{
						Key:  "directory",
						Name: "插件目录",
						Settings: []Setting{
							NewSetting(
								SettingDefinition{
									Key:                "pluginDir",
									Name:               "插件目录路径",
									Description:        "留空使用 user 目录；修改后重启 Core 生效",
									DisplayName:        true,
									DisplayDescription: true,
									Editor:             TextEditor{ReadOnly: true, Placeholder: "留空使用默认 user 目录"},
									Actions: []SettingAction{
										{
											Key:  "browsePluginDir",
											Type: ActionBrowseDirectory,
											Text: "浏览",
										},
									},
								},
								"",
								"",
							),
						},
					},
				},
			},
		},
	}

	settingsMu.RLock()
	for k, v := range settingValues {
		setSettingValue(panel, k, v)
	}
	settingsMu.RUnlock()
	return panel
}
