package dameon

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/logger"
	"CuckooInterface/core/utils"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// UI 可执行文件名，与守护进程位于同一目录
const uiExecutableName = "CuckooInterfaceUI.exe"

// startNormMode 以 normal 模式拉起 Core 主程序
func startNormMode() error {
	// 检查是否以 dameon 启动
	if !isDaemon() {
		return fmt.Errorf("Program is not started in dameon")
	}

	// 获取当前可执行文件目录
	exeDir, err := utils.GetExecutableDir()
	if err != nil {
		return fmt.Errorf("failed to get executable dir: %v", err)
	}

	// 以 normal 模式重新启动自身（Core 进程）
	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get self executable path: %v", err)
	}

	cmd := exec.Command(selfPath, "-"+constant.ArgTag, constant.NormalModeArgData)
	cmd.Dir = exeDir
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start normal mode: %v", err)
	}

	logger.GetLogger().Infof("Normal mode process started, PID: %d", cmd.Process.Pid)
	return nil
}

// StartUI 拉起同目录下的 CuckooInterfaceUI.exe
func StartUI() error {
	sm := GetStatusManager()

	// 防重复启动：如果 UI 已在运行则直接返回
	if sm.IsUIRunning() {
		logger.GetLogger().Info("UI is already running, skip")
		return nil
	}

	// 定位 UI 可执行文件
	exeDir, err := utils.GetExecutableDir()
	if err != nil {
		return fmt.Errorf("failed to get executable directory: %v", err)
	}
	uiPath := filepath.Join(exeDir, uiExecutableName)

	// 检查文件是否存在
	if _, err := os.Stat(uiPath); os.IsNotExist(err) {
		return fmt.Errorf("UI executable not found at: %s", uiPath)
	}

	// 启动 UI 进程
	cmd := exec.Command(uiPath)
	cmd.Dir = exeDir

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start UI process: %v", err)
	}

	logger.GetLogger().Infof("UI process started, PID: %d, path: %s", cmd.Process.Pid, uiPath)
	sm.SetUIProcess(cmd.Process)

	// 后台协程监控 UI 进程生命周期，退出后自动清除状态
	go func() {
		waitErr := cmd.Wait()
		if waitErr != nil {
			logger.GetLogger().Warnf("UI process exited with error: %v", waitErr)
		} else {
			logger.GetLogger().Info("UI process exited normally")
		}
		sm.ClearUIProcess()
	}()

	return nil
}

// StopUI 强制终止 UI 进程
func StopUI() error {
	sm := GetStatusManager()
	if !sm.IsUIRunning() {
		return nil
	}

	p := sm.GetUIProcess()
	if p != nil {
		if err := p.Kill(); err != nil {
			return fmt.Errorf("failed to kill UI process: %v", err)
		}
		logger.GetLogger().Info("UI process killed")
	}

	sm.ClearUIProcess()
	return nil
}
