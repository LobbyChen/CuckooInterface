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
func (spc *SinglePluginConfig) GetConfigString() string {
	return spc.configString
}

type CfgManager struct {
	pluginsCfg map[string]*SinglePluginConfig // PluginName:Cfg ptr
	CfgPath    string
	lock       sync.Mutex
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
func (cm *CfgManager) RegisterConfig(PluginName string) error {
	cm.lock.Lock()
	defer cm.lock.Unlock()
	// 检查是否已经注册
	if _, exists := cm.pluginsCfg[PluginName]; exists {
		return fmt.Errorf("plugin config already registered: %s", PluginName)
	}
	// 创建空配置
	spc := &SinglePluginConfig{
		configString: "",
	}
	// 创建文件
	fp := filepath.Join(cm.CfgPath, PluginName+".cfg")
	// 检测是否已经有该文件
	if utils.IsFileExist(fp) {
		// 加载
		if cfg, err := loadBinaryConfig(fp); err != nil {
			maps.Copy(cm.pluginsCfg, cfg)
		}
		// 返回
		return nil
	}
	f, err := os.Create(fp)
	if err != nil {
		return fmt.Errorf("failed to create plugin config file: %v", err)
	}
	err = f.Close()
	if err != nil {
		return fmt.Errorf("failed to close plugin config file: %v", err)
	}
	cm.pluginsCfg[PluginName] = spc
	return nil
}
func (cm *CfgManager) GetPluginConfig(PluginName string, obj *SinglePluginConfig) error {
	if _, ok := cm.pluginsCfg[PluginName]; !ok {
		return fmt.Errorf("plugin config doesn't exist: %s", PluginName)
	}
	cm.lock.Lock()
	defer cm.lock.Unlock()
	obj.configString = cm.pluginsCfg[PluginName].configString
	return nil
}
func (cm *CfgManager) WriteConfig(pluginName string, config *SinglePluginConfig) error {
	if _, ok := cm.pluginsCfg[pluginName]; !ok {
		return fmt.Errorf("plugin config doesn't exist: %s", pluginName)
	}
	cm.lock.Lock()
	obj := &SinglePluginConfig{
		configString: config.configString,
	}
	// 修改cfg
	cm.pluginsCfg[pluginName] = obj
	cm.lock.Unlock()
	// 写回磁盘
	return obj.SaveConfig(filepath.Join(cm.CfgPath, pluginName+".cfg"))
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
	// pluginName 由文件名推导，无需在文件中重复存储
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
