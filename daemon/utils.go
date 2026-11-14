// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package daemon

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/logger"
	"CuckooInterface/core/utils"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// UI 可执行文件名，与守护进程位于同一目录
const uiExecutableName = "CuckooInterfaceUI.exe"

// StartUI 拉起同目录下的 CuckooInterfaceUI.exe
func StartUI(nPipeName string) error {

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
	cmd := exec.Command(uiPath, constant.DaemonNamePipeArg, nPipeName)
	cmd.Dir = exeDir

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start UI process: %v", err)
	}

	logger.GetLogger().Infof("UI process started, PID: %d, path: %s", cmd.Process.Pid, uiPath)

	// UI 关闭只结束本次 UI 进程，不终止 Daemon 与 Core。
	if waitErr := cmd.Wait(); waitErr != nil {
		return waitErr
	}
	return nil
}

func GenerateHash() string {
	random := make([]byte, 16)
	_, _ = rand.Read(random)

	hash := sha256.Sum256(random)
	return hex.EncodeToString(hash[:])
}
