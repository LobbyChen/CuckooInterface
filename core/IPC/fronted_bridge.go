package IPC

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/plugin_manager"
	"fmt"
	"sync"
	"time"

	pipe "gopkg.in/natefinch/npipe.v2"
)

// 与IPC的UI层接口定义

type IpcInterface struct {
	km   *plugin_manager.KernelManager
	pfm  *plugin_manager.PluginFileManager
	pm   *plugin_manager.PluginManager
	once sync.Once

	listener *pipe.PipeListener
}

func (ipc *IpcInterface) init(km *plugin_manager.KernelManager, pfm *plugin_manager.PluginFileManager, pm *plugin_manager.PluginManager) {
	ipc.once.Do(func() {
		ipc.km = km
		ipc.pfm = pfm
		ipc.pm = pm
	})
}

func (ipc *IpcInterface) check() bool {
	if ipc.km == nil || ipc.pfm == nil || ipc.pm == nil {
		return false
	}
	return true
}

func (ipc *IpcInterface) CreatePipe() error {
	l, err := pipe.Listen(constant.NamePipe)
	if err != nil {
		return err
	}
	ipc.listener = l
	return nil
}

func (ipc *IpcInterface) DestroyPipe() error {
	return ipc.listener.Close()
}

func (ipc *IpcInterface) waitForSentData(data []byte) error {
	conn, err := ipc.listener.Accept()
	if err != nil {
		return err
	}
	err = conn.SetWriteDeadline(time.Now().Add(time.Second * 1))
	if err != nil {
		return err
	}
	sentData := GeneralPkg(data)
	cnt, err := conn.Write(sentData)
	if err != nil {
		return err
	}
	if cnt != len(sentData) {
		return fmt.Errorf("expected to write %d bytes, wrote %d", len(data), cnt)
	}
	return nil
}

func (ipc *IpcInterface) waitForReceivedData() ([]byte, error) {
	conn, err := ipc.listener.Accept()
	if err != nil {
		return nil, err
	}
	// 读取包
	err = conn.SetReadDeadline(time.Now().Add(time.Second * 1))
	if err != nil {
		return nil, err
	}
	recvData, err := ReadSinglePacket(conn)
	if err != nil {
		return nil, err
	}
	return recvData, nil
}
