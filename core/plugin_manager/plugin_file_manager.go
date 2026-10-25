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

// 解压安全限制，防止 zip bomb。
const (
	maxSingleEntrySize  = 256 << 20 // 单个条目最大 256MB（解压后）
	maxTotalExtractSize = 1 << 30   // 单个压缩包总解压量最大 1GB
)

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
	// 注意：filepath.Walk 的回调约定 err != nil 时 info 可能为 nil，必须先判错。
	err = filepath.Walk(absFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 单个条目不可访问时跳过，不中断整体扫描
		}
		if info.IsDir() {
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
	// 记录本次运行中已占用的解压目录，避免不同压缩包因同名 basename
	// 碰撞同一 targetDir 造成互相覆盖（插件劫持）。
	usedDirs := make(map[string]string) // targetDir(lower) -> source zip
	// 区分Kernel和Base以及Active 同时解压
	for _, file := range Plugins {
		dir := filepath.Dir(file.fp)
		targetDir := uniqueTargetDir(runtimeDir, trimExt(file.fp), usedDirs)
		if targetDir == "" {
			continue // 无法生成唯一目录（同名的重复源文件），跳过
		}
		var list *[]*SinglePluginFile
		var typo constantPkg.PlugType
		switch dir {
		case activeDir:
			list, typo = &pm.Active, constantPkg.ACTIVE
		case baseDir:
			list, typo = &pm.Base, constantPkg.BASE
		case kernelDir:
			list, typo = &pm.Kernel, constantPkg.KERNEL
		default:
			continue // 不属于任何已知类型目录
		}
		if err := extractHelper(file.fp, targetDir); err != nil {
			delete(usedDirs, strings.ToLower(targetDir))
			continue
		}
		*list = append(*list, &SinglePluginFile{fp: file.fp, dir: targetDir, Typo: typo})
		succ++
	}
	return succ, nil
}

// uniqueTargetDir 基于 zip 文件名在 runtimeDir 下生成唯一的解压目录。
// 若与本次运行中其他压缩包的目标目录冲突，则追加序号后缀。
// 返回空串表示该源文件与已登记源完全重复（同一路径），应跳过。
func uniqueTargetDir(runtimeDir, srcNoExt string, usedDirs map[string]string) string {
	base := filepath.Base(srcNoExt)
	if base == "" || base == "." || base == string(filepath.Separator) || isDriveRoot(base) {
		return ""
	}
	candidate := filepath.Join(runtimeDir, base)
	key := strings.ToLower(candidate)
	if owner, exists := usedDirs[key]; exists {
		if strings.EqualFold(owner, srcNoExt) {
			return "" // 同一来源重复出现，跳过
		}
		// 不同来源同名：追加序号避免覆盖
		for i := 1; ; i++ {
			candidate = filepath.Join(runtimeDir, fmt.Sprintf("%s-%d", base, i))
			key = strings.ToLower(candidate)
			if _, exists := usedDirs[key]; !exists {
				break
			}
		}
	}
	usedDirs[key] = srcNoExt
	return candidate
}

// isDriveRoot 判断是否为盘符根目录形式（如 "C:" / "C:\\"）。
func isDriveRoot(s string) bool {
	t := strings.TrimRight(s, `\/:`)
	return len(t) == 1 && ((t[0] >= 'A' && t[0] <= 'Z') || (t[0] >= 'a' && t[0] <= 'z'))
}

// 按类型加载插件元数据
func (pm *PluginFileManager) LoadPluginMeta(typo constantPkg.PlugType) (uint64, error) {
	var succ uint64
	pm.lock.Lock()
	defer pm.lock.Unlock()
	var list []*SinglePluginFile
	switch typo {
	case constantPkg.ACTIVE:
		list = pm.Active
	case constantPkg.BASE:
		list = pm.Base
	case constantPkg.KERNEL:
		list = pm.Kernel
	default:
		return succ, errors.New("unsupported plugin type")
	}
	var firstErr error
	for _, spf := range list {
		if e := loadMeta(spf); e != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("load meta for %s failed: %w", spf.fp, e)
			}
			continue // 元数据加载失败不计入成功数
		}
		succ++
	}
	if succ > 0 {
		pm.isLoaded = true
	}
	return succ, firstErr
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

// RemovePlugin 按 ID 移除插件，并清理其解压目录
func (pm *PluginFileManager) RemovePlugin(id string) error {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	var target *SinglePluginFile

	for _, l := range []*[]*SinglePluginFile{&pm.Kernel, &pm.Active, &pm.Base} {
		for i, spf := range *l {
			if spf.Meta.ID == id {
				target = spf
				*l = append((*l)[:i], (*l)[i+1:]...)
				break
			}
		}
		if target != nil {
			break
		}
	}

	if target == nil {
		return fmt.Errorf("plugin %s not found", id)
	}

	// 清理解压目录
	if target.dir != "" {
		_ = os.RemoveAll(target.dir)
	}
	return nil
}

// listByType 返回对应类型的插件列表指针；不支持的类型返回 nil。
// 调用方必须持有 pm.lock。
func (pm *PluginFileManager) listByType(pluginType constantPkg.PlugType) *[]*SinglePluginFile {
	switch pluginType {
	case constantPkg.KERNEL:
		return &pm.Kernel
	case constantPkg.ACTIVE:
		return &pm.Active
	case constantPkg.BASE:
		return &pm.Base
	default:
		return nil
	}
}

// AddExternalPlugin 添加外部未解压的插件文件，并返回登记后的插件条目。
func (pm *PluginFileManager) AddExternalPlugin(pluginPath string, pluginType constantPkg.PlugType) (*SinglePluginFile, error) {
	// 验证文件是否存在
	if !utils.IsFileExist(pluginPath) {
		return nil, fmt.Errorf("plugin file not found: %s", pluginPath)
	}

	// 获取绝对路径
	absPath, err := filepath.Abs(pluginPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// 获取可执行目录作为运行时目录
	baseDirForExtract, err := utils.GetExecutableDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable dir: %w", err)
	}
	runtimeDir := filepath.Join(baseDirForExtract, constantPkg.RuntimePluginFolder)

	pm.lock.Lock()
	defer pm.lock.Unlock()

	list := pm.listByType(pluginType)
	if list == nil {
		return nil, fmt.Errorf("unsupported plugin type: %v", pluginType)
	}

	// 同一来源文件重复添加时直接返回既有登记项，避免重复解压/加载
	for _, spf := range *list {
		if strings.EqualFold(spf.fp, absPath) {
			return spf, nil
		}
	}

	// 创建目标解压目录（同名冲突时追加序号，防止不同压缩包互相覆盖）
	usedDirs := make(map[string]string)
	for _, other := range pm.GetAllPluginsLocked() {
		usedDirs[strings.ToLower(other.dir)] = other.fp
	}
	targetDir := uniqueTargetDir(runtimeDir, trimExt(absPath), usedDirs)
	if targetDir == "" {
		return nil, fmt.Errorf("failed to allocate unique extraction dir for %s", absPath)
	}

	spf := &SinglePluginFile{
		fp:   absPath,
		dir:  targetDir,
		Typo: pluginType,
	}
	*list = append(*list, spf)
	return spf, nil
}

// GetAllPluginsLocked 返回全部插件条目；调用方必须已持有 pm.lock。
func (pm *PluginFileManager) GetAllPluginsLocked() []*SinglePluginFile {
	ret := make([]*SinglePluginFile, 0, len(pm.Active)+len(pm.Base)+len(pm.Kernel))
	ret = append(ret, pm.Active...)
	ret = append(ret, pm.Base...)
	ret = append(ret, pm.Kernel...)
	return ret
}

// ExtractAndLoadExternalPlugin 解压并加载单个外部插件
func (pm *PluginFileManager) ExtractAndLoadExternalPlugin(pluginPath string, pluginType constantPkg.PlugType) error {
	// 添加插件并精确定位刚登记的条目（避免并发添加时取错目标）
	targetPlugin, err := pm.AddExternalPlugin(pluginPath, pluginType)
	if err != nil {
		return err
	}

	// 解压插件
	if err = extractHelper(targetPlugin.fp, targetPlugin.dir); err != nil {
		pm.removeEntry(targetPlugin)
		return fmt.Errorf("failed to extract plugin: %w", err)
	}

	// 加载元数据
	if err = loadMeta(targetPlugin); err != nil {
		pm.removeEntry(targetPlugin)
		return fmt.Errorf("failed to load plugin metadata: %w", err)
	}

	pm.lock.Lock()
	pm.isLoaded = true
	pm.lock.Unlock()

	return nil
}

// removeEntry 从列表中摘除指定插件条目（用于加载失败回滚）
func (pm *PluginFileManager) removeEntry(target *SinglePluginFile) {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	if list := pm.listByType(target.Typo); list != nil {
		for i, spf := range *list {
			if spf == target {
				*list = append((*list)[:i], (*list)[i+1:]...)
				break
			}
		}
	}
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

// extractHelper 使用标准库解压。
// 安全加固：
//   - 拒绝绝对路径、盘符路径与含 ".." 的条目（防路径穿越）；
//   - 忽略条目自带权限位，统一使用 0644/0755（防 setuid 等异常模式）；
//   - 限制单条目与总解压量（zip bomb 防护），失败时清理本次新建的目标目录。
func extractHelper(fp, targetFolder string) error {
	r, err := zip.OpenReader(fp)
	if err != nil {
		return fmt.Errorf("open zip failed: %w", err)
	}
	defer r.Close()

	// 仅当目标目录本次新建时，失败才整体删除，避免误删既有数据
	createdNow := false
	if _, statErr := os.Stat(targetFolder); os.IsNotExist(statErr) {
		createdNow = true
	}
	cleanupOnFail := func() {
		if createdNow {
			_ = os.RemoveAll(targetFolder)
		}
	}

	// 解压安全限制，防止 zip bomb：写入过程中实时计数，超限立即中止。
	var written int64
	for _, f := range r.File {
		// 统一路径分隔符并拒绝绝对路径 / 含 ".." 的条目（防路径穿越）
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if strings.HasPrefix(name, "/") || filepath.IsAbs(name) || hasWindowsDrive(name) {
			cleanupOnFail()
			return fmt.Errorf("illegal absolute path in zip: %s", f.Name)
		}
		cleanName := filepath.Clean(name)
		if cleanName == ".." || strings.HasPrefix(cleanName, "../") || strings.Contains(cleanName, "/../") {
			cleanupOnFail()
			return fmt.Errorf("illegal file path in zip: %s", f.Name)
		}
		destPath := filepath.Join(targetFolder, cleanName)
		if !strings.HasPrefix(destPath, filepath.Clean(targetFolder)+string(os.PathSeparator)) &&
			destPath != filepath.Clean(targetFolder) {
			cleanupOnFail()
			return fmt.Errorf("illegal file path in zip: %s", f.Name)
		}
		// 如果是目录，创建后跳过
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				cleanupOnFail()
				return fmt.Errorf("create dir for %s failed: %w", destPath, err)
			}
			continue
		}
		// 单条目声明大小预检（拦截元数据谎报的 zip bomb）
		if int64(f.UncompressedSize64) > maxSingleEntrySize {
			cleanupOnFail()
			return fmt.Errorf("zip entry %s too large: %d bytes (max %d)", f.Name, f.UncompressedSize64, maxSingleEntrySize)
		}
		// 确保父目录存在
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			cleanupOnFail()
			return fmt.Errorf("create dir for %s failed: %w", destPath, err)
		}
		// 打开压缩包内文件
		rc, err := f.Open()
		if err != nil {
			cleanupOnFail()
			return fmt.Errorf("open entry %s failed: %w", f.Name, err)
		}
		// 忽略条目自带的权限位（可能包含 setuid/异常模式），统一使用安全权限
		mode := os.FileMode(0644)
		if f.Mode()&0111 != 0 {
			mode = 0755
		}
		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			rc.Close()
			cleanupOnFail()
			return fmt.Errorf("create file %s failed: %w", destPath, err)
		}
		// LimitReader 硬性截断：即使条目头声明的大小造假，实际写出字节数也被限制；
		// 同时累加总量，超过总限额立即失败。
		limit := io.LimitReader(rc, maxSingleEntrySize+1)
		n, copyErr := io.Copy(outFile, limit)
		outFile.Close()
		rc.Close()
		if copyErr != nil {
			cleanupOnFail()
			return fmt.Errorf("extract %s failed: %w", f.Name, copyErr)
		}
		written += n
		if written > maxTotalExtractSize || n > maxSingleEntrySize {
			cleanupOnFail()
			return fmt.Errorf("zip %s exceeds extraction limits (total %d bytes, max %d)", fp, written, maxTotalExtractSize)
		}
	}
	return nil
}

// hasWindowsDrive 判断 zip 条目名是否以 Windows 盘符开头（如 "C:/x"）。
func hasWindowsDrive(name string) bool {
	if len(name) < 2 || name[1] != ':' {
		return false
	}
	c := name[0]
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
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
