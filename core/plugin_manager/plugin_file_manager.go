package plugin_manager

import (
	constantPkg "CuckooInterface/core/constant"
	"CuckooInterface/core/utils"
	"CuckooInterface/plugins"
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type SinglePluginFile struct {
	fp   string // 插件原文件路径
	dir  string // 插件解压后路径
	Typo constantPkg.PlugType
	Meta plugins.PluginMetaData
}

func (spf *SinglePluginFile) Name() string {
	return spf.Meta.Name
}

// 插件文件管理
type PluginFileManager struct {
	// 维护的plugins
	Kernel []*SinglePluginFile
	Active []*SinglePluginFile
	Base   []*SinglePluginFile
	// 锁
	lock sync.Mutex
	// 状态
	isLoaded bool
}

func NewFileManager() *PluginFileManager {
	return &PluginFileManager{
		Kernel:   make([]*SinglePluginFile, 0),
		Active:   make([]*SinglePluginFile, 0),
		Base:     make([]*SinglePluginFile, 0),
		isLoaded: false,
	}
}
func (pm *PluginFileManager) UnZipAllPlugins(pluginsFolder string) (uint64, error) {
	var succ uint64
	var Plugins []SinglePluginFile
	// 统一为绝对路径
	absFolder, err := filepath.Abs(pluginsFolder)
	if err != nil {
		return 0, err
	}
	activeDir := filepath.Join(absFolder, filepath.Base(constantPkg.ActivePluginFolder))
	baseDir := filepath.Join(absFolder, filepath.Base(constantPkg.BasePluginFolder))
	kernelDir := filepath.Join(absFolder, filepath.Base(constantPkg.KernelPluginFolder))
	// 解压目标统一放在可执行目录下的 plugins 运行时目录
	baseDirForExtract, err := utils.GetExecutableDir()
	if err != nil {
		return 0, err
	}
	runtimeDir := filepath.Join(baseDirForExtract, constantPkg.RuntimePluginFolder)
	// 对于所有文件进行遍历
	err = filepath.Walk(absFolder, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		if err != nil {
			return nil
		}
		Plugins = append(Plugins, SinglePluginFile{
			fp: path,
		})
		return nil
	})
	if err != nil {
		return 0, err
	}
	pm.lock.Lock()
	defer pm.lock.Unlock()
	// 区分Kernel和Base以及Active 同时解压
	for _, file := range Plugins {
		dir := filepath.Dir(file.fp)
		targetDir := filepath.Join(runtimeDir, filepath.Base(trimExt(file.fp)))
		switch dir {
		case activeDir:
			pm.Active = append(pm.Active, &SinglePluginFile{
				fp:   file.fp,
				dir:  targetDir,
				Typo: constantPkg.ACTIVE,
			})
			err = ExtractHelper(file.fp, targetDir)
		case baseDir:
			pm.Base = append(pm.Base, &SinglePluginFile{
				fp:   file.fp,
				dir:  targetDir,
				Typo: constantPkg.BASE,
			})
			err = ExtractHelper(file.fp, targetDir)
		case kernelDir:
			pm.Kernel = append(pm.Kernel, &SinglePluginFile{
				fp:   file.fp,
				dir:  targetDir,
				Typo: constantPkg.KERNEL,
			})
			err = ExtractHelper(file.fp, targetDir)
		}
		if err != nil {
			continue
		}
		succ++
	}
	return succ, nil
}
func (pm *PluginFileManager) LoadPluginMeta(typo constantPkg.PlugType) (uint64, error) {
	var err error
	var succ uint64
	switch typo {
	case constantPkg.ACTIVE:
		for i, _ := range pm.Active {
			pm.lock.Lock()
			loadMeta(pm.Active[i])
			succ++
			pm.lock.Unlock()
		}
	case constantPkg.BASE:
		for i, _ := range pm.Base {
			pm.lock.Lock()
			loadMeta(pm.Base[i])
			succ++
			pm.lock.Unlock()
		}
	case constantPkg.KERNEL:
		for i, _ := range pm.Kernel {
			pm.lock.Lock()
			loadMeta(pm.Kernel[i])
			succ++
			pm.lock.Unlock()
		}
	default:
		return succ, errors.New("unsupported plugin type")
	}
	if succ > 0 {
		pm.isLoaded = true
	}
	return succ, err
}
func (pm *PluginFileManager) GetKernelPlugins() []*SinglePluginFile {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	return pm.Kernel
}
func (pm *PluginFileManager) GetActivePlugins() []*SinglePluginFile {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	return pm.Active
}
func (pm *PluginFileManager) GetBasePlugins() []*SinglePluginFile {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	return pm.Base
}
func (pm *PluginFileManager) AddPlugins(onPluginAdd func(*SinglePluginFile), singlePlugin ...*SinglePluginFile) {
	// 先在锁内完成切片追加，再在锁外启动回调 goroutine
	var toNotify []*SinglePluginFile
	pm.lock.Lock()
	for _, plugin := range singlePlugin {
		switch plugin.Typo {
		case constantPkg.ACTIVE:
			pm.Active = append(pm.Active, plugin)
		case constantPkg.BASE:
			pm.Base = append(pm.Base, plugin)
		case constantPkg.KERNEL:
			pm.Kernel = append(pm.Kernel, plugin)
		default:
			continue
		}
		toNotify = append(toNotify, plugin)
	}
	pm.lock.Unlock()
	for _, plugin := range toNotify {
		go onPluginAdd(plugin)
	}
}
func (pm *PluginFileManager) IsLoaded() bool {
	return pm.isLoaded
}

// ExtractHelper 使用标准库解压，兼容 Windows/PowerShell 创建的 ZIP
func ExtractHelper(fp, targetFolder string) error {
	r, err := zip.OpenReader(fp)
	if err != nil {
		return fmt.Errorf("open zip failed: %w", err)
	}
	defer r.Close()
	for _, f := range r.File {
		// 统一路径分隔符
		cleanName := filepath.Clean(strings.ReplaceAll(f.Name, "\\", "/"))
		destPath := filepath.Join(targetFolder, cleanName)
		if !strings.HasPrefix(destPath, filepath.Clean(targetFolder)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path in zip: %s", f.Name)
		}
		// 如果是目录，创建后跳过
		if f.FileInfo().IsDir() {
			os.MkdirAll(destPath, 0755)
			continue
		}
		// 确保父目录存在
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("create dir for %s failed: %w", destPath, err)
		}
		// 解压文件
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open entry %s failed: %w", f.Name, err)
		}
		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return fmt.Errorf("create file %s failed: %w", destPath, err)
		}
		_, copyErr := io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if copyErr != nil {
			return fmt.Errorf("extract %s failed: %w", f.Name, copyErr)
		}
	}
	return nil
}

// trimExt 去除扩展名
func trimExt(f string) string {
	ext := filepath.Ext(f)            // 获取扩展名
	return strings.TrimSuffix(f, ext) // 去除扩展名
}

// ReadPlugin 阅读插件配置
func loadMeta(plugin *SinglePluginFile) error {
	cfgfp := filepath.Join(plugin.dir, constantPkg.MetaFile)
	// check if file valid
	if !utils.IsFileExist(cfgfp) {
		return errors.New("file not exist")
	}
	obj, e := utils.LoadJson[plugins.PluginMetaData](cfgfp)
	if e != nil {
		return e
	}
	plugin.Meta = obj
	return nil
}

// readMetaString 返回 META-INF.json 的原始内容。
func readMetaString(plugin *SinglePluginFile) (string, error) {
	cfgfp := filepath.Join(plugin.dir, constantPkg.MetaFile)
	if !utils.IsFileExist(cfgfp) {
		return "", errors.New("file not exist")
	}
	data, err := os.ReadFile(cfgfp)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
