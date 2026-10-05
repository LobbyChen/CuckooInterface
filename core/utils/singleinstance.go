package utils

import (
	"fmt"
	"net"
	"os"
	"time"
)

// probeOldInstance 尝试连接旧实例的单实例端口。
// 连接成功说明确有旧实例在运行，返回 true。
func probeOldInstance(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// startSingleInstanceServer 启动单实例监听服务。
//
// 该服务只负责「占位」，证明当前实例已持有单实例端口，不再接受任何
// 命令。历史上这里曾对收到的 "quit" 直接 os.Exit(0)，任何本地进程都能
// 借此随时终止 Core/Daemon；现已移除全部命令处理逻辑。
func startSingleInstanceServer(port int) {
	go func() {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			// 端口被占则放弃服务，不阻塞启动
			return
		}
		defer listener.Close()
		for {
			// 只接受并立即关闭连接：连接本身即「旧实例存活」的证明，
			// 不读取、不响应任何数据。
			conn, err := listener.Accept()
			if err != nil {
				continue
			}
			conn.Close()
		}
	}()
}

// EnsureSingleInstance 确保单实例运行。
//
// 若端口已被占用（旧实例存活），新实例直接退出，把位置让给旧实例；
// 不再尝试通知旧实例退出，也不存在可供外部进程利用的命令通道。
func EnsureSingleInstance(port int) {
	if probeOldInstance(port) {
		fmt.Fprintln(os.Stderr, "another instance is already running; this instance will exit")
		os.Exit(1)
	}
	// 启动自己的监听服务，占用单实例端口
	startSingleInstanceServer(port)
}
