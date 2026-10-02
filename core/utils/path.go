package utils

import (
	"os"
	"path/filepath"
)

// GetExecutableDir 获取当前可执行文件所在的绝对目录
func GetExecutableDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	// 解析符号链接并获取目录
	realPath, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		// 如果解析失败，直接使用原路径的目录
		return filepath.Dir(exePath), nil
	}
	return filepath.Dir(realPath), nil
}

// GetWorkingDir 获取当前工作目录
func GetWorkingDir() (string, error) {
	return os.Getwd()
}

// JoinPaths 安全地拼接路径
func JoinPaths(base string, paths ...string) string {
	return filepath.Join(append([]string{base}, paths...)...)
}

// InitFolders 初始化所有必要的文件夹
// 如果文件夹不存在则创建，存在则跳过
func InitFolders(folders []string) error {
	basedir, err := GetExecutableDir()
	if err != nil {
		return err
	}
	for _, path := range folders {
		fullPath := filepath.Join(basedir, path)
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			return err
		}
	}
	return nil
}

// GetFolderPath 获取指定目录的绝对路径
func GetFolderPath(folderName string) string {
	basedir, err := GetExecutableDir()
	if err != nil {
		return ""
	}
	return filepath.Join(basedir, folderName)
}

// IsFile checks whether the path is a file,
// it returns false when the path is a directory or does not exist.
func IsFile(f string) bool {
	fi, e := os.Stat(f)
	if e != nil {
		return false
	}
	return !fi.IsDir()
}

// IsFileExist 判断路径是否存在且为普通文件。
func IsFileExist(f string) bool {
	return IsFile(f)
}
