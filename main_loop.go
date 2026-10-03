package main

import (
	"CuckooInterface/core/config"
	"CuckooInterface/core/constant"
	"CuckooInterface/core/event"
	IPC "CuckooInterface/core/ipc"
	"CuckooInterface/core/logger"
	"CuckooInterface/core/plugin_manager"
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
	// 确保单实例运行
	utils.EnsureSingleInstance(NormalModePort)

	// 初始化全局日志
	log := logger.Init(logger.LoggerConfig{
		Level:         "info",
		LogDir:        utils.GetFolderPath(constant.LogFolder),
		MaxSize:       100,
		MaxBackups:    7,
		MaxAge:        30,
		Compress:      false,
		ConsoleOutput: true,
		CacheSize:     1000,
	})
	log.Info("CuckooInterface Core starting in Normal mode...")

	// 初始化核心工作目录结构
	if err := utils.InitFolders(constant.Folders); err != nil {
		log.Fatal(fmt.Sprintf("Failed to initialize core folders: %v", err))
	}

	// 初始化配置管理器
	cfgManager := config.NewCfgManager()
	cfgPath := utils.GetFolderPath(constant.ConfigFolder)
	cfgManager.SetCfgPath(cfgPath)
	if err := cfgManager.LoadValidConfig(cfgPath); err != nil {
		log.Warnf("Failed to load existing configs, starting with empty config state: %v", err)
	}

	// 初始化事件总线
	eventBus := event.NewEventBus()

	// 构造 Host API：同时注入到 CGO 桥接层（全局）和 KernelManager。
	// 必须使用同一实例，因为 KernelManager 会在 InitSingleKernelAsync 中
	// 包装 IntoLoopReport，CGO 侧的 go_into_loop_report_gateway 读取的也是该全局实例。
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

	// 8. 创建并启动 IPC 命名管道服务
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
	loadPlugins(pfm, &km, pm, log)

	// 阻塞等待退出信号或崩溃
	waitForShutdown(&km, log)
}

// loadPlugins 负责从磁盘扫描、解压并加载所有类型的插件
func loadPlugins(pfm *plugin_manager.PluginFileManager, km *plugin_manager.KernelManager, pm *plugin_manager.PluginManager, log *logger.Logger) {
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

	// 启动所有已初始化 Kernel 的事件循环
	km.StartAllLoops()

	// 自动加载 Base 和 Active 插件
	allPlugins := append([]*plugin_manager.SinglePluginFile{}, pfm.GetBasePlugins()...)
	allPlugins = append(allPlugins, pfm.GetActivePlugins()...)
	for _, pf := range allPlugins {
		if err := pm.LoadPlugin(pf); err != nil {
			log.Errorf("Failed to auto-load plugin [%s]: %v", pf.Name(), err)
		} else {
			// 为成功加载的插件注册配置文件占位
			_ = config.NewCfgManager().RegisterConfig(pf.Meta.ID)
			log.Infof("Auto-loaded plugin [%s]", pf.Name())
		}
	}
}

// waitForShutdown 阻塞主协程，直到收到操作系统的终止信号
func waitForShutdown(km *plugin_manager.KernelManager, log *logger.Logger) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	sig := <-quit
	log.Infof("Received shutdown signal: %v. Cleaning up...", sig)

	// 关闭所有内核运行时环境
	km.ShutdownAll()
	log.Info("CuckooInterface Core shut down gracefully.")
}
