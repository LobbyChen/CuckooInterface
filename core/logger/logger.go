package logger

import (
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"io"
	"os"
	"path/filepath"
	"sync"
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
}

// Logger 封装后的日志记录器
type Logger struct {
	mu           sync.RWMutex
	zapLog       *zap.SugaredLogger
	LoggerConfig LoggerConfig
	core         zapcore.Core
	writers      []zapcore.WriteSyncer // 跟踪所有当前的 writer，以便动态添加
	cores        []zapcore.Core        // 扁平维护所有子 Core，避免 AddWriter 产生嵌套 Tee 树
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
	// 1. 解析日志级别
	level := zapcore.InfoLevel
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		fmt.Printf("invalid log level '%s', using info\n", cfg.Level)
		level = zapcore.InfoLevel
	}
	// 2. 确保日志目录存在
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		fmt.Printf("failed to create log directory %s: %v\n", cfg.LogDir, err)
		os.Exit(1)
	}
	// 3. 配置 Lumberjack 进行日志切割
	// 文件名格式: <Date>-<Number>.log 由 lumberjack 自动处理时间戳和序号
	lumberJackLogger := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.LogDir, "app.log"), // 基础文件名，lumberjack 会追加时间戳
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
	}
	// 4. 创建 WriteSyncers
	var writers []zapcore.WriteSyncer
	// 添加文件输出
	fileWriter := zapcore.AddSync(lumberJackLogger)
	writers = append(writers, fileWriter)
	// 添加控制台输出
	if cfg.ConsoleOutput {
		consoleWriter := zapcore.AddSync(os.Stdout)
		writers = append(writers, consoleWriter)
	}
	// 5. 创建编码器配置
	encoderLoggerConfig := zap.NewProductionEncoderConfig()
	encoderLoggerConfig.TimeKey = "time"
	encoderLoggerConfig.EncodeTime = zapcore.ISO8601TimeEncoder // 2026-04-24T10:00:00.000+0800
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
	// 为新的 writer 创建一个 Core，使用 Console 格式以便于人类阅读
	encoderLoggerConfig := zap.NewProductionEncoderConfig()
	encoderLoggerConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderLoggerConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	newCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderLoggerConfig),
		newWriter,
		lvl,
	)
	// 将新 Core 追加到扁平切片中，然后用 NewTee 一次性重建，
	// 避免将旧 Tee 作为子节点嵌套导致递归树过深。
	l.cores = append(l.cores, newCore)
	newCombinedCore := zapcore.NewTee(l.cores...)
	// 更新 logger
	newZapLogger := zap.New(newCombinedCore, zap.AddCaller(), zap.AddCallerSkip(1))
	l.zapLog = newZapLogger.Sugar()
	l.core = newCombinedCore
}

// Debug 级别日志
func (l *Logger) Debug(args ...any) {
	l.zapLog.Debug(args...)
}

// Info 级别日志
func (l *Logger) Info(args ...any) {
	l.zapLog.Info(args...)
}

// Warn 级别日志
func (l *Logger) Warn(args ...any) {
	l.zapLog.Warn(args...)
}

// Error 级别日志
func (l *Logger) Error(args ...any) {
	l.zapLog.Error(args...)
}

// Fatal 级别日志
func (l *Logger) Fatal(args ...any) {
	l.zapLog.Fatal(args...)
}

// Debugf 格式化 Debug 日志
func (l *Logger) Debugf(template string, args ...any) {
	l.zapLog.Debugf(template, args...)
}

// Infof 格式化 Info 日志
func (l *Logger) Infof(template string, args ...any) {
	l.zapLog.Infof(template, args...)
}

// Warnf 格式化 Warn 日志
func (l *Logger) Warnf(template string, args ...any) {
	l.zapLog.Warnf(template, args...)
}

// Errorf 格式化 Error 日志
func (l *Logger) Errorf(template string, args ...any) {
	l.zapLog.Errorf(template, args...)
}

// Infow 结构化日志 (Key-Value pairs)
// 示例: Infow("failed to fetch URL", "url", "example.com", "attempt", 3, "duration", time.Second)
func (l *Logger) Infow(msg string, keysAndValues ...any) {
	l.zapLog.Infow(msg, keysAndValues...)
}

// Debugw 结构化 Debug 日志
func (l *Logger) Debugw(msg string, keysAndValues ...any) {
	l.zapLog.Debugw(msg, keysAndValues...)
}

// Warnw 结构化 Warn 日志
func (l *Logger) Warnw(msg string, keysAndValues ...any) {
	l.zapLog.Warnw(msg, keysAndValues...)
}

// Errorw 结构化 Error 日志
func (l *Logger) Errorw(msg string, keysAndValues ...any) {
	l.zapLog.Errorw(msg, keysAndValues...)
}

// Sync 刷新缓冲区
func (l *Logger) Sync() error {
	return l.zapLog.Sync()
}
