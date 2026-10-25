// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package utils

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// IPC 共享密钥机制
//
// Core / Daemon 的命名管道默认允许同会话（甚至更低权限）进程连接，
// 而管道承载了插件安装、Core 生命周期控制等敏感操作。为此引入基于
// 用户独占文件的共享密钥：
//   - 首个启动的实例生成随机 32 字节 hex 密钥，以 0600 权限写入
//     %LOCALAPPDATA% 下的密钥文件；
//   - Core/Daemon 服务端与 UI 客户端从同一文件读取密钥；
//   - 客户端在发送任何业务请求前必须先通过 "auth" 握手校验密钥，
//     未通过握手的连接一律立即断开；
//   - 密钥比较使用恒定时间算法，防止计时侧信道。

const ipcSecretFileName = "CuckooInterface.ipc.secret"

var (
	ipcSecretCacheMu sync.Mutex
	ipcSecretCache   string
)

func ipcSecretPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = os.TempDir()
		}
		dir = filepath.Join(base, "CuckooInterface")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create ipc secret dir: %w", err)
	}
	return filepath.Join(dir, ipcSecretFileName), nil
}

// GetIPCSharedSecret 返回当前用户的 IPC 共享密钥。
// 若密钥文件不存在则原子地创建之（O_EXCL + 重读，避免两个实例并发写坏）。
// 结果在进程内缓存：轮换密钥需重启相关进程。
func GetIPCSharedSecret() (string, error) {
	ipcSecretCacheMu.Lock()
	defer ipcSecretCacheMu.Unlock()
	if ipcSecretCache != "" {
		return ipcSecretCache, nil
	}

	path, err := ipcSecretPath()
	if err != nil {
		return "", err
	}

	// 先尝试读取已有密钥
	if data, err := os.ReadFile(path); err == nil {
		secret := strings.TrimSpace(string(data))
		if isValidIPCSecret(secret) {
			ipcSecretCache = secret
			return secret, nil
		}
		// 内容非法：视为损坏，重建
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to read ipc secret file: %w", err)
	}

	// 生成新密钥并以独占方式写入
	for attempt := 0; attempt < 3; attempt++ {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("failed to generate ipc secret: %w", err)
		}
		secret := hex.EncodeToString(buf)

		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			if os.IsExist(err) {
				// 其它实例抢先创建了密钥文件，直接采用其内容
				data, rerr := os.ReadFile(path)
				if rerr != nil {
					return "", fmt.Errorf("failed to read existing ipc secret: %w", rerr)
				}
				existing := strings.TrimSpace(string(data))
				if !isValidIPCSecret(existing) {
					return "", errors.New("existing ipc secret file is malformed")
				}
				ipcSecretCache = existing
				return existing, nil
			}
			return "", fmt.Errorf("failed to create ipc secret file: %w", err)
		}
		if _, err := f.WriteString(secret); err != nil {
			f.Close()
			os.Remove(path)
			return "", fmt.Errorf("failed to write ipc secret file: %w", err)
		}
		if err := f.Close(); err != nil {
			return "", fmt.Errorf("failed to close ipc secret file: %w", err)
		}
		ipcSecretCache = secret
		return secret, nil
	}
	return "", errors.New("failed to establish ipc secret after retries")
}

// VerifyIPCSecret 用恒定时间比较校验客户端提交的密钥。
func VerifyIPCSecret(submitted string) bool {
	expected, err := GetIPCSharedSecret()
	if err != nil {
		return false
	}
	if !isValidIPCSecret(submitted) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(strings.TrimSpace(submitted))) == 1
}

func isValidIPCSecret(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
