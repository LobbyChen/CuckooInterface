package utils

import (
	"fmt"
	"net"
	"os"
	"time"
)

// notifyOldInstanceToQuit 尝试连接旧实例并发送退出命令
// 返回 true 表示成功连接并通知了旧实例
func notifyOldInstanceToQuit(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("quit"))
	return true
}

// startSingleInstanceServer 启动单实例监听服务，接收后续新实例的退出通知
// 收到 "quit" 后立即结束当前进程，让新实例接管
func startSingleInstanceServer(port int) {
	go func() {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			// 端口被占则放弃服务，不阻塞启动
			return
		}
		defer listener.Close()
		for {
			conn, err := listener.Accept()
			if err != nil {
				continue
			}
			// 读取命令
			buf := make([]byte, 256)
			n, _ := conn.Read(buf)
			conn.Close()
			cmd := string(buf[:n])
			if cmd == "quit" {
				// 收到新实例的退出通知，立即退出
				os.Exit(0)
			}
		}
	}()
}

// ensureSingleInstance 确保单实例运行
// 若已有旧实例运行，则通知其退出，再启动自己的监听服务
func EnsureSingleInstance(port int) {
	if notifyOldInstanceToQuit(port) {
		// 已通知旧实例退出，等待其释放端口
		// 旧实例 os.Exit 后端口会进入 TIME_WAIT，短暂重试即可
		portReleased := false
		for i := 0; i < 30; i++ {
			time.Sleep(100 * time.Millisecond)
			// 尝试监听一次，成功说明旧实例已退出
			if l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port)); err == nil {
				l.Close()
				portReleased = true
				// 立即关闭后可能仍有 TIME_WAIT，再等一下
				time.Sleep(200 * time.Millisecond)
				break
			}
		}
		// 旧实例未在限定时间内退出，端口仍被占用，新实例不应继续启动
		if !portReleased {
			fmt.Fprintln(os.Stderr, "another instance is still running and did not exit in time; aborting")
			os.Exit(1)
		}
	}
	// 启动自己的监听服务，准备接收后续新实例的通知
	startSingleInstanceServer(port)
}
