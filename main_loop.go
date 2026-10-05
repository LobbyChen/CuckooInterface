package main

import (
	"CuckooInterface/core/config"
	"CuckooInterface/core/constant"
	"CuckooInterface/core/event"
	IPC "CuckooInterface/core/ipc"
	"CuckooInterface/core/logger"
	"CuckooInterface/core/plugin_manager"
	"CuckooInterface/core/provider"
	"CuckooInterface/core/provider/foundation"
	"CuckooInterface/core/provider/system/disk"
	"CuckooInterface/core/utils"
	"CuckooInterface/plugins"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// RunMainLoop 实现 Normal 模式的核心主循环
func RunMainLoop() {
	// 先初始化核心目录结构（logs/user/plugins/configs 等）。
	// 必须早于 LoadCoreConfig：首次启动时 configs 目录还不存在，
	// 默认配置落盘依赖该目录已创建。
	if err := utils.InitFolders(constant.Folders); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize core folders: %v\n", err)
		os.Exit(1)
	}

	// 初始化配置管理器
	// 加载配置
	cfgManager := config.NewCfgManager()
	cfgPath := utils.GetFolderPath(constant.ConfigFolder)
	cfgManager.SetCfgPath(cfgPath)
	// 无条件尝试加载已有插件配置：LoadValidConfig 内部对不存在的
	// 目录返回 os.ErrNotExist，此时静默跳过即可。此前用 IsFileExist
	// 判断目录恒为 false，导致插件 .cfg 配置重启后全部丢失。
	if err := cfgManager.LoadValidConfig(cfgPath); err != nil && !os.IsNotExist(err) {
		fmt.Printf("Warning: failed to load plugin configs: %v\n", err)
	}
	// 读取全局配置；文件损坏等错误不阻断启动，但需记录而非吞掉
	if err := config.LoadCoreConfig(cfgPath); err != nil {
		fmt.Printf("Warning: %v\n", err)
	}
	globalCfg := config.GetCoreConfig()
	if globalCfg.SingleInstance {
		// 确保单实例运行
		utils.EnsureSingleInstance(NormalModePort)
	}
	// 初始化全局日志
	log := logger.Init(logger.LoggerConfig{
		Level:         globalCfg.LogLevel,
		LogDir:        utils.GetFolderPath(constant.LogFolder),
		MaxSize:       int(globalCfg.LogMaxSize),
		MaxBackups:    7,
		MaxAge:        30,
		Compress:      globalCfg.LogCompress,
		ConsoleOutput: true,
		CacheSize:     1000,
	})
	log.Info("CuckooInterface Core starting in Normal mode...")

	// 初始化事件总线
	eventBus := event.NewEventBus()
	// 构造内部事件管理器
	internalEventBus := event.NewInternalEventManager(eventBus, log)

	// 注册内部事件源
	frameworkMonitor := &foundation.FrameworkMonitor{}
	registerInternalEvent(internalEventBus, frameworkMonitor, log)
	// 磁盘插拔监控（system.disk.*）
	registerInternalEvent(internalEventBus, &disk.DiskMonitor{}, log)
	// TODO 注入更多events
	// 注意：internalEventBus.Start() 延后到插件加载完成后再调用，
	// 保证 foundation.framework.started 等启动事件不会在监听器注册前丢失。
	// 构造 Host API
	hostAPI := &plugins.HostAPI{
		EmitEvent: func(eventName, payload string) {
			eventBus.Emit(eventName, payload)
		},
		LogInfo: func(msg string) {
			log.Info(msg)
		},
		LogError: func(msg string) {
			log.Error(msg)
		},
		GetPluginConfig: func(pluginName string) string {
			var spc config.SinglePluginConfig
			if err := cfgManager.GetPluginConfig(pluginName, &spc); err != nil {
				return ""
			}
			return spc.GetConfigString()
		},
		IntoLoopReport: nil, // 由 KernelManager 动态覆盖拦截
		Panic: func(reason string) {
			log.Errorf("KERNEL PANIC: %s", reason)
			_ = utils.ShowErrorBox(constant.ErrorMsgTitle, fmt.Sprintf("Kernel crashed:\n%s", reason))
		},
	}
	plugins.InjectHostAPI(hostAPI)

	// 初始化管理器层级
	pfm := plugin_manager.NewFileManager()
	km := plugin_manager.KernelManager{}
	km.InjectPluginFileManager(pfm)
	km.InjectHostAPI(hostAPI)
	pm := plugin_manager.NewPluginManager(log, &km, pfm, eventBus)
	// 插件装载状态变化时广播 foundation.plugin.loaded / unloaded
	pm.SetPluginEventHook(func(loaded bool, meta plugins.PluginMetaData, typo constant.PlugType) {
		if loaded {
			frameworkMonitor.NotifyPluginLoaded(meta.ID, meta.Name, typo)
		} else {
			frameworkMonitor.NotifyPluginUnloaded(meta.ID, meta.Name, typo)
		}
	})

	// 创建并启动 IPC 命名管道服务
	ipcBridge := &IPC.CoreIpcInterface{}
	ipcBridge.Init(&km, pfm, pm, eventBus, log)

	if err := ipcBridge.CreatePipe(); err != nil {
		log.Fatalf("Failed to create Core IPC pipe: %v", err)
	}
	defer ipcBridge.DestroyPipe()

	// 在后台 Goroutine 中持续监听 UI 请求
	go ipcBridge.ServeLoop()
	log.Infof("Core IPC listening on pipe: %s", constant.CoreNamePipe)

	// 扫描、解压并加载插件
	loadPlugins(pfm, &km, pm, log, cfgManager)

	// 所有插件加载完成后，再启动内部事件源。
	// 保证 foundation.framework.started 等启动事件能被已注册的监听器收到。
	internalEventBus.Start()

	// 阻塞等待退出信号或崩溃
	waitForShutdown(&km, log, internalEventBus)
}

// registerInternalEvent 注册一个内部事件源，并把注册过程中的错误记入日志。
func registerInternalEvent(iem *event.InternalEventManager, ev provider.InternalEvent, log *logger.Logger) {
	for _, err := range iem.AddInternalEvent(ev) {
		log.Errorf("Failed to register internal event: %v", err)
	}
}

// loadPlugins 负责从磁盘扫描、解压并加载所有类型的插件
func loadPlugins(pfm *plugin_manager.PluginFileManager, km *plugin_manager.KernelManager, pm *plugin_manager.PluginManager, log *logger.Logger, cfgManager *config.CfgManager) {
	userDir := utils.GetFolderPath(constant.UserFolder)

	// 解压用户目录下的所有插件到 runtime/plugins 目录
	count, err := pfm.UnZipAllPlugins(userDir)
	if err != nil {
		log.Errorf("Failed to unzip plugins: %v", err)
	} else {
		log.Infof("Successfully extracted %d plugin(s)", count)
	}

	// 读取元数据
	for _, typo := range []constant.PlugType{constant.KERNEL, constant.BASE, constant.ACTIVE} {
		if c, err := pfm.LoadPluginMeta(typo); err != nil {
			log.Errorf("Failed to load meta for type %v: %v", typo, err)
		} else {
			log.Infof("Loaded %d metadata for type %v", c, typo)
		}
	}

	ctx := context.Background()

	// 优先加载并初始化所有 Kernel 插件
	kernelFiles := pfm.GetKernelPlugins()
	for _, kf := range kernelFiles {
		if err := km.LoadSingleKernel(kf, ctx); err != nil {
			log.Errorf("Failed to load kernel [%s]: %v", kf.Name(), err)
			continue
		}
		kernels := km.GetKernelPlugins()
		if len(kernels) > 0 {
			targetKernel := kernels[len(kernels)-1]
			if initErr := <-km.InitSingleKernelAsync(targetKernel); initErr != nil {
				log.Errorf("Failed to init kernel [%s]: %v", kf.Name(), initErr)
			}
		}
	}

	// 自动加载 Base 和 Active 插件
	allPlugins := append([]*plugin_manager.SinglePluginFile{}, pfm.GetBasePlugins()...)
	allPlugins = append(allPlugins, pfm.GetActivePlugins()...)
	for _, pf := range allPlugins {
		if err := pm.LoadPlugin(pf); err != nil {
			log.Errorf("Failed to auto-load plugin [%s]: %v", pf.Name(), err)
		} else {
			// 为成功加载的插件注册配置文件占位。
			if err := cfgManager.RegisterConfig(pf.Meta.ID); err != nil {
				log.Warnf("Failed to register config for plugin [%s]: %v", pf.Name(), err)
			}
			log.Infof("Auto-loaded plugin [%s]", pf.Name())
		}
	}

	// 所有插件注册完成后再启动 Kernel 事件循环，避免启动早期
	// 由插件/框架发出的事件在监听器注册前被丢弃。
	km.StartAllLoops()

	// 对运行中的 Kernel 启动崩溃监控：一旦运行期崩溃（VEH 隔离后），
	// 通过 Panic 回调通知用户并更新状态，避免 UI 显示与实际不符。
	km.StartCrashMonitor()
}

// waitForShutdown 阻塞主协程，直到收到操作系统的终止信号
func waitForShutdown(km *plugin_manager.KernelManager, log *logger.Logger, iem *event.InternalEventManager) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	sig := <-quit
	log.Infof("Received shutdown signal: %v. Cleaning up...", sig)

	// 关闭所有内核运行时环境
	km.ShutdownAll()
	// 关闭内部事件
	iem.Stop()
	log.Info("CuckooInterface Core shut down gracefully.")
}
