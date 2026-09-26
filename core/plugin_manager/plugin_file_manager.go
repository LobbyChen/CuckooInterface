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

// 解压所有插件
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
			err = extractHelper(file.fp, targetDir)
		case baseDir:
			pm.Base = append(pm.Base, &SinglePluginFile{
				fp:   file.fp,
				dir:  targetDir,
				Typo: constantPkg.BASE,
			})
			err = extractHelper(file.fp, targetDir)
		case kernelDir:
			pm.Kernel = append(pm.Kernel, &SinglePluginFile{
				fp:   file.fp,
				dir:  targetDir,
				Typo: constantPkg.KERNEL,
			})
			err = extractHelper(file.fp, targetDir)
		}
		if err != nil {
			continue
		}
		succ++
	}
	return succ, nil
}

// 按类型加载插件元数据
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

// 获取现有插件文件中的Kernel插件
func (pm *PluginFileManager) GetKernelPlugins() []*SinglePluginFile {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	return pm.Kernel
}

// 获取现有插件文件中的Active插件
func (pm *PluginFileManager) GetActivePlugins() []*SinglePluginFile {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	return pm.Active
}

// 获取现有插件文件中的Base插件
func (pm *PluginFileManager) GetBasePlugins() []*SinglePluginFile {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	return pm.Base
}

// 获取所有现有插件文件
func (pm *PluginFileManager) GetAllPlugins() []*SinglePluginFile {
	ret := make([]*SinglePluginFile, 0)
	ret = append(ret, pm.GetActivePlugins()...)
	ret = append(ret, pm.GetBasePlugins()...)
	ret = append(ret, pm.GetKernelPlugins()...)
	return ret
}

// AddExternalPlugin 添加外部未解压的插件文件
func (pm *PluginFileManager) AddExternalPlugin(pluginPath string, pluginType constantPkg.PlugType) error {
	// 验证文件是否存在
	if !utils.IsFileExist(pluginPath) {
		return fmt.Errorf("plugin file not found: %s", pluginPath)
	}

	// 获取绝对路径
	absPath, err := filepath.Abs(pluginPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// 获取可执行目录作为运行时目录
	baseDirForExtract, err := utils.GetExecutableDir()
	if err != nil {
		return fmt.Errorf("failed to get executable dir: %w", err)
	}
	runtimeDir := filepath.Join(baseDirForExtract, constantPkg.RuntimePluginFolder)

	// 创建目标解压目录
	targetDir := filepath.Join(runtimeDir, filepath.Base(trimExt(absPath)))

	// 创建 SinglePluginFile 对象
	spf := &SinglePluginFile{
		fp:   absPath,
		dir:  targetDir,
		Typo: pluginType,
	}

	pm.lock.Lock()
	defer pm.lock.Unlock()

	// 根据类型添加到对应的列表
	switch pluginType {
	case constantPkg.KERNEL:
		pm.Kernel = append(pm.Kernel, spf)
	case constantPkg.ACTIVE:
		pm.Active = append(pm.Active, spf)
	case constantPkg.BASE:
		pm.Base = append(pm.Base, spf)
	default:
		return fmt.Errorf("unsupported plugin type: %v", pluginType)
	}

	return nil
}

// ExtractAndLoadPlugin 解压并加载单个外部插件
func (pm *PluginFileManager) ExtractAndLoadPlugin(pluginPath string, pluginType constantPkg.PlugType) error {
	// 先添加插件
	err := pm.AddExternalPlugin(pluginPath, pluginType)
	if err != nil {
		return err
	}

	// 找到刚添加的插件
	var targetPlugin *SinglePluginFile

	pm.lock.Lock()
	switch pluginType {
	case constantPkg.KERNEL:
		if len(pm.Kernel) > 0 {
			targetPlugin = pm.Kernel[len(pm.Kernel)-1]
		}
	case constantPkg.ACTIVE:
		if len(pm.Active) > 0 {
			targetPlugin = pm.Active[len(pm.Active)-1]
		}
	case constantPkg.BASE:
		if len(pm.Base) > 0 {
			targetPlugin = pm.Base[len(pm.Base)-1]
		}
	}
	pm.lock.Unlock()

	if targetPlugin == nil {
		return errors.New("failed to find the added plugin")
	}

	// 解压插件
	err = extractHelper(targetPlugin.fp, targetPlugin.dir)
	if err != nil {
		return fmt.Errorf("failed to extract plugin: %w", err)
	}

	// 加载元数据
	err = loadMeta(targetPlugin)
	if err != nil {
		return fmt.Errorf("failed to load plugin metadata: %w", err)
	}

	pm.lock.Lock()
	pm.isLoaded = true
	pm.lock.Unlock()

	return nil
}

// copyFile 使用 io.Copy 拷贝文件
func copyFile(src, dst string) error {
	// 打开源文件
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("无法打开源文件: %w", err)
	}
	defer sourceFile.Close()

	// 创建目标文件
	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("无法创建目标文件: %w", err)
	}
	defer destFile.Close()

	// 拷贝内容
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("拷贝文件失败: %w", err)
	}

	// 同步到磁盘
	err = destFile.Sync()
	if err != nil {
		return fmt.Errorf("同步文件失败: %w", err)
	}

	return nil
}

// extractHelper 使用标准库解压
func extractHelper(fp, targetFolder string) error {
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
