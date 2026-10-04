package config

import (
	"CuckooInterface/core/utils"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// CoreConfigFile Core 全局配置持久化文件名（存放于 configs/ 目录）
const CoreConfigFile = "core.json"

// AppearanceConfig 外观设置（与前端约定的 key：theme / accentColor / fontScale）
type AppearanceConfig struct {
	Theme       string  `json:"theme"`       // light / dark / system
	AccentColor string  `json:"accentColor"` // 强调色 #RRGGBB
	FontScale   float64 `json:"fontScale"`   // 字体缩放百分比
}

// CoreConfig Core 全局配置结构体。
//
// 启动时由 LoadCoreConfig 从 configs/core.json 读取；
// SaveSettings 更新设置后会同步进该结构体并自动写回磁盘，实现长期化。
func defaultCoreConfig() *CoreConfig {
	return &CoreConfig{
		AutoStart:       false,
		SingleInstance:  true,
		LogLevel:        "info",
		LogMaxSize:      100.0,
		LogCompress:     true,
		AutoLoadPlugins: true,
		ContinueOnError: true,
		PluginDir:       "",
		Appearance: AppearanceConfig{
			Theme:       "system",
			AccentColor: "#0078D4",
			FontScale:   100.0,
		},
	}
}

// CoreConfig Core 全局配置结构体，字段与 SettingPanel 中的设置 key 一一对应。
type CoreConfig struct {
	// 通用
	AutoStart      bool `json:"autoStart"`      // 开机自启动
	SingleInstance bool `json:"singleInstance"` // 仅允许运行一个实例

	// 日志
	LogLevel    string  `json:"logLevel"`    // debug / info / warn / error
	LogMaxSize  float64 `json:"logMaxSize"`  // 单个日志文件最大大小（MB）
	LogCompress bool    `json:"logCompress"` // 压缩旧日志

	// 插件
	AutoLoadPlugins bool   `json:"autoLoadPlugins"` // 启动时自动加载插件
	ContinueOnError bool   `json:"continueOnError"` // 插件加载失败时继续
	PluginDir       string `json:"pluginDir"`       // 插件目录路径

	// 外观
	Appearance AppearanceConfig `json:"appearance"`
}

var (
	coreCfgMu sync.RWMutex
	coreCfg   *CoreConfig
	coreCfgFP string
)

// LoadCoreConfig 在 Core 启动时调用：从 dir/core.json 读取全局配置为结构体。
//   - 文件不存在：以默认配置落盘，返回 nil；
//   - 文件损坏：内存回退默认配置，返回错误（进程仍可继续运行）；
//   - 读取成功：把配置值同步到设置面板与外观设置的内存值。
func LoadCoreConfig(dir string) error {
	fp := filepath.Join(dir, CoreConfigFile)

	coreCfgMu.Lock()
	coreCfgFP = fp
	coreCfgMu.Unlock()

	cfg := defaultCoreConfig()
	if utils.IsFileExist(fp) {
		data, err := os.ReadFile(fp)
		if err != nil {
			applyCoreConfig(cfg)
			return fmt.Errorf("failed to read core config: %w", err)
		}
		if err := json.Unmarshal(data, cfg); err != nil {
			applyCoreConfig(defaultCoreConfig())
			return fmt.Errorf("failed to parse core config %s: %w", fp, err)
		}
	} else {
		// 首次启动：将默认配置持久化到磁盘
		if err := writeCoreConfigFile(fp, cfg); err != nil {
			applyCoreConfig(cfg)
			return fmt.Errorf("failed to persist default core config: %w", err)
		}
	}
	applyCoreConfig(cfg)
	return nil
}

// applyCoreConfig 记录全局配置，并同步到设置面板与外观设置的内存值
func applyCoreConfig(cfg *CoreConfig) {
	coreCfgMu.Lock()
	coreCfg = cfg
	coreCfgMu.Unlock()

	settingsMu.Lock()
	settingValues["autoStart"] = cfg.AutoStart
	settingValues["singleInstance"] = cfg.SingleInstance
	settingValues["logLevel"] = cfg.LogLevel
	settingValues["logMaxSize"] = cfg.LogMaxSize
	settingValues["logCompress"] = cfg.LogCompress
	settingValues["autoLoadPlugins"] = cfg.AutoLoadPlugins
	settingValues["continueOnError"] = cfg.ContinueOnError
	settingValues["pluginDir"] = cfg.PluginDir
	settingsMu.Unlock()

	appearanceMu.Lock()
	appearanceValues["theme"] = cfg.Appearance.Theme
	appearanceValues["accentColor"] = cfg.Appearance.AccentColor
	appearanceValues["fontScale"] = cfg.Appearance.FontScale
	appearanceMu.Unlock()
}

// GetCoreConfig 返回当前 Core 全局配置的副本
// 在 LoadCoreConfig 之前调用时返回默认配置。
func GetCoreConfig() *CoreConfig {
	coreCfgMu.RLock()
	defer coreCfgMu.RUnlock()
	if coreCfg == nil {
		return defaultCoreConfig()
	}
	cp := *coreCfg
	return &cp
}

// SaveCoreConfig 把当前全局配置写回磁盘（configs/core.json）。
func SaveCoreConfig() error {
	coreCfgMu.RLock()
	fp := coreCfgFP
	var cfg *CoreConfig = coreCfg
	coreCfgMu.RUnlock()
	if fp == "" || cfg == nil {
		return fmt.Errorf("core config has not been loaded yet")
	}
	cp := *cfg
	return writeCoreConfigFile(fp, &cp)
}

func writeCoreConfigFile(fp string, cfg *CoreConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fp, data, 0666)
}
