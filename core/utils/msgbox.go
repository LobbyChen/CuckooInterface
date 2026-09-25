package utils

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func ShowErrorBox(title, message string) error {
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return err
	}
	messagePtr, err := windows.UTF16PtrFromString(message)
	if err != nil {
		return err
	}

	ret, _, _ := windows.NewLazySystemDLL("user32.dll").
		NewProc("MessageBoxW").
		Call(0, uintptr(unsafe.Pointer(messagePtr)),
			uintptr(unsafe.Pointer(titlePtr)), 0x10)

	if ret == 0 {
		return windows.GetLastError()
	}
	return nil
}
