package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 通过 PID 终止进程
func terminateProcess(pid uint32) error {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return fmt.Errorf("failed to open process: %v", err)
	}
	defer windows.CloseHandle(handle)

	err = windows.TerminateProcess(handle, 1)
	if err != nil {
		return fmt.Errorf("failed to terminate process: %v", err)
	}

	fmt.Printf("Successfully terminated process with PID %d\n", pid)
	return nil
}

// 通过进程名查找并杀死进程
func KillProcessByName(processName string) error {
	// 创建快照
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %v", err)
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	err = windows.Process32First(snapshot, &entry)
	if err != nil {
		return fmt.Errorf("failed to get first process: %v", err)
	}

	for {
		// 比较进程名
		name := windows.UTF16ToString(entry.ExeFile[:])
		if name == processName {
			err = terminateProcess(entry.ProcessID)
			if err != nil {
				return err
			}
		}

		err = windows.Process32Next(snapshot, &entry)
		if err != nil {
			break // 没有更多进程了
		}
	}

	return nil
}

func GetExecutableName() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Base(exePath), nil
}
