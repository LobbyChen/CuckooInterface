// Copyright (C) 2026 LobbyChen
// SPDX-License-Identifier: AGPL-3.0-only
package daemon

import (
	"CuckooInterface/core/constant"
	IPC "CuckooInterface/core/ipc"
	"CuckooInterface/core/logger"
	"CuckooInterface/core/utils"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	pipe "gopkg.in/natefinch/npipe.v2"
)

// CoreControlFunc 定义控制 Core 生命周期的函数签名
type CoreControlFunc func() error

// DaemonIpcInterface 定义与守护进程/UI层交互的接口
type DaemonIpcInterface struct {
	logger    *logger.Logger
	listener  *pipe.PipeListener
	startCore CoreControlFunc // 注入的启动函数
	stopCore  CoreControlFunc // 注入的停止函数
	once      sync.Once
}

// Init 初始化 Daemon IPC 接口
// startCore: 用于启动核心业务逻辑的回调
// stopCore: 用于停止核心业务逻辑的回调
func (ipc *DaemonIpcInterface) Init(logger *logger.Logger, startCore, stopCore CoreControlFunc) {
	ipc.once.Do(func() {
		ipc.logger = logger
		ipc.startCore = startCore
		ipc.stopCore = stopCore
	})
}

// CreatePipe 创建命名管道监听器
func (ipc *DaemonIpcInterface) CreatePipe() error {
	l, err := pipe.Listen(constant.DaemonNamePipe)
	if err != nil {
		return fmt.Errorf("failed to listen on pipe %s: %w", constant.DaemonNamePipe, err)
	}
	ipc.listener = l
	ipc.logger.Infof("Daemon IPC listener created on pipe: %s", constant.DaemonNamePipe)
	return nil
}

// DestroyPipe 关闭命名管道
func (ipc *DaemonIpcInterface) DestroyPipe() error {
	if ipc.listener != nil {
		err := ipc.listener.Close()
		if err != nil {
			return err
		}
		ipc.logger.Info("Daemon IPC listener closed")
	}
	return nil
}

// HandleRequest 处理来自客户端的请求循环
func (ipc *DaemonIpcInterface) HandleRequest() {
	if ipc.listener == nil {
		ipc.logger.Error("Daemon IPC listener is nil, cannot handle requests")
		return
	}

	for {
		conn, err := ipc.listener.Accept()
		if err != nil {
			ipc.logger.Errorf("Daemon IPC accept error: %v", err)
			break
		}
		ipc.handleConnection(conn)
	}
}

// handleConnection 在单个连接上完成"握手 + 一次请求-响应"。
// Daemon 管道控制 Core 生命周期（start/stop/restart），属于敏感操作，
// 必须先通过共享密钥 auth 握手；未握手的连接直接断开，不处理业务包。
func (ipc *DaemonIpcInterface) handleConnection(conn net.Conn) {
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(time.Second * 5))
	data, err := IPC.ReadSinglePacket(conn)
	if err != nil {
		ipc.logger.Warnf("Daemon IPC read error: %v", err)
		return
	}

	// —— 强制握手 ——
	var handshake IPC.IpcRequest
	if err := json.Unmarshal(data, &handshake); err != nil || handshake.Method != IPC.MethodAuth {
		ipc.logger.Warn("Daemon IPC connection rejected: auth handshake required")
		return
	}
	var authParams IPC.AuthParams
	if err := json.Unmarshal(handshake.Params, &authParams); err != nil || !utils.VerifyIPCSecret(authParams.Secret) {
		ipc.writeResp(conn, ipc.errorResp(handshake.ID, "authentication failed"))
		ipc.logger.Warn("Daemon IPC auth handshake failed: invalid shared secret")
		return
	}
	if !ipc.writeResp(conn, ipc.successResp(handshake.ID, mustMarshalJSON(true))) {
		return
	}

	// —— 业务请求 ——
	_ = conn.SetReadDeadline(time.Now().Add(time.Second * 5))
	data, err = IPC.ReadSinglePacket(conn)
	if err != nil {
		ipc.logger.Warnf("Daemon IPC read business packet error: %v", err)
		return
	}
	ipc.writeResp(conn, ipc.processCommand(data))
}

func (ipc *DaemonIpcInterface) writeResp(conn net.Conn, payload []byte) bool {
	_ = conn.SetWriteDeadline(time.Now().Add(time.Second * 5))
	if _, err := conn.Write(IPC.GeneralPkg(payload)); err != nil {
		ipc.logger.Warnf("Daemon IPC write error: %v", err)
		return false
	}
	return true
}

func mustMarshalJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("null")
	}
	return b
}

// processCommand 解析 JSON-RPC 请求并分发处理
func (ipc *DaemonIpcInterface) processCommand(data []byte) []byte {
	var req IPC.IpcRequest
	if err := json.Unmarshal(data, &req); err != nil {
		// 不回显内部解析错误细节，防止信息泄露
		return ipc.errorResp(0, "invalid request")
	}
	// auth 只允许作为握手包出现，不允许绕过握手直接调用
	if req.Method == IPC.MethodAuth {
		return ipc.errorResp(req.ID, "unauthorized")
	}

	var (
		result json.RawMessage
		err    error
	)

	switch req.Method {
	case IPC.MethodDaemonPing:
		result, err = json.Marshal("pong")
	case IPC.MethodDaemonStartCore:
		result, err = ipc.handleStartCore()
	case IPC.MethodDaemonStopCore:
		result, err = ipc.handleStopCore()
	case IPC.MethodDaemonRestartCore:
		result, err = ipc.handleRestartCore()
	default:
		return ipc.errorResp(req.ID, "unknown method: "+req.Method)
	}

	if err != nil {
		return ipc.errorResp(req.ID, err.Error())
	}
	return ipc.successResp(req.ID, result)
}

func (ipc *DaemonIpcInterface) handleStartCore() (json.RawMessage, error) {
	if ipc.startCore == nil {
		return nil, fmt.Errorf("start_core_func_not_injected")
	}
	if err := ipc.startCore(); err != nil {
		return nil, fmt.Errorf("start core failed: %w", err)
	}
	ipc.logger.Info("Core started via IPC")
	return json.Marshal(true)
}

func (ipc *DaemonIpcInterface) handleStopCore() (json.RawMessage, error) {
	if ipc.stopCore == nil {
		return nil, fmt.Errorf("stop_core_func_not_injected")
	}
	if err := ipc.stopCore(); err != nil {
		return nil, fmt.Errorf("stop core failed: %w", err)
	}
	ipc.logger.Info("Core stopped via IPC")
	return json.Marshal(true)
}

func (ipc *DaemonIpcInterface) handleRestartCore() (json.RawMessage, error) {
	// 先停止，再启动
	if ipc.stopCore != nil {
		if err := ipc.stopCore(); err != nil {
			ipc.logger.Warnf("stop core during restart failed: %v", err)
		}
	}
	if ipc.startCore == nil {
		return nil, fmt.Errorf("start_core_func_not_injected")
	}
	if err := ipc.startCore(); err != nil {
		return nil, fmt.Errorf("restart core failed: %w", err)
	}
	ipc.logger.Info("Core restarted via IPC")
	return json.Marshal(true)
}

// 响应辅助

func (ipc *DaemonIpcInterface) successResp(id int64, data json.RawMessage) []byte {
	resp := IPC.IpcResponse{ID: id, Success: true, Data: data}
	b, _ := json.Marshal(resp)
	return b
}

func (ipc *DaemonIpcInterface) errorResp(id int64, errMsg string) []byte {
	resp := IPC.IpcResponse{ID: id, Success: false, Error: errMsg}
	b, _ := json.Marshal(resp)
	return b
}
