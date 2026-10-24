// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package runtime_manager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigFile 是 RuntimeManager 的持久化配置文件名，位于 configs/ 目录。
const ConfigFile = "runtimes.json"

const (
	envPythonHome       = "CUCKOO_PYTHON_HOME"
	envPythonHomeSource = "CUCKOO_PYTHON_HOME_SOURCE"
)

const pythonRuntime = "python"

// RuntimeConfig 描述某个运行时类型在目标机器上使用的具体安装。
type RuntimeConfig struct {
	// Home 为空表示自动探测；非空时使用该 Python 安装目录。
	Home string `json:"home"`
}

// Config 是 RuntimeManager 的持久化配置。
type Config struct {
	Runtimes map[string]RuntimeConfig `json:"runtimes"`
}

func defaultConfig() Config {
	return Config{Runtimes: make(map[string]RuntimeConfig)}
}

func (m *Manager) loadConfig() error {
	cfg := defaultConfig()
	var resultErr error

	m.cfgMu.Lock()
	data, err := os.ReadFile(m.cfgPath)
	switch {
	case err == nil:
		data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
		if err := json.Unmarshal(data, &cfg); err != nil {
			cfg = defaultConfig()
			resultErr = fmt.Errorf("parse %s: %w", m.cfgPath, err)
		} else if cfg.Runtimes == nil {
			cfg.Runtimes = make(map[string]RuntimeConfig)
		}
	case os.IsNotExist(err):
		resultErr = m.persistConfigLocked(cfg)
	default:
		cfg = defaultConfig()
		resultErr = fmt.Errorf("read %s: %w", m.cfgPath, err)
	}
	m.cfg = cfg
	m.cfgMu.Unlock()

	// 环境变量必须在 Kernel 初始化前确定。auto 模式会把实际选中的
	// Python 显式写进环境变量，保证 Kernel 与运行时页面看到同一份安装。
	m.applyEnv()
	return resultErr
}

func (m *Manager) persistConfigLocked(cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(m.cfgPath), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	// 0600：运行时选择属于用户偏好，不应允许其他用户读写。
	if err := os.WriteFile(m.cfgPath, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", m.cfgPath, err)
	}
	return nil
}

func (m *Manager) selectedHome(runtime string) string {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return m.cfg.Runtimes[runtime].Home
}

// setSelectedHome 更新指定运行时的选择并立即写盘。
func (m *Manager) setSelectedHome(runtime, home string) error {
	home = strings.TrimSpace(home)

	m.cfgMu.Lock()
	previous := m.cfg.Runtimes[runtime]
	if home == "" {
		delete(m.cfg.Runtimes, runtime)
	} else {
		m.cfg.Runtimes[runtime] = RuntimeConfig{Home: home}
	}
	if err := m.persistConfigLocked(m.cfg); err != nil {
		if previous.Home == "" {
			delete(m.cfg.Runtimes, runtime)
		} else {
			m.cfg.Runtimes[runtime] = previous
		}
		m.cfgMu.Unlock()
		return err
	}
	m.cfgMu.Unlock()

	m.applyEnv()
	return nil
}

// applyEnv 把运行时选择同步到进程环境变量，供 PythonKernel 初始化时读取。
// auto 模式会解析出第一份兼容的 Python，并标记来源为 system。
func (m *Manager) applyEnv() {
	m.cfgMu.RLock()
	configured := m.cfg.Runtimes[pythonRuntime].Home
	m.cfgMu.RUnlock()

	if configured != "" {
		if install, ok := probePythonHome(configured, "custom"); ok {
			_ = os.Setenv(envPythonHome, install.Home)
			_ = os.Setenv(envPythonHomeSource, "config")
			return
		}
		// 配置的安装已不可用，回退到自动探测。
	}

	_ = os.Unsetenv(envPythonHome)
	_ = os.Unsetenv(envPythonHomeSource)
	for _, install := range m.pythonInstallations(false) {
		if install.Compatible {
			_ = os.Setenv(envPythonHome, install.Home)
			_ = os.Setenv(envPythonHomeSource, "system")
			return
		}
	}
}
