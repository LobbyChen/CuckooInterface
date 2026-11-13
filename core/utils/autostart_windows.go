// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package utils

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const (
	autoStartKeyName   = `Software\Microsoft\Windows\CurrentVersion\Run`
	autoStartValueName = "CuckooInterface"
)

// SetAutoStart 写入/删除当前用户的开机自启动项。
func SetAutoStart(enabled bool) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, autoStartKeyName, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("open autostart registry key: %w", err)
	}
	defer key.Close()

	if !enabled {
		// 不存在时忽略错误。
		_ = key.DeleteValue(autoStartValueName)
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	if exe, err = filepath.Abs(exe); err != nil {
		return fmt.Errorf("resolve absolute executable path: %w", err)
	}

	command := fmt.Sprintf(`"%s" -launch_type dameon -no_ui`, exe)
	if err := key.SetStringValue(autoStartValueName, command); err != nil {
		return fmt.Errorf("write autostart registry value: %w", err)
	}
	return nil
}
