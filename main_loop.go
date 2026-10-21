// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
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
	"CuckooInterface/core/runtime_manager"
	"CuckooInterface/core/service"
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
	runMainLoop(context.Background(), false)
}

// runMainLoop 是 Core 的实际主循环；ctx 用于 Windows 服务停止时触发优雅退出。
func runMainLoop(ctx context.Context, isService bool) {
	// 初始化配置管理器
	// 加载配置
	cfgManager := config.NewCfgManager()
	cfgPath := utils.GetFolderPath(constant.ConfigFolder)
	cfgManager.SetCfgPath(cfgPath)
	if utils.IsFileExist(cfgPath) {
		cfgManager.LoadValidConfig(cfgPath)
	}
	config.LoadCoreConfig(cfgPath)
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
	mode := "Normal"
	if isService {
		mode = "Service"
	}
	log.Infof("CuckooInterface Core starting in %s mode...", mode)

	// 初始化核心工作目录结构
	if err := utils.InitFolders(constant.Folders); err != nil {
		log.Fatal(fmt.Sprintf("Failed to initialize core folders: %v", err))
	}

	// 开机自启动是当前用户注册表项；服务进程运行在系统账户下，不在此处理。
	if !isService {
		if err := utils.SetAutoStartForSID(globalCfg.ServiceUserSID, globalCfg.AutoStart); err != nil {
			log.Warnf("Failed to reconcile autostart setting: %v", err)
		} else if globalCfg.AutoStart {
			log.Info("Autostart is enabled (silent, no UI)")
		}
	}
	if globalCfg.ServiceMode {
		if installed, err := service.IsInstalled(); err != nil {
			log.Warnf("Failed to query Core service: %v", err)
		} else if !installed {
			log.Warnf("Service mode is enabled in config, but the Core service is not installed")
		}
	}

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
	// 启动内部事件
	internalEventBus.Start()
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
	// RuntimeManager 负责 Kernel 运行时状态、控制面与运行环境偏好。
	// 必须在 loadPlugins 之前创建，以便把 Python 运行时偏好写入进程环境。
	rm := runtime_manager.New(&km, pm, log, cfgPath)
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
	ipcBridge.Init(&km, pfm, pm, rm, eventBus, log)

	if err := ipcBridge.CreatePipe(); err != nil {
		log.Fatalf("Failed to create Core IPC pipe: %v", err)
	}
	defer ipcBridge.DestroyPipe()

	// 在后台 Goroutine 中持续监听 UI 请求
	go ipcBridge.ServeLoop()
	log.Infof("Core IPC listening on pipe: %s", constant.CoreNamePipe)

	// 扫描、解压并加载插件
	loadPlugins(pfm, &km, pm, log, cfgManager)

	// 阻塞等待退出信号、服务停止请求或崩溃
	waitForShutdown(ctx, &km, log, internalEventBus)
}

// registerInternalEvent 注册一个内部事件源，并把注册过程中的错误记入日志。
func registerInternalEvent(iem *event.InternalEventManager, ev provider.InternalEvent, log *logger.Logger) {
	for _, err := range iem.AddInternalEvent(ev) {
		log.Errorf("Failed to register internal event: %v", err)
	}
}

// loadPlugins 负责从磁盘扫描、解压并加载所有类型的插件
func loadPlugins(pfm *plugin_manager.PluginFileManager, km *plugin_manager.KernelManager, pm *plugin_manager.PluginManager, log *logger.Logger, cfgManager *config.CfgManager) {
	coreCfg := config.GetCoreConfig()
	userDir := config.GetPluginRoot()
	if err := config.EnsurePluginRoot(userDir); err != nil {
		log.Errorf("Failed to prepare plugin root [%s]: %v; falling back to default user directory", userDir, err)
		userDir = utils.GetFolderPath(constant.UserFolder)
	}
	log.Infof("Using plugin root: %s", userDir)

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
			if !coreCfg.ContinueOnError {
				log.Warn("continueOnError is disabled; stopping kernel loading")
				break
			}
			continue
		}
		kernels := km.GetKernelPlugins()
		if len(kernels) > 0 {
			targetKernel := kernels[len(kernels)-1]
			if initErr := <-km.InitSingleKernelAsync(targetKernel); initErr != nil {
				log.Errorf("Failed to init kernel [%s]: %v", kf.Name(), initErr)
				if !coreCfg.ContinueOnError {
					log.Warn("continueOnError is disabled; stopping kernel loading")
					break
				}
			}
		}
	}

	// 启动所有已初始化 Kernel 的事件循环
	km.StartAllLoops()

	if !coreCfg.AutoLoadPlugins {
		log.Info("Auto-load plugins is disabled; Base/Active plugins will not be loaded automatically")
		return
	}

	// 自动加载 Base 和 Active 插件
	allPlugins := append([]*plugin_manager.SinglePluginFile{}, pfm.GetBasePlugins()...)
	allPlugins = append(allPlugins, pfm.GetActivePlugins()...)
	for _, pf := range allPlugins {
		if err := pm.LoadPlugin(pf); err != nil {
			log.Errorf("Failed to auto-load plugin [%s]: %v", pf.Name(), err)
			if !coreCfg.ContinueOnError {
				log.Warn("continueOnError is disabled; stopping plugin loading")
				break
			}
			continue
		}
		// 为成功加载的插件注册配置文件占位。
		if err := cfgManager.RegisterConfig(pf.Meta.ID); err != nil {
			log.Warnf("Failed to register config for plugin [%s]: %v", pf.Name(), err)
		}
		log.Infof("Auto-loaded plugin [%s]", pf.Name())
	}
}

// waitForShutdown 阻塞主协程，直到收到操作系统终止信号或服务停止请求。
func waitForShutdown(ctx context.Context, km *plugin_manager.KernelManager, log *logger.Logger, iem *event.InternalEventManager) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Infof("Received shutdown signal: %v. Cleaning up...", sig)
	case <-ctx.Done():
		log.Info("Received service stop request. Cleaning up...")
	}

	// 关闭所有内核运行时环境
	km.ShutdownAll()
	// 关闭内部事件
	iem.Stop()
	log.Info("CuckooInterface Core shut down gracefully.")
}
