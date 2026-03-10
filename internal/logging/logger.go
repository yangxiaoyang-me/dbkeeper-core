// Package logging 提供 dbkeeper-core 的结构化日志功能。
//
// 日志按级别分文件输出：
//   - info.log：记录 DEBUG 和 INFO 级别日志（正常运行信息）
//   - error.log：记录 WARN 和 ERROR 级别日志（警告和错误信息）
//
// 使用 Go 标准库 log/slog 实现 JSON 格式输出，每条日志自动附带应用名和协程 ID，
// 便于在并发场景下追踪日志来源。
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Logger 是结构化日志记录器，封装 slog.Logger 并管理日志文件生命周期。
// 使用场景：应用启动时创建，贯穿整个备份流程记录运行日志。
// 日志输出为 JSON 格式，自动附带 app 名称和 goroutine ID。
type Logger struct {
	base *slog.Logger
	// infoFile 和 errorFile 保留文件句柄，用于应用退出时关闭。
	infoFile  *os.File
	errorFile *os.File
}

// New 创建日志记录器并初始化日志文件。
// 主要逻辑：创建日志目录、打开 info.log 和 error.log 两个文件、
// 配置级别范围处理器实现日志分流。
// 使用场景：应用启动时调用，传入日志目录路径（如 ./logs）。
func New(logDir string) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir failed: %w", err)
	}

	infoPath := filepath.Join(logDir, "info.log")
	errorPath := filepath.Join(logDir, "error.log")

	infoFile, err := os.OpenFile(infoPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open info.log failed: %w", err)
	}
	errorFile, err := os.OpenFile(errorPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		_ = infoFile.Close()
		return nil, fmt.Errorf("open error.log failed: %w", err)
	}

	infoHandler := newLevelRangeHandler(
		slog.NewJSONHandler(infoFile, &slog.HandlerOptions{Level: slog.LevelDebug}),
		slog.LevelDebug,
		slog.LevelInfo,
	)
	errorHandler := newLevelRangeHandler(
		slog.NewJSONHandler(errorFile, &slog.HandlerOptions{Level: slog.LevelWarn}),
		slog.LevelWarn,
		slog.Level(math.MaxInt32),
	)

	base := slog.New(newMultiHandler(infoHandler, errorHandler)).With("app", "dbkeeper-core")
	return &Logger{
		base:      base,
		infoFile:  infoFile,
		errorFile: errorFile,
	}, nil
}

// Close 关闭日志文件句柄，释放资源。
// 使用场景：应用退出时调用（通常在 defer 中），确保日志数据完整写入磁盘。
func (l *Logger) Close() error {
	var errs []string
	if l.infoFile != nil {
		if err := l.infoFile.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if l.errorFile != nil {
		if err := l.errorFile.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close logger files failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Debug 输出 DEBUG 级别日志，自动附带 goroutine ID。
// 使用场景：记录详细的调试信息，仅写入 info.log。
func (l *Logger) Debug(msg string, args ...any) {
	l.base.Debug(prefix(msg), append([]any{"goroutine", goroutineID()}, args...)...)
}

// Info 输出 INFO 级别日志，自动附带 goroutine ID。
// 使用场景：记录正常运行信息（如备份开始、完成），写入 info.log。
func (l *Logger) Info(msg string, args ...any) {
	l.base.Info(prefix(msg), append([]any{"goroutine", goroutineID()}, args...)...)
}

// Warn 输出 WARN 级别日志，自动附带 goroutine ID。
// 使用场景：记录非致命性警告（如上传失败但不影响整体流程），写入 error.log。
func (l *Logger) Warn(msg string, args ...any) {
	l.base.Warn(prefix(msg), append([]any{"goroutine", goroutineID()}, args...)...)
}

// Error 输出 ERROR 级别日志，自动附带 goroutine ID。
// 使用场景：记录严重错误（如备份失败、组件初始化失败），写入 error.log。
func (l *Logger) Error(msg string, args ...any) {
	l.base.Error(prefix(msg), append([]any{"goroutine", goroutineID()}, args...)...)
}

// prefix 为日志消息添加统一前缀 "[dbkeeper-core] "。
// 使用场景：标识日志来源，便于在多服务混合日志中快速过滤。
func prefix(msg string) string {
	return "[dbkeeper-core] " + msg
}

// goroutineID 获取当前 goroutine 的 ID。
// 主要逻辑：从 runtime.Stack 输出中解析 goroutine ID 数字。
// 使用场景：并发备份时区分不同任务的日志，便于问题排查。
func goroutineID() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	line := strings.TrimPrefix(string(buf[:n]), "goroutine ")
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return -1
	}
	id, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return -1
	}
	return id
}

// levelRangeHandler 是按日志级别范围过滤的 slog.Handler 实现。
// 只处理 [min, max] 范围内的日志级别，超出范围的日志会被忽略。
// 使用场景：实现日志分流，将不同级别的日志写入不同文件。
// 例如：将 DEBUG~INFO 写入 info.log，将 WARN~ERROR 写入 error.log。
type levelRangeHandler struct {
	base slog.Handler // 底层实际处理器（如 slog.JSONHandler）
	min  slog.Level   // 允许的最小日志级别（含）
	max  slog.Level   // 允许的最大日志级别（含）
}

// newLevelRangeHandler 创建级别范围过滤处理器。
// 参数 base 为底层处理器，min/max 为允许的日志级别范围。
func newLevelRangeHandler(base slog.Handler, min, max slog.Level) slog.Handler {
	return &levelRangeHandler{base: base, min: min, max: max}
}

// Enabled 判断指定级别是否在允许范围内。
func (h *levelRangeHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return lvl >= h.min && lvl <= h.max && h.base.Enabled(ctx, lvl)
}

// Handle 处理日志记录，超出级别范围的记录会被静默丢弃。
func (h *levelRangeHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level < h.min || r.Level > h.max {
		return nil
	}
	return h.base.Handle(ctx, r)
}

// WithAttrs 返回附加属性后的新处理器，保持级别范围不变。
func (h *levelRangeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &levelRangeHandler{
		base: h.base.WithAttrs(attrs),
		min:  h.min,
		max:  h.max,
	}
}

// WithGroup 返回附加分组后的新处理器，保持级别范围不变。
func (h *levelRangeHandler) WithGroup(name string) slog.Handler {
	return &levelRangeHandler{
		base: h.base.WithGroup(name),
		min:  h.min,
		max:  h.max,
	}
}

// multiHandler 是多路日志处理器，将日志同时分发到多个子处理器。
// 使用场景：实现同一条日志同时写入 info.log 和 error.log（由各自的级别过滤器决定是否实际写入）。
type multiHandler struct {
	handlers []slog.Handler // 子处理器列表
}

// newMultiHandler 创建多路处理器。
func newMultiHandler(handlers ...slog.Handler) slog.Handler {
	return &multiHandler{handlers: handlers}
}

// Enabled 只要有任意一个子处理器启用了该级别，就返回 true。
func (h *multiHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	for _, item := range h.handlers {
		if item.Enabled(ctx, lvl) {
			return true
		}
	}
	return false
}

// Handle 将日志记录分发到所有启用该级别的子处理器，返回第一个遇到的错误。
func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, item := range h.handlers {
		if !item.Enabled(ctx, r.Level) {
			continue
		}
		if err := item.Handle(ctx, r); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// WithAttrs 为所有子处理器附加属性，返回新的多路处理器。
func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Handler, 0, len(h.handlers))
	for _, item := range h.handlers {
		out = append(out, item.WithAttrs(attrs))
	}
	return &multiHandler{handlers: out}
}

// WithGroup 为所有子处理器附加分组，返回新的多路处理器。
func (h *multiHandler) WithGroup(name string) slog.Handler {
	out := make([]slog.Handler, 0, len(h.handlers))
	for _, item := range h.handlers {
		out = append(out, item.WithGroup(name))
	}
	return &multiHandler{handlers: out}
}
