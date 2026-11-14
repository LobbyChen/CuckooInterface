// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/logger"
	"CuckooInterface/core/utils"
	"context"
	"fmt"
	"os"
	"os/exec"
)

// coreCommand 是 Daemon 发给 Core 保活循环的控制命令
type coreCommand int

const (
	coreStart coreCommand = iota
	coreStop
)

// startCoreProcess 启动一份 Core 进程，并返回进程句柄和退出结果通道。
func startCoreProcess(log *logger.Logger) (*exec.Cmd, <-chan error, error) {
	exeDir, err := utils.GetExecutableDir()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get executable dir: %w", err)
	}

	selfPath, err := os.Executable()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get self executable path: %w", err)
	}

	// Core 与 Daemon 使用同一个可执行文件
	cmd := exec.Command(selfPath, "-"+constant.ArgTag, constant.NormalModeArgData)
	cmd.Dir = exeDir

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("failed to start Core process: %w", err)
	}
	log.Infof("Core process started, PID: %d", cmd.Process.Pid)

	// cmd.Wait 只能调用一次
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()
	return cmd, waitCh, nil
}

// waitCoreExit 等待当前 Core 退出
//   - stopped=true：收到 coreStop，Core 已被主动结束；
//   - stopped=false：Core 自行退出，或 Daemon 正在退出。
func waitCoreExit(log *logger.Logger, ctx context.Context, control <-chan coreCommand, cmd *exec.Cmd, waitCh <-chan error) (bool, error) {
	for {
		select {
		case err := <-waitCh:
			if err != nil {
				log.Warnf("Core process exited with error: %v", err)
			} else {
				log.Info("Core process exited normally")
			}
			return false, err

		case c := <-control:
			// Core 运行期间的重复 Start 直接忽略。
			if c != coreStop {
				continue
			}
			if err := cmd.Process.Kill(); err != nil {
				log.Warnf("Failed to kill Core process: %v", err)
			}
			<-waitCh
			return true, nil

		case <-ctx.Done():
			// Daemon 退出，主动结束当前 Core。
			if err := cmd.Process.Kill(); err != nil {
				log.Warnf("Failed to kill Core process: %v", err)
			}
			<-waitCh
			return false, ctx.Err()
		}
	}
}

// KeepCoreAliveLoop 是 Core 进程的唯一管理者
// - coreStart：恢复保活，拉起 Core；
// - coreStop：结束当前 Core，并暂停保活；
// - ctx 取消：结束当前 Core，退出循环。
func KeepCoreAliveLoop(log *logger.Logger, ctx context.Context, control <-chan coreCommand) {
	log.Info("Pending to start Core process")
	running := true
	for {
		if ctx.Err() != nil {
			return
		}

		// 暂停状态：等待 Start 或 Daemon 退出，不做忙等。
		if !running {
			select {
			case <-ctx.Done():
				return
			case c := <-control:
				if c == coreStart {
					running = true
				}
			}
			continue
		}

		cmd, waitCh, err := startCoreProcess(log)
		if err != nil {
			log.Warnf("Failed to start Core: %v", err)
			// 启动失败后等待下一次控制命令
			select {
			case <-ctx.Done():
				return
			case c := <-control:
				if c == coreStop {
					running = false
				}
			}
			continue
		}

		stopped, err := waitCoreExit(log, ctx, control, cmd, waitCh)
		if ctx.Err() != nil {
			return
		}
		if stopped {
			running = false
			continue
		}
		if err != nil {
			log.Warnf("Core exited unexpectedly: %v", err)
		}
	}
}
