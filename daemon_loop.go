package main

import (
	"CuckooInterface/core/logger"
	"CuckooInterface/core/utils"
	"CuckooInterface/dameon"
	"fmt"
	"os"
)

// DaemonApp 封装守护进程逻辑
type DaemonApp struct {
	logger    *logger.Logger
	coreApp   *CoreApp
	daemonIpc *dameon.DaemonIpcInterface
	sigCh     chan os.Signal
}

// NewDaemonApp 创建守护进程实例
func NewDaemonApp(coreApp *CoreApp) *DaemonApp {
	return &DaemonApp{
		coreApp: coreApp,
		sigCh:   make(chan os.Signal, 1),
	}
}

// Init 初始化守护进程
func (app *DaemonApp) Init() error {
	// 初始化日志
	app.logger = logger.Init(logger.LoggerConfig{
		Level:         "info",
		LogDir:        "./log/daemon",
		MaxSize:       100,
		MaxBackups:    7,
		MaxAge:        30,
		Compress:      false,
		ConsoleOutput: true,
	})

	app.logger.Info("Initializing Daemon...")

	// 初始化 Daemon IPC 接口
	app.daemonIpc = &dameon.DaemonIpcInterface{}
	app.daemonIpc.Init(app.logger, app.startCore, app.stopCore)
	if err := app.daemonIpc.CreatePipe(); err != nil {
		return fmt.Errorf("failed to create daemon IPC pipe: %w", err)
	}
	// 拉起一次Core
	go func() {
		err := app.startCore()
		if err != nil {
			utils.ShowErrorBox("Error", fmt.Sprint(err))
		}
	}()
	// 拉起一次UI
	go dameon.StartUI()
	return nil
}

// Run 运行守护进程循环
func (app *DaemonApp) Run() {
	app.logger.Info("Daemon is running. Press Ctrl+C to exit.")

	// 在单独的 goroutine 中处理 IPC 请求
	go app.daemonIpc.HandleRequest()

	// 等待退出信号
	<-app.sigCh

	app.logger.Info("Daemon received exit signal.")
	app.Shutdown()
}

// startCore 启动核心业务逻辑
func (app *DaemonApp) startCore() error {
	if app.coreApp.IsRunning() {
		app.logger.Info("Core is already running.")
		return nil
	}

	app.logger.Info("Starting Core via Daemon...")

	// 启动 Core
	if err := app.coreApp.Init(); err != nil {
		app.logger.Errorf("Failed to initialize Core: %v", err)
		return fmt.Errorf("failed to initialize Core: %w", err)
	}

	return nil
}

// stopCore 停止核心业务逻辑
func (app *DaemonApp) stopCore() error {
	if !app.coreApp.IsRunning() {
		app.logger.Info("Core is not running.")
		return nil
	}

	app.logger.Info("Stopping Core via Daemon...")
	app.coreApp.Shutdown()
	return nil
}

// Shutdown 关闭守护进程
func (app *DaemonApp) Shutdown() {
	app.logger.Info("Shutting down Daemon...")

	// 停止 Core
	app.stopCore()

	// 停止 UI
	if err := dameon.StopUI(); err != nil {
		app.logger.Warnf("Failed to stop UI during daemon shutdown: %v", err)
	}

	// 关闭 Daemon IPC
	if app.daemonIpc != nil {
		app.daemonIpc.DestroyPipe()
	}

}
