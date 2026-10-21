// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package config

import (
	"CuckooInterface/core/logger"
	"CuckooInterface/core/service"
	"CuckooInterface/core/utils"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
)

// SettingsApplyResult 描述一次设置保存中各 key 的实际结果。
type SettingsApplyResult struct {
	Saved           bool              `json:"saved"`
	Applied         []string          `json:"applied"`
	RequiresRestart []string          `json:"requiresRestart"`
	Errors          map[string]string `json:"errors,omitempty"`
}

// SaveSettings 校验、应用并持久化设置。
// 可热更新的设置立即生效；启动相关设置写入配置并标记为 RequiresRestart。
func SaveSettings(values map[string]interface{}) SettingsApplyResult {
	result := SettingsApplyResult{Errors: make(map[string]string)}
	old := GetCoreConfig()
	valid := make(map[string]interface{})
	applied := make([]string, 0)
	restart := make([]string, 0)

	markApplied := func(key string) { applied = append(applied, key) }
	markRestart := func(key string) { restart = append(restart, key) }
	fail := func(key string, err error) { result.Errors[key] = err.Error() }

	if v, ok := values["autoStart"]; ok {
		enabled := toBool(v, old.AutoStart)
		if err := utils.SetAutoStartForSID(old.ServiceUserSID, enabled); err != nil {
			fail("autoStart", err)
		} else {
			valid["autoStart"] = enabled
			markApplied("autoStart")
		}
	}

	if v, ok := values["singleInstance"]; ok {
		enabled := toBool(v, old.SingleInstance)
		valid["singleInstance"] = enabled
		if enabled != old.SingleInstance {
			markRestart("singleInstance")
		} else {
			markApplied("singleInstance")
		}
	}

	if v, ok := values["serviceMode"]; ok {
		enabled := toBool(v, old.ServiceMode)
		if enabled != old.ServiceMode {
			if enabled {
				sid, sidErr := currentUserSID()
				if sidErr != nil {
					fail("serviceMode", fmt.Errorf("获取当前用户 SID 失败: %w", sidErr))
				} else if err := service.InstallElevated(""); err != nil {
					fail("serviceMode", err)
				} else {
					valid["serviceMode"] = true
					valid["serviceUserSid"] = sid
					markApplied("serviceMode")
				}
			} else if err := service.UninstallElevated(); err != nil {
				fail("serviceMode", err)
			} else {
				valid["serviceMode"] = false
				valid["serviceUserSid"] = ""
				markApplied("serviceMode")
			}
		} else {
			markApplied("serviceMode")
		}
	}

	if v, ok := values["logLevel"]; ok {
		level := strings.ToLower(strings.TrimSpace(toString(v, old.LogLevel)))
		if err := logger.GetLogger().SetLevel(level); err != nil {
			fail("logLevel", err)
		} else {
			valid["logLevel"] = level
			markApplied("logLevel")
		}
	}

	_, maxPresent := values["logMaxSize"]
	_, compressPresent := values["logCompress"]
	rotationMax := old.LogMaxSize
	if v, ok := values["logMaxSize"]; ok {
		logMaxSize := toFloat(v, old.LogMaxSize)
		if logMaxSize < 10 || logMaxSize > 1024 {
			fail("logMaxSize", fmt.Errorf("日志大小必须在 10-1024 MB 之间"))
		} else {
			rotationMax = logMaxSize
			valid["logMaxSize"] = logMaxSize
		}
	}
	logCompress := old.LogCompress
	if v, ok := values["logCompress"]; ok {
		logCompress = toBool(v, old.LogCompress)
		valid["logCompress"] = logCompress
	}
	if (maxPresent && valid["logMaxSize"] != nil) || compressPresent {
		if err := logger.GetLogger().SetFileRotation(int(math.Round(rotationMax)), logCompress); err != nil {
			if maxPresent {
				fail("logMaxSize", err)
			}
			if compressPresent {
				fail("logCompress", err)
			}
		} else {
			if maxPresent {
				markApplied("logMaxSize")
			}
			if compressPresent {
				markApplied("logCompress")
			}
		}
	} else if maxPresent {
		// 最大值无效时不重复应用，错误已在上方记录。
		delete(valid, "logCompress")
	}

	if v, ok := values["autoLoadPlugins"]; ok {
		enabled := toBool(v, old.AutoLoadPlugins)
		valid["autoLoadPlugins"] = enabled
		if enabled != old.AutoLoadPlugins {
			markRestart("autoLoadPlugins")
		} else {
			markApplied("autoLoadPlugins")
		}
	}

	if v, ok := values["continueOnError"]; ok {
		enabled := toBool(v, old.ContinueOnError)
		valid["continueOnError"] = enabled
		if enabled != old.ContinueOnError {
			markRestart("continueOnError")
		} else {
			markApplied("continueOnError")
		}
	}

	if v, ok := values["pluginDir"]; ok {
		dir, err := resolvePluginDir(toString(v, old.PluginDir))
		if err != nil {
			fail("pluginDir", err)
		} else {
			valid["pluginDir"] = dir
			if dir != old.PluginDir {
				markRestart("pluginDir")
			} else {
				markApplied("pluginDir")
			}
		}
	}

	if v, ok := values["theme"]; ok {
		theme := strings.ToLower(strings.TrimSpace(toString(v, old.Appearance.Theme)))
		switch theme {
		case "light", "dark", "system":
			valid["theme"] = theme
			markApplied("theme")
		default:
			fail("theme", fmt.Errorf("未知主题: %s", theme))
		}
	}

	if v, ok := values["accentColor"]; ok {
		color := strings.TrimSpace(toString(v, old.Appearance.AccentColor))
		if !isHexColor(color) {
			fail("accentColor", fmt.Errorf("无效的强调色: %s", color))
		} else {
			valid["accentColor"] = color
			markApplied("accentColor")
		}
	}

	if v, ok := values["fontScale"]; ok {
		scale := toFloat(v, old.Appearance.FontScale)
		if scale < 50 || scale > 200 {
			fail("fontScale", fmt.Errorf("字号缩放必须在 50-200%% 之间"))
		} else {
			valid["fontScale"] = scale
			markApplied("fontScale")
		}
	}

	// 写入内存与磁盘。应用失败或校验失败的 key 不会落盘。
	if len(valid) > 0 {
		for key, value := range valid {
			if _, ok := appearanceKeys[key]; ok {
				appearanceMu.Lock()
				appearanceValues[key] = value
				appearanceMu.Unlock()
				continue
			}
			settingsMu.Lock()
			settingValues[key] = value
			settingsMu.Unlock()
		}
		syncSettingsToCoreConfig(valid)
		if err := SaveCoreConfig(); err != nil {
			result.Errors["config"] = fmt.Sprintf("保存配置失败: %v", err)
		} else {
			result.Saved = true
		}
	}

	sort.Strings(applied)
	sort.Strings(restart)
	result.Applied = dedupe(applied)
	result.RequiresRestart = dedupe(restart)
	if len(result.Errors) == 0 {
		result.Errors = nil
	}
	return result
}

func currentUserSID() (string, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return "", err
	}
	defer token.Close()

	user, err := token.GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

func resolvePluginDir(value string) (string, error) {
	dir := strings.TrimSpace(value)
	if dir == "" {
		return "", nil
	}
	if !filepath.IsAbs(dir) {
		base, err := utils.GetExecutableDir()
		if err != nil {
			return "", fmt.Errorf("解析插件目录失败: %w", err)
		}
		dir = filepath.Join(base, dir)
	}
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("插件目录不存在: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("插件路径不是目录: %s", dir)
	}
	if err := EnsurePluginRoot(dir); err != nil {
		return "", fmt.Errorf("创建插件子目录失败: %w", err)
	}
	return dir, nil
}

func isHexColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(value[1:], 16, 32)
	return err == nil
}

func dedupe(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}
