// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	// Name 是 Windows 服务名，也是 SCM 中的唯一标识。
	Name        = "CuckooInterfaceCore"
	DisplayName = "CuckooInterface Core"
	Description = "CuckooInterface 插件平台后台服务"
)

// Install 注册（或更新）Core 服务并设为自动启动，要求当前进程有管理员权限。
func Install(exePath string) error {
	return installDirect(exePath)
}

// Uninstall 停止并删除 Core 服务，要求当前进程有管理员权限。
func Uninstall() error {
	return uninstallDirect()
}

// InstallElevated 注册（或更新）Core 服务并设为自动启动。
// 当前进程权限不足时会弹出 UAC，由提权后的辅助进程完成安装。
func InstallElevated(exePath string) error {
	if err := installDirect(exePath); err == nil {
		return nil
	} else if !isAccessDenied(err) {
		return err
	}
	return runElevated("service-install")
}

// UninstallElevated 停止并删除 Core 服务。
func UninstallElevated() error {
	if err := uninstallDirect(); err == nil {
		return nil
	} else if !isAccessDenied(err) {
		return err
	}
	return runElevated("service-uninstall")
}

// IsInstalled 返回服务是否已在 SCM 中注册。
func IsInstalled() (bool, error) {
	m, err := mgr.Connect()
	if err != nil {
		return false, fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(Name)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return false, nil
		}
		return false, fmt.Errorf("open service: %w", err)
	}
	s.Close()
	return true, nil
}

// IsRunning 返回服务当前是否处于运行（或启动中）状态。
func IsRunning() bool {
	m, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer m.Disconnect()

	s, err := m.OpenService(Name)
	if err != nil {
		return false
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return false
	}
	return status.State == svc.Running || status.State == svc.StartPending
}

func installDirect(exePath string) error {
	if exePath == "" {
		var err error
		exePath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("resolve executable: %w", err)
		}
	}
	if exePath == "" {
		return fmt.Errorf("executable path is empty")
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	binPath := fmt.Sprintf(`"%s" -launch_type service`, exePath)

	if s, err := m.OpenService(Name); err == nil {
		defer s.Close()
		cfg, cfgErr := s.Config()
		if cfgErr != nil {
			return fmt.Errorf("read service config: %w", cfgErr)
		}
		cfg.BinaryPathName = binPath
		cfg.DisplayName = DisplayName
		cfg.Description = Description
		cfg.StartType = mgr.StartAutomatic
		if err := s.UpdateConfig(cfg); err != nil {
			return fmt.Errorf("update service config: %w", err)
		}
		return nil
	}

	cfg := mgr.Config{
		DisplayName: DisplayName,
		Description: Description,
		StartType:   mgr.StartAutomatic,
	}
	s, err := m.CreateService(Name, exePath, cfg, "-launch_type", "service")
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer s.Close()

	return nil
}

func uninstallDirect() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(Name)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil
		}
		return fmt.Errorf("open service: %w", err)
	}
	defer s.Close()

	status, err := s.Query()
	self := err == nil && status.ProcessId == uint32(os.Getpid())
	if err == nil && status.State != svc.Stopped && !self {
		_, _ = s.Control(svc.Stop)
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(300 * time.Millisecond)
			status, err = s.Query()
			if err == nil && status.State == svc.Stopped {
				break
			}
		}
	}
	// 服务进程自行卸载时不能等待自身停止；Delete 会将其标记为删除，
	// 当前进程退出后服务即被移除。

	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	return nil
}

func isAccessDenied(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED)
}

// runElevated 通过 UAC 启动同一可执行文件的辅助模式。
func runElevated(action string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	// PowerShell 单引号字符串转义。
	escapedExe := strings.ReplaceAll(exe, "'", "''")
	script := fmt.Sprintf(
		"$p = Start-Process -FilePath '%s' -ArgumentList @('-launch_type','%s') -Verb RunAs -Wait -PassThru -ErrorAction Stop; exit $p.ExitCode",
		escapedExe, action,
	)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("elevated %s failed: %s", action, message)
	}
	return nil
}
