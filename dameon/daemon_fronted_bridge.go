package dameon

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/ipc"
	"CuckooInterface/core/logger"
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
		return fmt.Errorf("failed to listen on pipe %s: %w", constant.CoreNamePipe, err)
	}
	ipc.listener = l
	ipc.logger.Infof("Daemon IPC listener created on pipe: %s", constant.CoreNamePipe)
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
			// 管道关闭或出错，退出循环
			ipc.logger.Errorf("Daemon IPC accept error: %v", err)
			break
		}

		// 处理单个连接
		go func(c net.Conn) {
			defer c.Close()

			// 设置读取超时
			err := c.SetReadDeadline(time.Now().Add(time.Second * 5))
			if err != nil {
				return
			}

			// 读取数据包
			data, err := IPC.ReadSinglePacket(c)
			if err != nil {
				ipc.logger.Errorf("Daemon IPC read error: %v", err)
				return
			}

			// 处理数据并生成响应
			response := ipc.processCommand(data)

			// 设置写入超时
			err = c.SetWriteDeadline(time.Now().Add(time.Second * 5))
			if err != nil {
				return
			}

			// 发送响应
			sentData := IPC.GeneralPkg(response)
			_, err = c.Write(sentData)
			if err != nil {
				ipc.logger.Errorf("Daemon IPC write error: %v", err)
			}
		}(conn)
	}
}

// processCommand 根据接收到的载荷处理命令并返回响应
func (ipc *DaemonIpcInterface) processCommand(data []byte) []byte {
	command := string(data)

	var responseMsg string

	switch command {
	case "ping":
		responseMsg = "pong"
	case "start":
		if ipc.startCore != nil {
			err := ipc.startCore()
			if err != nil {
				responseMsg = fmt.Sprintf("error: %v", err)
				ipc.logger.Errorf("Failed to start core via IPC: %v", err)
			} else {
				responseMsg = "ack_start"
				ipc.logger.Info("Core started via IPC")
			}
		} else {
			responseMsg = "error: start_core_func_not_injected"
		}
	case "stop":
		if ipc.stopCore != nil {
			err := ipc.stopCore()
			if err != nil {
				responseMsg = fmt.Sprintf("error: %v", err)
				ipc.logger.Errorf("Failed to stop core via IPC: %v", err)
			} else {
				responseMsg = "ack_stop"
				ipc.logger.Info("Core stopped via IPC")
			}
		} else {
			responseMsg = "error: stop_core_func_not_injected"
		}
	default:
		responseMsg = "unknown_command"
	}

	return []byte(responseMsg)
}
