// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package main

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/logger"
	"CuckooInterface/core/utils"
	"CuckooInterface/daemon"
	"context"
	"path/filepath"
	"sync/atomic"

	"github.com/getlantern/systray"
)

func MainDaemonLoop() {
	// 初始化 Daemon 日志
	log := logger.Init(logger.LoggerConfig{
		Level:         "info",
		LogDir:        utils.GetFolderPath(filepath.Join(constant.LogFolder, "daemon")),
		MaxSize:       100,
		MaxBackups:    7,
		MaxAge:        30,
		Compress:      false,
		ConsoleOutput: true,
		CacheSize:     1000,
	})
	log.Info("CuckooInterface Daemon starting...")

	// coreCtx 只控制 Daemon 退出
	coreCtx, cancelCore := context.WithCancel(context.Background())

	// Core 生命周期由保活循环独占
	control := make(chan coreCommand)
	coreDone := make(chan struct{})
	go func() { // KeepAliveLoop在单次Daemon生命周期中只允许调用一次
		KeepCoreAliveLoop(log, coreCtx, control)
		close(coreDone)
	}()

	ipc := daemon.NewDaemonIpc()
	ipc.Init(log,
		func() error {
			select {
			case control <- coreStart:
				return nil
			case <-coreCtx.Done():
				return coreCtx.Err()
			}
		},
		func() error {
			select {
			case control <- coreStop:
				return nil
			case <-coreCtx.Done():
				return coreCtx.Err()
			}
		},
	)

	pipeName, err := ipc.StartPipe()
	if err != nil {
		log.Fatalf("Failed to create Daemon IPC pipe: %v", err)
	}
	defer ipc.DestroyPipe()
	go ipc.ServeLoop()

	// UI 关闭不影响 Daemon 和 Core
	var uiRunning atomic.Bool
	startUI := func() { // StartUI函数定义
		if !uiRunning.CompareAndSwap(false, true) {
			return
		}
		go func() {
			defer uiRunning.Store(false)
			if err := daemon.StartUI(pipeName); err != nil {
				log.Warnf("UI process exited: %v", err)
			}
		}()
	}

	if isUi {
		startUI()
	}

	systray.Run(func() {
		SetupAndLaunchTray(coreCtx, cancelCore, startUI)
	}, cancelCore)

	cancelCore()
	<-coreDone
}
