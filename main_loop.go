package main

import (
	"CuckooInterface/core/config"
	"CuckooInterface/core/constant"
	"CuckooInterface/core/event"
	IPC "CuckooInterface/core/ipc"
	ckl "CuckooInterface/core/logger"
	"CuckooInterface/core/plugin_manager"
	"CuckooInterface/core/utils"
	"CuckooInterface/plugins"
	"context"
	"fmt"
	"path/filepath"
	"sync"
)

// CoreApp 封装核心业务逻辑
type CoreApp struct {
	logger     *ckl.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	ebus       *event.EventBus
	fileMgr    *plugin_manager.PluginFileManager
	kernelMgr  *plugin_manager.KernelManager
	pluginMgr  *plugin_manager.PluginManager
	ipc        *IPC.CoreIpcInterface
	cfgMgr     *config.CfgManager
	runtimeDir string
	isRunning  bool
	mu         sync.Mutex
}

// NewCoreApp 创建核心应用实例
func NewCoreApp() *CoreApp {
	return &CoreApp{}
}

// Init 初始化核心组件
func (app *CoreApp) Init() error {
	app.mu.Lock()
	defer app.mu.Unlock()

	if app.isRunning {
		return fmt.Errorf("core is already running")
	}

	// 每次 Core 启动都创建新的上下文，保证 Daemon 的 restartCore 可真正重新启动。
	app.ctx, app.cancel = context.WithCancel(context.Background())

	// 获取目录
	var err error
	app.runtimeDir, err = utils.GetExecutableDir()
	if err != nil {
		return fmt.Errorf("failed to get executable dir: %w", err)
	}

	// 初始化日志
	app.logger = ckl.Init(ckl.DefaultLoggerConfig())
	app.logger.Info("Initializing CuckooInterface Core...")

	// 初始化文件夹
	if err := utils.InitFolders(constant.Folders); err != nil {
		return fmt.Errorf("failed to create folders: %w", err)
	}

	// 创建核心组件
	app.ebus = event.NewEventBus()
	app.fileMgr = plugin_manager.NewFileManager()
	app.kernelMgr = &plugin_manager.KernelManager{}
	app.cfgMgr = &config.CfgManager{}
	app.cfgMgr.SetCfgPath(filepath.Join(app.runtimeDir, constant.ConfigFolder))

	// 加载配置
	if err := app.cfgMgr.LoadValidConfig(filepath.Join(app.runtimeDir, constant.ConfigFolder)); err != nil {
		app.logger.Warnf("Failed to load valid config: %v", err)
	}

	// 注入 HostAPI
	api := &plugins.HostAPI{
		LogError: func(msg string) { app.logger.Error(msg) },
		LogInfo:  func(msg string) { app.logger.Info(msg) },
		GetPluginConfig: func(pluginName string) string {
			var cfg config.SinglePluginConfig
			err := app.cfgMgr.GetPluginConfig(pluginName, &cfg)
			if err != nil {
				app.logger.Errorf("Failed to get plugin config for %s: %v", pluginName, err)
				return ""
			}
			return cfg.GetConfigString()
		},
		EmitEvent: func(eventName, payload string) {
			app.ebus.Emit(eventName, payload)
		},
		IntoLoopReport: func(kernelID string) {}, // 将在 KernelManager 中具体实现
		Panic: func(reason string) {
			app.logger.Errorf("Kernel panic: %s", reason)
		},
	}
	app.kernelMgr.InjectHostAPI(api)
	plugins.InjectHostAPI(api)
	app.kernelMgr.InjectPluginFileManager(app.fileMgr)

	// 解压并扫描插件
	app.logger.Info("Unzipping and scanning plugins...")
	if cnt, err := app.fileMgr.UnZipAllPlugins(constant.UserFolder); err != nil {
		app.logger.Errorf("Unzip failed: %v", err)
	} else {
		app.logger.Infof("Unzipped %d plugin packages", cnt)
	}

	for _, t := range []constant.PlugType{constant.KERNEL, constant.BASE, constant.ACTIVE} {
		if cnt, err := app.fileMgr.LoadPluginMeta(t); err != nil {
			app.logger.Errorf("Load meta for type %d failed: %v", t, err)
		} else {
			app.logger.Infof("Loaded %d plugins metadata for type %d", cnt, t)
		}
	}

	// 加载并初始化 Kernel
	app.logger.Info("Loading kernel plugins...")
	for _, kFile := range app.fileMgr.GetKernelPlugins() {
		if err := app.kernelMgr.LoadSingleKernel(kFile, app.ctx); err != nil {
			app.logger.Errorf("Load kernel %s failed: %v", kFile.Name(), err)
			continue
		}
	}

	var initChans []<-chan error
	for _, k := range app.kernelMgr.GetKernelPlugins() {
		initChans = append(initChans, app.kernelMgr.InitSingleKernelAsync(k))
	}

	for _, ch := range initChans {
		if err := <-ch; err != nil {
			app.logger.Errorf("Kernel init failed: %v", err)
		}
	}
	app.logger.Info("All kernels initialized successfully")
	app.kernelMgr.StartAllLoops()

	// 加载 Base/Active 插件
	app.pluginMgr = plugin_manager.NewPluginManager(app.logger, app.kernelMgr, app.fileMgr, app.ebus)
	app.logger.Info("Loading Base and Active plugins...")
	allPlugins := append(app.fileMgr.GetBasePlugins(), app.fileMgr.GetActivePlugins()...)
	for _, p := range allPlugins {
		if err := app.pluginMgr.LoadPlugin(p); err != nil {
			app.logger.Errorf("Load plugin %s failed: %v", p.Name(), err)
		} else {
			app.logger.Infof("Plugin %s loaded and registered", p.Name())
		}
	}

	// 启动 IPC
	app.ipc = &IPC.CoreIpcInterface{}
	app.ipc.Init(app.kernelMgr, app.fileMgr, app.pluginMgr, app.ebus, app.logger)
	if err := app.ipc.CreatePipe(); err != nil {
		return fmt.Errorf("failed to create IPC pipe: %w", err)
	}

	go app.ipc.ServeLoop()
	app.logger.Info("IPC pipe server started, waiting for frontend connections...")

	app.isRunning = true
	return nil
}

// Run 阻塞运行直到上下文取消
func (app *CoreApp) Run() {
	<-app.ctx.Done()
	app.Shutdown()
}

// Shutdown 关闭核心业务
func (app *CoreApp) Shutdown() {
	app.mu.Lock()
	defer app.mu.Unlock()

	if !app.isRunning {
		return
	}

	app.logger.Info("Shutting down Core...")

	// 关闭 IPC
	if app.ipc != nil {
		app.ipc.DestroyPipe()
	}

	// 关闭所有 Kernel
	if app.kernelMgr != nil {
		app.kernelMgr.ShutdownAll()
	}

	// 取消上下文
	app.cancel()

	app.logger.Info("Core shutdown complete.")
	app.isRunning = false
}

// IsRunning 检查核心是否正在运行
func (app *CoreApp) IsRunning() bool {
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.isRunning
}
