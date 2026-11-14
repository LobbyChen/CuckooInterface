// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package daemon

import (
	"CuckooInterface/core/constant"
	IPC "CuckooInterface/core/ipc"
	"CuckooInterface/core/logger"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Microsoft/go-winio"
)

// IPC 读写超时
const (
	ipcReadTimeout  = 5 * time.Second
	ipcWriteTimeout = 5 * time.Second
)

// CoreControlFunc 定义 Core 生命周期控制函数。
type CoreControlFunc func() error

// DaemonIpc 是 Daemon 暴露给 UI 的控制面
type DaemonIpc struct {
	isUiRunning   atomic.Bool
	isCoreRunning atomic.Bool

	listenerMu sync.Mutex
	listener   net.Listener
	pipeName   string
	closing    atomic.Bool

	logger *logger.Logger

	// controlMu 串行化 Core 启停操作，避免同一时刻执行多次。
	controlMu sync.Mutex
	startCore CoreControlFunc
	stopCore  CoreControlFunc

	funcMap map[string]func(params json.RawMessage) (json.RawMessage, error)
}

func NewDaemonIpc() *DaemonIpc {
	return &DaemonIpc{}
}

// Init 注入日志与 Core 生命周期控制函数。
func (ipc *DaemonIpc) Init(log *logger.Logger, startCore, stopCore CoreControlFunc) {
	ipc.isUiRunning.Store(false)
	ipc.isCoreRunning.Store(false)
	ipc.logger = log
	ipc.startCore = startCore
	ipc.stopCore = stopCore
	ipc.funcMap = map[string]func(params json.RawMessage) (json.RawMessage, error){
		IPC.MethodDaemonPing:        ipc.handlePing,
		IPC.MethodDaemonStartCore:   ipc.handleStartCore,
		IPC.MethodDaemonStopCore:    ipc.handleStopCore,
		IPC.MethodDaemonRestartCore: ipc.handleRestartCore,
	}
}

// StartPipe 创建本次会话的命名管道，并返回带随机后缀的完整管道名。
func (ipc *DaemonIpc) StartPipe() (string, error) {
	npipe := constant.DaemonNamePipe + GenerateHash()[:36]
	l, err := winio.ListenPipe(npipe, nil)
	if err != nil {
		return "", fmt.Errorf("failed to listen on pipe %s: %w", npipe, err)
	}

	ipc.listenerMu.Lock()
	if ipc.listener != nil {
		ipc.listenerMu.Unlock()
		_ = l.Close()
		return "", fmt.Errorf("daemon IPC pipe is already running")
	}
	ipc.listener = l
	ipc.pipeName = npipe
	ipc.listenerMu.Unlock()

	ipc.closing.Store(false)
	return npipe, nil
}

// PipeName 返回当前管道名，供启动 UI 时通过 -daemonpipe 传入。
func (ipc *DaemonIpc) PipeName() string {
	ipc.listenerMu.Lock()
	defer ipc.listenerMu.Unlock()
	return ipc.pipeName
}

// DestroyPipe 关闭监听器并终止 ServeLoop。
func (ipc *DaemonIpc) DestroyPipe() error {
	ipc.listenerMu.Lock()
	l := ipc.listener
	ipc.listener = nil
	ipc.pipeName = ""
	ipc.listenerMu.Unlock()

	if l == nil {
		return nil
	}
	ipc.closing.Store(true)
	return l.Close()
}

// ServeLoop 串行处理 UI 请求
func (ipc *DaemonIpc) ServeLoop() {
	for {
		l := ipc.currentListener()
		if l == nil {
			return
		}
		conn, err := l.Accept()
		if err != nil {
			if ipc.closing.Load() {
				return
			}
			ipc.logger.Errorf("Daemon IPC accept failed: %v", err)
			return
		}
		ipc.handleConnection(conn)
	}
}

func (ipc *DaemonIpc) currentListener() net.Listener {
	ipc.listenerMu.Lock()
	defer ipc.listenerMu.Unlock()
	return ipc.listener
}

// handleConnection 在单个连接上完成一次请求-响应。
func (ipc *DaemonIpc) handleConnection(conn net.Conn) {
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(ipcReadTimeout))
	data, err := IPC.ReadSinglePacket(conn)
	if err != nil {
		ipc.logger.Errorf("Daemon IPC read failed: %v", err)
		return
	}

	response := ipc.HandleRequest(data)

	_ = conn.SetWriteDeadline(time.Now().Add(ipcWriteTimeout))
	if _, err := conn.Write(IPC.GeneralPkg(response)); err != nil {
		ipc.logger.Errorf("Daemon IPC write failed: %v", err)
	}
}

// HandleRequest 解析请求并分发
func (ipc *DaemonIpc) HandleRequest(data []byte) []byte {
	var req IPC.IpcRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return ipc.errorResp(0, "invalid request: "+err.Error())
	}

	handler, ok := ipc.funcMap[req.Method]
	if !ok {
		return ipc.errorResp(req.ID, "unknown method: "+req.Method)
	}

	result, err := handler(req.Params)
	if err != nil {
		return ipc.errorResp(req.ID, err.Error())
	}
	return ipc.successResp(req.ID, result)
}

// 方法处理

func (ipc *DaemonIpc) handlePing(_ json.RawMessage) (json.RawMessage, error) {
	return json.Marshal("pong")
}

func (ipc *DaemonIpc) handleStartCore(_ json.RawMessage) (json.RawMessage, error) {
	ipc.controlMu.Lock()
	defer ipc.controlMu.Unlock()

	if ipc.startCore == nil {
		return nil, fmt.Errorf("start core is not configured")
	}
	// 是否真的需要启动由保活循环判断，这里只负责发送命令。
	if err := ipc.startCore(); err != nil {
		return nil, fmt.Errorf("start core failed: %w", err)
	}
	ipc.isCoreRunning.Store(true)
	ipc.logger.Info("Core start requested via daemon IPC")
	return json.Marshal(true)
}

func (ipc *DaemonIpc) handleStopCore(_ json.RawMessage) (json.RawMessage, error) {
	ipc.controlMu.Lock()
	defer ipc.controlMu.Unlock()

	if ipc.stopCore == nil {
		return nil, fmt.Errorf("stop core is not configured")
	}
	if err := ipc.stopCore(); err != nil {
		return nil, fmt.Errorf("stop core failed: %w", err)
	}
	ipc.isCoreRunning.Store(false)
	ipc.logger.Info("Core stop requested via daemon IPC")
	return json.Marshal(true)
}

func (ipc *DaemonIpc) handleRestartCore(_ json.RawMessage) (json.RawMessage, error) {
	ipc.controlMu.Lock()
	defer ipc.controlMu.Unlock()

	// 重启必须是 stop → start 的完整序列，不能根据本地状态跳过 stop。
	if ipc.stopCore != nil {
		if err := ipc.stopCore(); err != nil {
			ipc.logger.Warnf("stop core during restart failed: %v", err)
		}
		ipc.isCoreRunning.Store(false)
	}
	if ipc.startCore == nil {
		return nil, fmt.Errorf("start core is not configured")
	}
	if err := ipc.startCore(); err != nil {
		return nil, fmt.Errorf("restart core failed: %w", err)
	}
	ipc.isCoreRunning.Store(true)
	ipc.logger.Info("Core restart requested via daemon IPC")
	return json.Marshal(true)
}

// 状态同步
func (ipc *DaemonIpc) SetUIRunning(running bool) {
	ipc.isUiRunning.Store(running)
}

func (ipc *DaemonIpc) SetCoreRunning(running bool) {
	ipc.isCoreRunning.Store(running)
}

// 响应辅助

func (ipc *DaemonIpc) successResp(id int64, data json.RawMessage) []byte {
	resp := IPC.IpcResponse{ID: id, Success: true, Data: data}
	b, _ := json.Marshal(resp)
	return b
}

func (ipc *DaemonIpc) errorResp(id int64, errMsg string) []byte {
	resp := IPC.IpcResponse{ID: id, Success: false, Error: errMsg}
	b, _ := json.Marshal(resp)
	return b
}
