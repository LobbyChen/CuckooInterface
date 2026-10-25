package main

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/logger"
	"CuckooInterface/core/service"
	"CuckooInterface/core/utils"
	"CuckooInterface/daemon"
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

	// 注册为系统服务后，Core 由 SCM 管理，Daemon 不应再拉起第二个 Core。
	managedByService := service.IsRunning()
	if managedByService {
		log.Info("Core is managed by Windows service; daemon will only provide UI/tray")
	}

	// 定义 Core 生命周期控制函数
	startCoreFunc := func() error {
		if service.IsRunning() {
			return fmt.Errorf("Core 由 Windows 服务管理，请在系统服务中启动")
		}
		coreMu.Lock()
		coreStopped = false
		coreMu.Unlock()
		return startCoreProcess(log)
	}
	stopCoreFunc := func() error {
		if service.IsRunning() {
			return fmt.Errorf("Core 由 Windows 服务管理，请在系统服务中停止")
		}
		coreMu.Lock()
		coreStopped = true
		coreMu.Unlock()
		return stopCoreProcess(log)
	}

	// 初始化 Daemon IPC 命名管道
	daemonIpc := &daemon.DaemonIpcInterface{}
	daemonIpc.Init(log, startCoreFunc, stopCoreFunc)

	if err := daemonIpc.CreatePipe(); err != nil {
		log.Fatalf("Failed to create Daemon IPC pipe: %v", err)
	}
	defer daemonIpc.DestroyPipe()

	// 后台启动 IPC 请求处理循环
	go daemonIpc.HandleRequest()
	log.Infof("Daemon IPC listening on pipe: %s", constant.DaemonNamePipe)

	// 初始拉起 Core 子进程（服务模式下跳过）
	if !managedByService {
		if err := startCoreProcess(log); err != nil {
			log.Errorf("Failed to start Core initially: %v", err)
		}
	}

	// 初始拉起 UI 子进程（-no_ui 用于静默开机自启）
	if daemon.ShouldOpenUI() {
		if err := daemon.StartUI(); err != nil {
			log.Errorf("Failed to start UI initially: %v", err)
		}
	} else {
		log.Info("UI auto-start disabled by -no_ui")
	}

	// 启动 Core 保活协程
	go coreKeepAliveLoop(log, managedByService)

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
		daemon.SetupTray(trayCtx)
	}, func() {
		// onExit 回调
		log.Info("Tray onExit triggered, cleaning up child processes...")

		coreMu.Lock()
		coreStopped = true
		coreMu.Unlock()

		close(daemonQuit) // 通知保活协程退出
		trayCancel()      // 通知托盘内部的监听协程退出

		_ = stopCoreProcess(log)
		_ = daemon.StopUI()
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
	if service.IsRunning() {
		return fmt.Errorf("core is managed by Windows service")
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

// stopCoreProcess 优雅终止 Core 子进程：
//  1. 先向进程投递 SIGTERM（Windows 上会触发 Go signal.Notify 处理器），
//     让 Core 有机会执行 defer 清理（关闭 IPC 管道、ShutdownAll 内核、刷日志）；
//  2. 等待最多 gracefulStopTimeout，期间轮询进程状态；
//  3. 超时仍未退出则 Kill 强杀兜底。
func stopCoreProcess(log *logger.Logger) error {
	coreMu.Lock()
	defer coreMu.Unlock()

	if !coreRunning || coreCmd == nil || coreCmd.Process == nil {
		return nil
	}

	pid := coreCmd.Process.Pid
	cmd := coreCmd

	// 第一步：请求优雅退出
	if err := coreCmd.Process.Signal(syscall.SIGTERM); err != nil {
		// 某些 Windows 场景不支持 Signal（如进程已退出/权限问题），降级为直接 Kill
		log.Warnf("SIGTERM to Core (pid %d) failed: %v; falling back to kill", pid, err)
		if kerr := cmd.Process.Kill(); kerr != nil {
			return fmt.Errorf("failed to kill Core process: %w", kerr)
		}
		coreRunning = false
		coreCmd = nil
		log.Infof("Core process (pid %d) killed", pid)
		return nil
	}

	// 第二步：带超时地等待进程真正退出。
	// 注意不能调用 cmd.Wait()——后台协程已经持有该 Wait；这里用
	// FindProcess+Signal(0)/Kill 的轮询方式探测存活状态。
	deadline := time.Now().Add(gracefulStopTimeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			coreMu.Unlock()
			// 等待后台 Wait 协程把状态字段复位（有锁保护，短暂自旋）
			waitCoreStateCleared(cmd, 2*time.Second)
			coreMu.Lock()
			if coreCmd == cmd {
				coreRunning = false
				coreCmd = nil
			}
			log.Infof("Core process (pid %d) exited gracefully", pid)
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 第三步：超时强杀兜底
	if err := cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to force kill Core process after graceful timeout: %w", err)
	}
	coreRunning = false
	coreCmd = nil
	log.Warnf("Core process (pid %d) did not exit within %v; killed", pid, gracefulStopTimeout)
	return nil
}

const gracefulStopTimeout = 8 * time.Second

// processAlive 探测 pid 是否仍在运行。
// 注意：本程序的目标平台是 Windows（核心依赖 CGO/命名管道），Windows 上
// os.FindProcess 对不存在的进程返回错误，存在但无权限时 Signal(0) 返回
// ERROR_ACCESS_DENIED——两种情况都按"存活/未知"处理，由最终 Kill 兜底。
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errno, ok := err.(syscall.Errno); ok {
		const errorAccessDenied = syscall.Errno(5)
		if errno == errorAccessDenied {
			// 进程存在但当前用户无权向其发送信号
			return true
		}
		// 其它 Win32 错误（如 ERROR_INVALID_HANDLE/ESRCH）视为进程已退出
		return false
	}
	// 非 Errno 类错误（例如 Unix 上的 "not supported"）：保守认为存活
	return true
}

// waitCoreStateCleared 等待 startCoreProcess 中的后台 Wait 协程完成状态复位。
func waitCoreStateCleared(cmd *exec.Cmd, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		coreMu.Lock()
		done := coreCmd != cmd
		coreMu.Unlock()
		if done {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// shouldRestartCore 判断是否需要重启 Core
func shouldRestartCore() bool {
	coreMu.Lock()
	defer coreMu.Unlock()
	return !coreStopped && !coreRunning
}

// coreKeepAliveLoop 保活协程
func coreKeepAliveLoop(log *logger.Logger, managedByService bool) {
	if managedByService {
		log.Info("Core keep-alive loop disabled (service-managed Core)")
		return
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-daemonQuit:
			log.Info("Core keep-alive loop stopped")
			return
		case <-ticker.C:
			if service.IsRunning() {
				// 服务可能在 Daemon 运行期间被启用，交给 SCM 管理。
				continue
			}
			if shouldRestartCore() {
				log.Info("Core process not running, attempting restart...")
				if err := startCoreProcess(log); err != nil {
					log.Errorf("Failed to restart Core: %v", err)
				}
			}
		}
	}
}
