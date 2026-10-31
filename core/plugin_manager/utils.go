package plugin_manager

import (
	"CuckooInterface/core/constant"
	"CuckooInterface/core/utils"
	"CuckooInterface/plugins"
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// extractZipEntry 从 ZIP 文件中提取指定文件，并保存到目标路径
func extractZipEntry(zipPath, entryName, destPath string) error {
	if entryName == "" {
		return fmt.Errorf("entry name cannot be empty")
	}
	if destPath == "" {
		return fmt.Errorf("destination path cannot be empty")
	}

	// 统一 ZIP 内部路径分隔符
	entryName = strings.ReplaceAll(entryName, "\\", "/")

	// 打开 ZIP 文件
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip failed: %w", err)
	}
	defer r.Close()

	// 查找指定文件
	var target *zip.File

	for _, f := range r.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")

		if name == entryName {
			// 防止 ZIP 中存在同名条目
			if target != nil {
				return fmt.Errorf("duplicate entry in zip: %s", entryName)
			}

			if f.FileInfo().IsDir() {
				return fmt.Errorf("entry is a directory: %s", entryName)
			}

			target = f
		}
	}

	if target == nil {
		return fmt.Errorf("file not found in zip: %s", entryName)
	}

	// 确保目标目录存在
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create destination directory failed: %w", err)
	}

	// 打开 ZIP 内部文件
	rc, err := target.Open()
	if err != nil {
		return fmt.Errorf("open zip entry failed: %w", err)
	}
	defer rc.Close()

	// 在目标目录创建临时文件
	// 使用临时文件，避免解压失败时留下不完整的目标文件
	tmpFile, err := os.CreateTemp(destDir, ".extract-*")
	if err != nil {
		return fmt.Errorf("create temporary file failed: %w", err)
	}

	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	defer tmpFile.Close()

	// 将 ZIP 内部文件内容复制到临时文件
	if _, err := io.Copy(tmpFile, rc); err != nil {
		return fmt.Errorf("extract zip entry failed: %w", err)
	}

	// 关闭临时文件，确保写入完成
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temporary file failed: %w", err)
	}

	// 将临时文件重命名为最终目标文件
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("rename temporary file failed: %w", err)
	}

	return nil
}

func CheckPluginType(zipPath string) (constant.PlugType, error) {
	// 检查文件存在
	if !utils.IsFileExist(zipPath) {
		return constant.PlugType(0x00), fmt.Errorf("file not vaild :%s", zipPath)
	}

	// 从插件包提取 META 到 Core 的临时目录
	tmpDir := utils.GetFolderPath(constant.TempFolder)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return constant.PlugType(0x00), fmt.Errorf("create temp directory failed: %w", err)
	}
	tmpFile, err := os.CreateTemp(tmpDir, "META-INF-*.json")
	if err != nil {
		return constant.PlugType(0x00), fmt.Errorf("create temp file failed: %w", err)
	}
	tmpFilePath := tmpFile.Name()
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpFilePath)
		return constant.PlugType(0x00), fmt.Errorf("close temp file failed: %w", err)
	}
	defer os.Remove(tmpFilePath)

	err = extractZipEntry(zipPath, constant.MetaFile, tmpFilePath)
	if err != nil {
		return constant.PlugType(0x00), err
	}

	// 读取插件类型
	obj, err := utils.LoadJson[plugins.PluginMetaData](tmpFilePath)
	if err != nil {
		return constant.PlugType(0x00), err
	}

	switch obj.Type {
	case constant.KERNEL, constant.BASE, constant.ACTIVE:
		return obj.Type, nil
	default:
		return constant.PlugType(0x00), fmt.Errorf("unsupported plugin type: %d", obj.Type)
	}
}
