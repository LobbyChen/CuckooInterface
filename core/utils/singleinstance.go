// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package utils

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// 单实例协议常量。
//
// 旧版本仅比较裸字符串 "quit"，任何本地进程连接端口并写入该字符串即可
// 强杀实例（拒绝服务）。现在改为基于一次性令牌的握手：
//   - 首个持有端口的实例生成随机 token；
//   - 后续实例先探测端口是否属于本应用的单实例服务（magic 校验），
//     再读取当前用户独占（0600）token 文件，携带正确 token 请求接管；
//   - 持锁实例只有在 token 匹配时才优雅退出。
const (
	siProtocolMagic   = "CUCKOO-SINGLE-INSTANCE-V2"
	siQuitTokenPrefix = "QUIT-TOKEN:"
	siMaxLineLen      = 512
)

var (
	siQuitOnce sync.Once
)

// gracefulQuit 只允许触发一次退出，避免竞态下重复 os.Exit。
func gracefulQuit() {
	siQuitOnce.Do(func() {
		os.Exit(0)
	})
}

// siTokenPath 返回当前用户的单实例令牌文件路径。
// 文件权限 0600，仅同一用户可读写；不同端口使用不同文件。
func siTokenPath(port int) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	return fmt.Sprintf("%s%cCuckooInterface-%d.token", dir, os.PathSeparator, port), nil
}

// generateSiToken 生成 32 字节的十六进制随机令牌。
func generateSiToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// writeSiTokenFile 以仅当前用户可访问的权限写入令牌文件。
func writeSiTokenFile(path, token string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(token), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readSiTokenFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if len(token) < 32 || len(token) > 128 {
		return "", errors.New("invalid token file content")
	}
	for _, ch := range token {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return "", errors.New("invalid token file content")
		}
	}
	return token, nil
}

// probeAndNotifyOldInstance 尝试与占用端口的旧实例完成令牌握手。
//
// 返回值：
//   - true:  旧实例已验证令牌并正在/已经退出，调用方可继续绑定端口；
//   - false: 端口未被占用、或占用者不是本应用实例（magic 不匹配）、
//     或令牌不可用/不匹配。调用方绝不能在未确认接管的情况下退出自身。
func probeAndNotifyOldInstance(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err != nil {
		// 端口空闲：没有旧实例。
		return false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	// 第一步：探测 magic。第三方进程占用该端口时不会应答本应用的魔数，
	// 此时直接放弃接管流程，由后续 Listen 失败来报告真实冲突。
	if _, err := conn.Write([]byte(siProtocolMagic + "\n")); err != nil {
		return false
	}
	ack := make([]byte, 1)
	if _, err := io.ReadFull(conn, ack); err != nil {
		return false
	}
	if ack[0] != 'O' { // 旧实例应答 "OK\n"
		return false
	}

	// 第二步：读取当前用户独占的令牌文件并携带令牌请求接管。
	tokenPath, err := siTokenPath(port)
	if err != nil {
		return false
	}
	token, err := readSiTokenFile(tokenPath)
	if err != nil {
		return false
	}
	if _, err := fmt.Fprintf(conn, "%s%s\n", siQuitTokenPrefix, token); err != nil {
		return false
	}
	done := make([]byte, 1)
	if _, err := io.ReadFull(conn, done); err != nil {
		return false
	}
	// 无论应答是 Q(接受) 还是 N(拒绝)，都说明对端是本应用实例；
	// 只有接受时旧实例才会退出。
	return done[0] == 'Q'
}

// startSingleInstanceServer 启动单实例监听服务，接收后续新实例的接管请求。
// onFatal 在监听器发生致命错误时被调用（携带错误），用于日志记录。
func startSingleInstanceServer(port int, token string, onFatal func(error)) {
	go func() {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			if onFatal != nil {
				onFatal(fmt.Errorf("single-instance listener failed on port %d: %w", port, err))
			}
			return
		}
		defer listener.Close()

		tokenPath, pathErr := siTokenPath(port)
		if pathErr == nil {
			if err := writeSiTokenFile(tokenPath, token); err != nil && onFatal != nil {
				onFatal(fmt.Errorf("failed to persist single-instance token: %w", err))
			}
		}

		for {
			conn, err := listener.Accept()
			if err != nil {
				// listener 被主动关闭时静默退出，避免忙循环。
				var opErr *net.OpError
				if errors.As(err, &opErr) && opErr.Err.Error() == "use of closed network connection" {
					return
				}
				if strings.Contains(err.Error(), "closed") {
					return
				}
				// 临时性错误：退避后重试，防止 accept 风暴空转 CPU。
				time.Sleep(50 * time.Millisecond)
				continue
			}
			go handleSingleInstanceConn(conn, token)
		}
	}()
}

// handleSingleInstanceConn 处理单个接管请求连接，执行令牌握手。
func handleSingleInstanceConn(conn net.Conn, token string) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	reader := io.LimitReader(conn, siMaxLineLen)
	lines := 0
	for {
		line, err := readTrimmedLine(reader)
		if err != nil {
			return // 读错误或 EOF：断开即可
		}
		if line == "" {
			continue
		}
		lines++
		if lines > 4 {
			return // 防御异常流量
		}
		switch {
		case line == siProtocolMagic:
			_, _ = conn.Write([]byte("OK\n"))
		case strings.HasPrefix(line, siQuitTokenPrefix):
			got := strings.TrimPrefix(line, siQuitTokenPrefix)
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1 {
				_, _ = conn.Write([]byte("Q\n"))
				gracefulQuit() // 令牌正确：让新实例接管
			} else {
				_, _ = conn.Write([]byte("N\n")) // 令牌错误：拒绝，不影响自身
			}
			return
		default:
			return // 未知命令：忽略并断开
		}
	}
}

// readTrimmedLine 从 r 中读取一行（\n 结尾），去除首尾空白。
func readTrimmedLine(r io.Reader) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 1)
	for sb.Len() < siMaxLineLen {
		n, err := r.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				return strings.TrimSpace(sb.String()), nil
			}
			sb.WriteByte(buf[0])
		}
		if err != nil {
			if sb.Len() > 0 {
				return strings.TrimSpace(sb.String()), nil
			}
			return "", err
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

// EnsureSingleInstance 确保单实例运行。
//
// 若已有旧实例运行且能通过令牌握手验证接管请求，则等待其退出后自己接管；
// 否则（包括端口被非本应用进程占用的情况）尝试自行绑定端口并启动监听服务。
// 只有当端口确实无法释放/绑定时才终止启动，绝不因外部伪造消息自杀。
func EnsureSingleInstance(port int) {
	token, err := generateSiToken()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to generate single-instance token:", err)
	}

	if probeAndNotifyOldInstance(port) {
		// 已通知旧实例退出，等待其释放端口。
		portReleased := false
		for i := 0; i < 30; i++ {
			time.Sleep(100 * time.Millisecond)
			if l, e := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port)); e == nil {
				l.Close()
				portReleased = true
				// 立即关闭后可能仍有短暂占用，再等一下。
				time.Sleep(200 * time.Millisecond)
				break
			}
		}
		if !portReleased {
			fmt.Fprintln(os.Stderr, "another instance is still running and did not exit in time; aborting")
			os.Exit(1)
		}
	}

	// 启动自己的监听服务，准备接收后续新实例的接管请求。
	startSingleInstanceServer(port, token, func(e error) {
		fmt.Fprintln(os.Stderr, e)
	})
}
