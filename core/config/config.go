// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package config

import (
	"CuckooInterface/core/utils"
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type SinglePluginConfig struct {
	configString string // PluginConfigString
}

func (spc *SinglePluginConfig) SaveConfig(fp string) error {
	configBytes := []byte(spc.configString)
	configLength := len(configBytes)
	// 构造文件字节流
	offset := 0
	data := make([]byte, configLength+8)
	binary.BigEndian.PutUint64(data[offset:offset+8], uint64(configLength))
	offset += 8
	copy(data[offset:offset+configLength], configBytes)
	offset += configLength
	// 写入文件
	return os.WriteFile(fp, data, 0666)
}

type CfgManager struct {
	pluginsCfg map[string]*SinglePluginConfig // PluginName:Cfg ptr
	CfgPath    string
	lock       sync.Mutex
}

// NewCfgManager 创建配置管理器并初始化内存配置表。
func NewCfgManager() *CfgManager {
	return &CfgManager{
		pluginsCfg: make(map[string]*SinglePluginConfig),
	}
}

func (cm *CfgManager) SetCfgPath(path string) {
	cm.lock.Lock()
	defer cm.lock.Unlock()
	cm.CfgPath = path
}

// LoadValidConfig 从指定目录加载所有 .cfg 插件配置文件并合并到内存。
func (cm *CfgManager) LoadValidConfig(dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("path %s is not a directory, only config folder can be loaded", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}
	cm.lock.Lock()
	defer cm.lock.Unlock()
	if cm.pluginsCfg == nil {
		cm.pluginsCfg = make(map[string]*SinglePluginConfig)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// 只处理 .cfg 文件
		if !strings.HasSuffix(entry.Name(), ".cfg") {
			continue
		}
		filePath := filepath.Join(dir, entry.Name())
		loadedCm, err := loadBinaryConfig(filePath)
		if err != nil {
			fmt.Printf("Warning: failed to load config from %s: %v\n", filePath, err)
			continue
		}
		// 合并配置
		maps.Copy(cm.pluginsCfg, loadedCm)
	}
	return nil
}

// EnsureConfig 确保插件配置在内存与磁盘中存在。
func (cm *CfgManager) EnsureConfig(pluginID, defaultConfig string) error {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return errors.New("plugin id is empty")
	}
	cm.lock.Lock()
	defer cm.lock.Unlock()
	if _, exists := cm.pluginsCfg[pluginID]; exists {
		return nil
	}
	fp := filepath.Join(cm.CfgPath, pluginID+".cfg")
	if utils.IsFileExist(fp) {
		if cfg, err := loadBinaryConfig(fp); err == nil {
			if loaded, ok := cfg[pluginID]; ok {
				cm.pluginsCfg[pluginID] = loaded
				return nil
			}
		}
	}
	spc := &SinglePluginConfig{configString: defaultConfig}
	if err := spc.SaveConfig(fp); err != nil {
		return fmt.Errorf("failed to create plugin config file: %w", err)
	}
	cm.pluginsCfg[pluginID] = spc
	return nil
}

// GetConfig 返回插件当前配置字符串。
func (cm *CfgManager) GetConfig(pluginID string) (string, error) {
	cm.lock.Lock()
	defer cm.lock.Unlock()
	cfg, ok := cm.pluginsCfg[pluginID]
	if !ok {
		return "", fmt.Errorf("plugin config doesn't exist: %s", pluginID)
	}
	return cfg.configString, nil
}

// SetConfig 更新插件配置并立即写盘。写盘失败时内存保持旧值。
func (cm *CfgManager) SetConfig(pluginID, configString string) error {
	cm.lock.Lock()
	defer cm.lock.Unlock()
	if _, ok := cm.pluginsCfg[pluginID]; !ok {
		return fmt.Errorf("plugin config doesn't exist: %s", pluginID)
	}
	obj := &SinglePluginConfig{configString: configString}
	if err := obj.SaveConfig(filepath.Join(cm.CfgPath, pluginID+".cfg")); err != nil {
		return err
	}
	cm.pluginsCfg[pluginID] = obj
	return nil
}
func loadBinaryConfig(fp string) (map[string]*SinglePluginConfig, error) {
	if !utils.IsFileExist(fp) {
		return nil, errors.New("config file not exist")
	}
	data, err := os.ReadFile(fp)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	// SaveConfig 写入格式: [8字节 configLength] + [configString]
	if len(data) < 8 {
		return nil, errors.New("invalid config file: too short")
	}
	offset := 0
	// 读取 configString 长度
	configLength := binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8
	// 读取 configString
	if offset+int(configLength) > len(data) {
		return nil, errors.New("invalid config file: cannot read config string")
	}
	configString := string(data[offset : offset+int(configLength)])
	offset += int(configLength)
	// 验证是否有多余数据
	if offset != len(data) {
		return nil, fmt.Errorf("invalid config file: extra data at end, expected %d bytes but got %d", offset, len(data))
	}
	// 创建配置对象
	spc := &SinglePluginConfig{
		configString: configString,
	}
	ret := map[string]*SinglePluginConfig{
		strings.TrimSuffix(filepath.Base(fp), filepath.Ext(filepath.Base(fp))): spc,
	}
	return ret, nil
}
