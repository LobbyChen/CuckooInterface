package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LoggerConfig 定义日志配置
type LoggerConfig struct {
	// Level 日志级别: debug, info, warn, error, dpanic, panic, fatal
	Level string `json:"level"`
	// LogDir 日志文件目录，默认为 ./log
	LogDir string `json:"log_dir"`
	// MaxSize 每个日志文件的最大大小 (MB)
	MaxSize int `json:"max_size"`
	// MaxBackups 保留旧日志文件的最大个数
	MaxBackups int `json:"max_backups"`
	// MaxAge 保留旧日志文件的最大天数
	MaxAge int `json:"max_age"`
	// Compress 是否压缩旧日志文件
	Compress bool `json:"compress"`
	// ConsoleOutput 是否输出到控制台
	ConsoleOutput bool `json:"console_output"`
	// CacheSize 内存缓存的最大日志条数，0 表示不限制（默认 1000）
	CacheSize int `json:"cache_size"`
}

// SingleLogRecord 单条结构化日志记录
type SingleLogRecord struct {
	Timestamp time.Time      `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Caller    string         `json:"caller,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"` // 保存 Infow/Errorw/With 传入的 KV 字段
}

// cacheState 共享的缓存状态（使用指针以避免 sync.RWMutex 在 Core 克隆时被意外复制）
type cacheState struct {
	mu      sync.RWMutex
	cache   []SingleLogRecord
	maxSize int
}

// CachedCore 实现 zapcore.Core 接口
type CachedCore struct {
	zapcore.LevelEnabler
	state    *cacheState
	contexts []zapcore.Field // 保存通过 With() 添加的上下文字段
}

// NewCachedCore 创建一个新的缓存 Core
func NewCachedCore(enabler zapcore.LevelEnabler, maxSize int) *CachedCore {
	return &CachedCore{
		LevelEnabler: enabler,
		state: &cacheState{
			cache:   make([]SingleLogRecord, 0, maxSize),
			maxSize: maxSize,
		},
	}
}

func (c *CachedCore) With(fields []zapcore.Field) zapcore.Core {
	clone := *c
	if len(c.contexts) > 0 || len(fields) > 0 {
		clone.contexts = make([]zapcore.Field, 0, len(c.contexts)+len(fields))
		clone.contexts = append(clone.contexts, c.contexts...)
		clone.contexts = append(clone.contexts, fields...)
	}
	return &clone
}

// Check 判断日志级别是否启用
func (c *CachedCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

// Write 核心方法：直接接收结构化数据，零解析开销存入内存
func (c *CachedCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	// 滑动窗口
	if c.state.maxSize > 0 && len(c.state.cache) >= c.state.maxSize {
		c.state.cache = c.state.cache[1:]
	}

	// 使用 zap 官方的 MapObjectEncoder 将 Field 零反射转换为 map[string]any
	enc := zapcore.NewMapObjectEncoder()

	// 注入 With() 携带的上下文字段
	for _, f := range c.contexts {
		f.AddTo(enc)
	}
	// 注入当前日志的字段
	for _, f := range fields {
		f.AddTo(enc)
	}

	record := SingleLogRecord{
		Timestamp: ent.Time,
		Level:     ent.Level.String(),
		Message:   ent.Message,
		Caller:    ent.Caller.String(), // 例如: "main.go:42"
		Fields:    enc.Fields,
	}

	c.state.cache = append(c.state.cache, record)
	return nil
}

// Sync 实现 Core 接口
func (c *CachedCore) Sync() error {
	return nil
}

// GetLogs 获取所有缓存的日志
func (c *CachedCore) GetLogs() []SingleLogRecord {
	c.state.mu.RLock()
	logs := make([]SingleLogRecord, len(c.state.cache))
	copy(logs, c.state.cache)
	c.state.mu.RUnlock()
	c.Clear()
	return logs
}

// Clear 清空缓存
func (c *CachedCore) Clear() {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.cache = c.state.cache[:0]
}

// Len 获取当前缓存的日志数量
func (c *CachedCore) Len() int {
	c.state.mu.RLock()
	defer c.state.mu.RUnlock()
	return len(c.state.cache)
}

// Logger 封装后的日志记录器
type Logger struct {
	mu           sync.RWMutex
	zapLog       *zap.SugaredLogger
	LoggerConfig LoggerConfig
	core         zapcore.Core
	writers      []zapcore.WriteSyncer // 跟踪所有当前的 writer，以便动态添加
	cores        []zapcore.Core        // 扁平维护所有子 Core，避免 AddWriter 产生嵌套 Tee 树
	cachedCore   *CachedCore           // 内存缓存 Core
}

var (
	globalLogger *Logger
	once         sync.Once
)

// DefaultLoggerConfig 返回默认配置
func DefaultLoggerConfig() LoggerConfig {
	return LoggerConfig{
		Level:         "info",
		LogDir:        "./log",
		MaxSize:       100, // MB
		MaxBackups:    7,
		MaxAge:        30, // days
		Compress:      false,
		ConsoleOutput: true,
		CacheSize:     1000, // 默认缓存 1000 条
	}
}

// Init 初始化全局日志实例
func Init(cfg ...LoggerConfig) *Logger {
	once.Do(func() {
		c := DefaultLoggerConfig()
		if len(cfg) > 0 {
			c = cfg[0]
		}
		globalLogger = NewLogger(c)
	})
	return globalLogger
}

// GetLogger 获取全局日志实例
func GetLogger() *Logger {
	if globalLogger == nil {
		return Init()
	}
	return globalLogger
}

// NewLogger 创建一个新的日志实例
func NewLogger(cfg LoggerConfig) *Logger {
	// 解析日志级别
	level := zapcore.InfoLevel
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		fmt.Printf("invalid log level '%s', using info\n", cfg.Level)
		level = zapcore.InfoLevel
	}

	// 确保日志目录存在
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		fmt.Printf("failed to create log directory %s: %v\n", cfg.LogDir, err)
		os.Exit(1)
	}

	// 配置 Lumberjack 进行日志切割
	lumberJackLogger := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.LogDir, "app.log"),
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
	}

	// 创建 WriteSyncers
	var writers []zapcore.WriteSyncer
	fileWriter := zapcore.AddSync(lumberJackLogger)
	writers = append(writers, fileWriter)

	if cfg.ConsoleOutput {
		consoleWriter := zapcore.AddSync(os.Stdout)
		writers = append(writers, consoleWriter)
	}

	// 创建编码器配置
	encoderLoggerConfig := zap.NewProductionEncoderConfig()
	encoderLoggerConfig.TimeKey = "time"
	encoderLoggerConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderLoggerConfig.CallerKey = "caller"
	encoderLoggerConfig.MessageKey = "msg"
	encoderLoggerConfig.LevelKey = "level"
	encoderLoggerConfig.StacktraceKey = "stacktrace"

	cores := make([]zapcore.Core, 0)

	// 文件 Core (JSON)
	fileCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderLoggerConfig),
		fileWriter,
		level,
	)
	cores = append(cores, fileCore)

	// 控制台 Core (Console/Color)
	if cfg.ConsoleOutput {
		consoleEncoderLoggerConfig := encoderLoggerConfig
		consoleEncoderLoggerConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		consoleCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(consoleEncoderLoggerConfig),
			zapcore.AddSync(os.Stdout),
			level,
		)
		cores = append(cores, consoleCore)
	}

	// 创建内存缓存 Core
	cacheSize := cfg.CacheSize
	if cacheSize <= 0 {
		cacheSize = 1000
	}
	cachedCore := NewCachedCore(level, cacheSize)
	cores = append(cores, cachedCore)

	// 初始化合并的 Core
	core := zapcore.NewTee(cores...)

	// 创建 Zap Logger
	zapLogger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	sugar := zapLogger.Sugar()

	l := &Logger{
		zapLog:       sugar,
		LoggerConfig: cfg,
		core:         core,
		writers:      writers,
		cores:        cores,
		cachedCore:   cachedCore, // 注入缓存 Core
	}
	return l
}

// AddWriter 动态添加额外的 io.Writer 到日志输出中。
func (l *Logger) AddWriter(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()

	newWriter := zapcore.AddSync(w)
	l.writers = append(l.writers, newWriter)

	// 获取当前 Level
	level := l.LoggerConfig.Level
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = zapcore.InfoLevel
	}

	// 为新的 writer 创建一个 Core
	encoderLoggerConfig := zap.NewProductionEncoderConfig()
	encoderLoggerConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderLoggerConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	newCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderLoggerConfig),
		newWriter,
		lvl,
	)

	// 将新 Core 追加到扁平切片中
	l.cores = append(l.cores, newCore)
	newCombinedCore := zapcore.NewTee(l.cores...)

	// 更新 logger
	newZapLogger := zap.New(newCombinedCore, zap.AddCaller(), zap.AddCallerSkip(1))
	l.zapLog = newZapLogger.Sugar()
	l.core = newCombinedCore
}

// --- 基础日志方法 ---

func (l *Logger) Debug(args ...any) {
	l.zapLog.Debug(args...)
}

func (l *Logger) Info(args ...any) {
	l.zapLog.Info(args...)
}

func (l *Logger) Warn(args ...any) {
	l.zapLog.Warn(args...)
}

func (l *Logger) Error(args ...any) {
	l.zapLog.Error(args...)
}

func (l *Logger) Fatal(args ...any) {
	l.zapLog.Fatal(args...)
}

// --- 格式化日志方法 ---

func (l *Logger) Debugf(template string, args ...any) {
	l.zapLog.Debugf(template, args...)
}

func (l *Logger) Infof(template string, args ...any) {
	l.zapLog.Infof(template, args...)
}

func (l *Logger) Warnf(template string, args ...any) {
	l.zapLog.Warnf(template, args...)
}

func (l *Logger) Errorf(template string, args ...any) {
	l.zapLog.Errorf(template, args...)
}

// --- 结构化日志方法 ---

func (l *Logger) Infow(msg string, keysAndValues ...any) {
	l.zapLog.Infow(msg, keysAndValues...)
}

func (l *Logger) Debugw(msg string, keysAndValues ...any) {
	l.zapLog.Debugw(msg, keysAndValues...)
}

func (l *Logger) Warnw(msg string, keysAndValues ...any) {
	l.zapLog.Warnw(msg, keysAndValues...)
}

func (l *Logger) Errorw(msg string, keysAndValues ...any) {
	l.zapLog.Errorw(msg, keysAndValues...)
}

// With 返回一个新的 Logger，携带额外的上下文字段 (完美支持 CachedCore)
func (l *Logger) With(keysAndValues ...any) *Logger {
	// 将 any 切片转换为 zap.Field 切片
	fields := make([]zapcore.Field, 0, len(keysAndValues)/2)
	for i := 0; i < len(keysAndValues); i += 2 {
		key := fmt.Sprint(keysAndValues[i])
		var val any
		if i+1 < len(keysAndValues) {
			val = keysAndValues[i+1]
		}
		fields = append(fields, zap.Any(key, val))
	}

	// 获取底层的 zap.Logger 并调用 With
	newZapLogger := l.zapLog.Desugar().With(fields...)

	// 返回一个新的 Logger 实例，共享相同的配置和 cores，但使用新的 sugared logger
	return &Logger{
		zapLog:       newZapLogger.Sugar(),
		LoggerConfig: l.LoggerConfig,
		core:         l.core,
		writers:      l.writers,
		cores:        l.cores,
		cachedCore:   l.cachedCore, // 共享同一个 cachedCore，其内部的 With 也会被正确调用
	}
}

// Sync 刷新缓冲区
func (l *Logger) Sync() error {
	return l.zapLog.Sync()
}

// GetCachedLogs 获取所有缓存的结构化日志
func (l *Logger) GetCachedLogs() []SingleLogRecord {
	if l.cachedCore == nil {
		return nil
	}
	return l.cachedCore.GetLogs()
}

// ClearCachedLogs 清空缓存的日志
func (l *Logger) ClearCachedLogs() {
	if l.cachedCore != nil {
		l.cachedCore.Clear()
	}
}

// GetCachedLogsLen 获取当前缓存的日志数量
func (l *Logger) GetCachedLogsLen() int {
	if l.cachedCore == nil {
		return 0
	}
	return l.cachedCore.Len()
}
