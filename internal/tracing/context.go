// Package tracing 提供分布式追踪上下文管理功能。
//
// 通过 context.Context 传递 trace ID，实现跨函数调用的日志关联。
// 每个快照任务在开始时生成唯一的 trace ID，后续所有日志都会携带该 ID，
// 便于在并发场景下追踪单个任务的完整执行链路。
package tracing

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// contextKey 是 context 键的类型，避免与其他包的键冲突。
type contextKey string

// traceIDKey 是 trace ID 在 context 中的键名。
const traceIDKey contextKey = "trace_id"

// WithTraceID 在 context 中注入 trace ID，返回新的 context。
// 使用场景：快照任务开始时调用，将 trace ID 注入 context 供下游日志使用。
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// GetTraceID 从 context 中提取 trace ID。
// 如果 context 中没有 trace ID，返回空字符串。
// 使用场景：记录日志时调用，自动关联当前任务的 trace ID。
func GetTraceID(ctx context.Context) string {
	if traceID, ok := ctx.Value(traceIDKey).(string); ok {
		return traceID
	}
	return ""
}

// GenerateTraceID 生成新的 trace ID。
// 格式：{unix时间戳}-{8位十六进制随机数}，如 "1704067200-a1b2c3d4"。
// 使用场景：每个快照任务开始时调用，生成该任务的唯一追踪标识。
func GenerateTraceID() string {
	rand.Seed(time.Now().UnixNano())
	return fmt.Sprintf("%d-%08x", time.Now().Unix(), rand.Uint32())
}
