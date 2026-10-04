package disk

import (
	"errors"
	"syscall"
	"unsafe"

	"github.com/shirou/gopsutil/disk"
)

type diskInfo struct {
	Total  int64  `json:"total"`
	Symbol string `json:"symbol"`
}

// getDiskTotalSize 获取指定盘符的磁盘总大小
// args: symbol - 盘符 string
// returns: 总大小(字节), 错误
func getDiskTotalSize(symbol string) (int64, error) {
	if len(symbol) == 0 {
		return 0, errors.New("盘符不能为空")
	}

	// 加载 kernel32.dll
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	// 确保盘符格式正确
	if len(symbol) == 1 {
		symbol += ":"
	}

	var totalBytes, freeBytes, totalAvailBytes int64

	// 调用 Windows API
	ret, _, err := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(symbol))),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalAvailBytes)),
	)

	if ret == 0 {
		return 0, err
	}

	return totalBytes, nil
}

func getDisks() []diskInfo {
	var disks []diskInfo

	// 不再使用 disk.Partitions(true)，因为它在遇到 BitLocker 锁定时会中断整个列表。

	driveLetters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	for _, letter := range driveLetters {
		drive := string(letter) + ":"

		// 尝试获取该盘的使用情况
		usage, err := disk.Usage(drive)

		if err != nil {
			continue
		}

		if usage.Total == 0 {
			continue
		}
		size, err := getDiskTotalSize(drive)
		if err != nil {
			continue
		}
		var d = diskInfo{
			Total:  size,
			Symbol: drive,
		}
		disks = append(disks, d)
	}
	return disks
}

func checkDisks(element diskInfo, slices []diskInfo) bool {
	symbol := false
	for index := range slices {
		if slices[index] == element {
			symbol = true
		}
	}
	return symbol
}

// refreshDiskInfo 返回当前磁盘列表中相对于 prevDisks 新增的那些盘。
// 与 getDiff 的区别是它只关心"新增"，且自己会重新采集一次磁盘列表。
func refreshDiskInfo(prevDisks []diskInfo) []diskInfo {
	var newDisk []diskInfo
	disks := getDisks()
	for _, disk := range disks {
		if !checkDisks(disk, prevDisks) {
			newDisk = append(newDisk, disk)
		}
	}
	return newDisk
}

func isEqual(a, b diskInfo) bool {
	return a.Total == b.Total && a.Symbol == b.Symbol
}

func getDiff(prev, curr []diskInfo) (red []diskInfo, add []diskInfo) {
	dic := make(map[diskInfo]bool)

	// 标记prev中存在的
	for _, disk := range prev {
		dic[disk] = true
	}

	// 遍历curr
	for _, disk := range curr {
		key := disk
		if dic[key] {
			dic[key] = false // 标记为已匹配
		} else {
			add = append(add, disk)
		}
	}

	// 遍历prev找出未匹配的
	for _, disk := range prev {
		if dic[disk] {
			red = append(red, disk)
		}
	}

	return red, add
}
