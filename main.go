package main

import (
	"CuckooInterface/core/config"
	"CuckooInterface/core/constant"
	"CuckooInterface/core/event"
	ckl "CuckooInterface/core/logger"
	"CuckooInterface/core/plugin_manager"
	"CuckooInterface/core/utils"
	"CuckooInterface/plugins"
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

func main() {
	// 获取软件目录
	runtimeFolder, err := utils.GetExecutableDir()
	if err != nil {
		panic(err)
	}
	// 确保单实例运行
	utils.EnsureSingleInstance()
	// 初始化上下文与日志
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := ckl.Init(ckl.DefaultLoggerConfig())
	defer logger.Sync()
	// 初始化资源文件夹
	logger.Info("Initializing system folders...")
	if err := utils.InitFolders(constant.Folders); err != nil {
		logger.Fatal("Failed to create folders: %v", err)
	}
	// 创建核心组件
	ebus := event.NewEventBus()
	fileMgr := plugin_manager.NewFileManager()
	kernelMgr := &plugin_manager.KernelManager{}
	cfgMgr := config.CfgManager{}
	// 设置配置
	cfgMgr.SetCfgPath(filepath.Join(runtimeFolder, constant.ConfigFolder))
	// 加载已经存在的配置
	cfgMgr.LoadValidConfig(filepath.Join(runtimeFolder, constant.ConfigFolder))
	// 注入 HostAPI
	api := &plugins.HostAPI{
		LogError: func(msg string) { logger.Error(msg) },
		LogInfo:  func(msg string) { logger.Info(msg) },
		GetPluginConfig: func(pluginName string) string {
			var cfg config.SinglePluginConfig
			err := cfgMgr.GetPluginConfig(pluginName, &cfg)
			if err != nil {
				logger.Errorf("Failed to get plugin config: %v", err)
				return ""
			}
			return cfg.GetConfigString()
		},
		EmitEvent: func(eventName, payload string) {
			ebus.Emit(eventName, payload)
		},
		IntoLoopReport: func(kernelID string) {},
		Panic: func(reason string) {
			logger.Errorf("Kernel panic: %s", reason)
		},
	}
	kernelMgr.InjectHostAPI(api)
	plugins.InjectHostAPI(api)
	kernelMgr.InjectPluginFileManager(fileMgr)
	// 解压并加载所有插件元数据
	logger.Info("Unzipping and scanning plugins...")
	if cnt, err := fileMgr.UnZipAllPlugins(constant.UserFolder); err != nil {
		logger.Errorf("Unzip failed: %v", err)
	} else {
		logger.Infof("Unzipped %d plugin packages", cnt)
	}
	// 加载所有类型的元数据
	for _, t := range []constant.PlugType{constant.KERNEL, constant.BASE, constant.ACTIVE} {
		if cnt, err := fileMgr.LoadPluginMeta(t); err != nil {
			logger.Errorf("Load meta for type %d failed: %v", t, err)
		} else {
			logger.Infof("Loaded %d plugins metadata for type %d", cnt, t)
		}
	}
	// 加载并初始化 Kernel
	logger.Info("Loading kernel plugins...")
	for _, kFile := range fileMgr.GetKernelPlugins() {
		if err := kernelMgr.LoadSingleKernel(kFile, ctx); err != nil {
			logger.Errorf("Load kernel %s failed: %v", kFile.Name(), err)
			continue
		}
	}
	// 异步初始化所有 Kernel
	var initChans []<-chan error
	for _, k := range kernelMgr.GetKernelPlugins() {
		initChans = append(initChans, kernelMgr.InitSingleKernelAsync(k))
	}
	// 等待所有 Kernel 初始化完成
	for _, ch := range initChans {
		if err := <-ch; err != nil {
			logger.Errorf("Kernel init failed: %v", err)
		}
	}
	logger.Info("All kernels initialized successfully")
	// 启用所有Kernel的事件循环
	logger.Info("Starting all kernel loops...")
	kernelMgr.StartAllLoops()
	// 创建上层插件管理器并加载 Base/Active 插件
	pm := plugin_manager.NewPluginManager(logger, kernelMgr, fileMgr, ebus)
	logger.Info("Loading Base and Active plugins...")
	allPlugins := append(fileMgr.GetBasePlugins(), fileMgr.GetActivePlugins()...)
	for _, p := range allPlugins {
		if err := pm.LoadPlugin(p); err != nil {
			logger.Errorf("Load plugin %s failed: %v", p.Name(), err)
		} else {
			logger.Infof("Plugin %s loaded and registered", p.Name())
		}
	}
	// 退出监听
	logger.Info("CuckooInterface is now running. Press Ctrl+C to exit.")
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logger.Info("Shutting down...")
	kernelMgr.ShutdownAll()
	cancel()
}
