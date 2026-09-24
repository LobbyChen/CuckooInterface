package plugin_manager

import (
	constantPkg "CuckooInterface/core/constant"
	"CuckooInterface/plugins"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// 加载 Kernel DLL 的超时时间。CGO 调用无法响应 Go Context 取消，
// 因此用独立 Goroutine + select 超时来防止主流程被永久阻塞。
const kernelLoadTimeout = 30 * time.Second

// kernelEntry 记录一个已加载 Kernel 的状态
type kernelEntry struct {
	kernel *plugins.Kernel
	id     string // kernel 唯一标识，用于 IntoLoopReport 分发
	isInit bool
}

type KernelManager struct {
	FileManager *PluginFileManager
	kernels     []kernelEntry
	HostAPI     *plugins.HostAPI
	HostAPIOnce sync.Once
	// initLoopReporterOnce 保证 IntoLoopReport 分发器只设置一次，
	// 避免多个 Kernel 并发初始化时互相覆盖共享的 api.IntoLoopReport。
	initLoopReporterOnce sync.Once
	lock                 sync.Mutex
}

// 注入HostAPI
func (km *KernelManager) InjectHostAPI(api *plugins.HostAPI) {
	km.lock.Lock()
	defer km.lock.Unlock()
	km.HostAPIOnce.Do(func() {
		km.HostAPI = api
	})
}

// 注入插件文件管理
func (km *KernelManager) InjectPluginFileManager(pm *PluginFileManager) {
	km.lock.Lock()
	defer km.lock.Unlock()
	km.FileManager = pm
}

// 加载单一插件核心
func (km *KernelManager) LoadSingleKernel(kernel *SinglePluginFile, ctx context.Context) error {
	// 检查有无此Kernel
	if !slices.Contains(km.FileManager.GetKernelPlugins(), kernel) {
		return errors.New("kernel plugin not found in kernel manager")
	}
	dllPath := filepath.Join(kernel.dir, constantPkg.KernelEntrance)
	// CGO 调用 (NewKernelWithDLL) 无法响应 Go Context 取消。
	// 将其放入独立 Goroutine，通过 select 监听 ctx.Done() 与超时，
	// 即使底层 C 线程仍在阻塞，Go 主流程也能及时放弃。
	type loadResult struct {
		k   *plugins.Kernel
		err error
	}
	resultCh := make(chan loadResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				resultCh <- loadResult{nil, fmt.Errorf("panic while loading kernel DLL: %v", r)}
			}
		}()
		k, e := plugins.NewKernelWithDLL(dllPath)
		resultCh <- loadResult{k, e}
	}()
	var res loadResult
	select {
	case res = <-resultCh:
	case <-ctx.Done():
		return fmt.Errorf("loading kernel DLL cancelled: %w", ctx.Err())
	case <-time.After(kernelLoadTimeout):
		return fmt.Errorf("loading kernel DLL timed out after %v", kernelLoadTimeout)
	}
	if res.err != nil {
		return res.err
	}
	// 读取 kernel ID，供 IntoLoopReport 分发使用
	kid, _, _, _ := res.k.Meta()
	km.lock.Lock()
	km.kernels = append(km.kernels, kernelEntry{kernel: res.k, id: kid, isInit: false})
	km.lock.Unlock()
	return nil
}

// 获取所有核心,不论是否加载
func (km *KernelManager) GetKernelPlugins() []*plugins.Kernel {
	km.lock.Lock()
	defer km.lock.Unlock()
	ret := make([]*plugins.Kernel, 0, len(km.kernels))
	for _, k := range km.kernels {
		ret = append(ret, k.kernel)
	}
	return ret
}

// 获取所有已经加载并初始化的核心
func (km *KernelManager) GetInitKernelPlugins() []*plugins.Kernel {
	km.lock.Lock()
	defer km.lock.Unlock()
	ret := []*plugins.Kernel{}
	for _, k := range km.kernels {
		if k.isInit {
			ret = append(ret, k.kernel)
		}
	}
	return ret
}

// 初始化单一的核心
func (km *KernelManager) InitSingleKernelAsync(kernel *plugins.Kernel) <-chan error {
	ch := make(chan error, 1)
	go func() {
		defer close(ch)
		km.lock.Lock()
		api := km.HostAPI
		km.initLoopReporterOnce.Do(func() {
			if api != nil {
				original := api.IntoLoopReport
				api.IntoLoopReport = func(kernelID string) {
					if original != nil {
						original(kernelID)
					}
					km.dispatchLoopReport(kernelID)
				}
			}
		})
		// 检查是否已加载
		found := false
		for _, k := range km.kernels {
			if k.kernel == kernel {
				found = true
				break
			}
		}
		km.lock.Unlock()
		if api == nil {
			ch <- errors.New("host api not injected")
			return
		}
		if !found {
			ch <- errors.New("kernel not loaded")
			return
		}
		err := kernel.InitRuntime(api)
		km.lock.Lock()
		for i, k := range km.kernels {
			if k.kernel == kernel {
				km.kernels[i].isInit = err == nil
				break
			}
		}
		km.lock.Unlock()
		ch <- err
	}()
	return ch
}

// dispatchLoopReport 根据 kernel_id 找到对应 Kernel 并通知其进入循环。
// 使用 select default 防止 StartLoop 已退出导致 channel 阻塞。
func (km *KernelManager) dispatchLoopReport(kernelID string) {
	km.lock.Lock()
	var target *plugins.Kernel
	for _, k := range km.kernels {
		if k.id == kernelID {
			target = k.kernel
			break
		}
	}
	km.lock.Unlock()
	if target == nil {
		return
	}
	if loopCh := target.GetChan(); loopCh != nil {
		select {
		case loopCh <- struct{}{}:
		default:
		}
	}
}

// 利用单一核心加载插件
func (km *KernelManager) LoadSinglePluginWithKernel(singlePlugin *SinglePluginFile) (*plugins.Kernel, *plugins.PluginDescriptor, error) {
	// 检查plugin是否有对应Kernel
	rt := singlePlugin.Meta.RuntimeType
	var matchedKernel *plugins.Kernel
	km.lock.Lock()
	for _, kernel := range km.kernels {
		// get meta
		_, _, _, runtime := kernel.kernel.Meta()
		if rt == runtime {
			if !kernel.isInit {
				km.lock.Unlock()
				return nil, nil, errors.New("valid kernel plugin does not init")
			}
			matchedKernel = kernel.kernel
			break
		}
	}
	km.lock.Unlock()
	if matchedKernel == nil {
		return nil, nil, errors.New("supported kernel plugin not found in kernel manager")
	}
	s, err := readMetaString(singlePlugin)
	if err != nil {
		return nil, nil, err
	}
	p, err := matchedKernel.LoadPlugin(singlePlugin.dir, s)
	if err != nil {
		return nil, nil, err
	}
	return matchedKernel, p, nil
}

// StartAllLoops 启动所有已初始化 Kernel 的事件循环
func (km *KernelManager) StartAllLoops() {
	km.lock.Lock()
	kernels := make([]*plugins.Kernel, 0, len(km.kernels))
	for _, k := range km.kernels {
		if k.isInit {
			kernels = append(kernels, k.kernel)
		}
	}
	km.lock.Unlock()
	for _, k := range kernels {
		func() {
			if r := recover(); r != nil {
				fmt.Printf("[KernelManager] Kernel loop panicked: %v\n", r)
			}
			if err := k.StartLoop(); err != nil {
				fmt.Printf("[KernelManager] Failed to start kernel loop: %v\n", err)
			}
		}()
	}
}

// ShutdownAll 关闭所有 Kernel
func (km *KernelManager) ShutdownAll() {
	km.lock.Lock()
	defer km.lock.Unlock()
	for _, k := range km.kernels {
		if k.isInit {
			k.kernel.ShutdownRuntime()
		}
		k.kernel.Destroy()
	}
}
