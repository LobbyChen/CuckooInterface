package main

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/logger"
	"CuckooInterface/core/utils"
	"CuckooInterface/dameon"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/getlantern/systray"
)

// Core 子进程状态追踪
var (
	coreCmd     *exec.Cmd
	coreMu      sync.Mutex
	coreRunning bool
	coreStopped bool
	daemonQuit  chan struct{}
)

// RunDaemonLoop Daemon 模式主循环
func RunDaemonLoop() {
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

	daemonQuit = make(chan struct{})

	// 确保 Daemon 单实例运行
	utils.EnsureSingleInstance(DaemonModePort)

	// 定义 Core 生命周期控制函数
	startCoreFunc := func() error {
		coreMu.Lock()
		coreStopped = false
		coreMu.Unlock()
		return startCoreProcess(log)
	}
	stopCoreFunc := func() error {
		coreMu.Lock()
		coreStopped = true
		coreMu.Unlock()
		return stopCoreProcess(log)
	}

	// 初始化 Daemon IPC 命名管道
	daemonIpc := &dameon.DaemonIpcInterface{}
	daemonIpc.Init(log, startCoreFunc, stopCoreFunc)

	if err := daemonIpc.CreatePipe(); err != nil {
		log.Fatalf("Failed to create Daemon IPC pipe: %v", err)
	}
	defer daemonIpc.DestroyPipe()

	// 后台启动 IPC 请求处理循环
	go daemonIpc.HandleRequest()
	log.Infof("Daemon IPC listening on pipe: %s", constant.DaemonNamePipe)

	// 初始拉起 Core 子进程
	if err := startCoreProcess(log); err != nil {
		log.Errorf("Failed to start Core initially: %v", err)
	}

	// 初始拉起 UI 子进程
	if err := dameon.StartUI(); err != nil {
		log.Errorf("Failed to start UI initially: %v", err)
	}

	// 启动 Core 保活协程
	go coreKeepAliveLoop(log)

	// 监听 OS 终止信号，用于优雅退出托盘
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("Received OS shutdown signal, quitting tray...")
		systray.Quit()
	}()

	// 创建 Context 用于安全控制托盘内部的 Goroutine 生命周期
	trayCtx, trayCancel := context.WithCancel(context.Background())
	defer trayCancel()

	// 主线程运行托盘循环
	systray.Run(func() {
		dameon.SetupTray(trayCtx)
	}, func() {
		// onExit 回调
		log.Info("Tray onExit triggered, cleaning up child processes...")

		coreMu.Lock()
		coreStopped = true
		coreMu.Unlock()

		close(daemonQuit) // 通知保活协程退出
		trayCancel()      // 通知托盘内部的监听协程退出

		_ = stopCoreProcess(log)
		_ = dameon.StopUI()
	})

	log.Info("CuckooInterface Daemon shut down gracefully.")
}

// startCoreProcess 以子进程方式启动 Core（-launch_type normal）
func startCoreProcess(log *logger.Logger) error {
	coreMu.Lock()
	defer coreMu.Unlock()

	if coreRunning {
		return fmt.Errorf("core is already running")
	}

	exeDir, err := utils.GetExecutableDir()
	if err != nil {
		return fmt.Errorf("failed to get executable dir: %w", err)
	}

	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get self executable path: %w", err)
	}

	// 拉起自身，并传入 normal 模式参数
	cmd := exec.Command(selfPath, "-"+constant.ArgTag, constant.NormalModeArgData)
	cmd.Dir = exeDir
	// 可选：将 Core 的输出重定向到 Daemon 的控制台或日志文件
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Core process: %w", err)
	}

	coreCmd = cmd
	coreRunning = true
	log.Infof("Core process started, PID: %d", cmd.Process.Pid)

	// 后台等待子进程退出，更新状态
	go func() {
		waitErr := cmd.Wait()
		coreMu.Lock()
		// 仅当 coreCmd 仍指向本 cmd 时才更新状态。
		// 避免 restart 场景下：旧进程被 kill 后，其 Wait 回调
		// 覆盖新进程已设置的 coreRunning=true / coreCmd=newCmd，
		// 进而导致保活协程误判 Core 未运行而无限重启。
		if coreCmd == cmd {
			coreRunning = false
			coreCmd = nil
		}
		coreMu.Unlock()

		if waitErr != nil {
			log.Warnf("Core process exited with error: %v", waitErr)
		} else {
			log.Info("Core process exited normally")
		}
	}()

	return nil
}

// stopCoreProcess 强制终止 Core 子进程
func stopCoreProcess(log *logger.Logger) error {
	coreMu.Lock()
	defer coreMu.Unlock()

	if !coreRunning || coreCmd == nil || coreCmd.Process == nil {
		return nil
	}

	if err := coreCmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to kill Core process: %w", err)
	}

	coreRunning = false
	coreCmd = nil
	log.Info("Core process killed")
	return nil
}

// shouldRestartCore 判断是否需要重启 Core
func shouldRestartCore() bool {
	coreMu.Lock()
	defer coreMu.Unlock()
	return !coreStopped && !coreRunning
}

// coreKeepAliveLoop 保活协程
func coreKeepAliveLoop(log *logger.Logger) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-daemonQuit:
			log.Info("Core keep-alive loop stopped")
			return
		case <-ticker.C:
			if shouldRestartCore() {
				log.Info("Core process not running, attempting restart...")
				if err := startCoreProcess(log); err != nil {
					log.Errorf("Failed to restart Core: %v", err)
				}
			}
		}
	}
}
