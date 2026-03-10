// Package notify 提供备份任务完成后的通知功能。
//
// 支持 HTTP 通知方式，可配置多个通知渠道同时发送。
// 支持 GET 和 POST 两种 HTTP 方法：
//   - GET：将通知载荷编码为 URL 查询参数
//   - POST：将通知载荷序列化为 JSON 请求体
//
// 特殊支持 chuckfang 渠道类型，将消息拼接到 URL 路径中。
// 使用场景：备份流程结束后，将备份结果汇总通知发送给管理员或监控系统。
package notify

import (
	"context"
	"time"
)

// Notifier 定义通知发送接口。
// 实现该接口即可扩展自定义通知方式。
// 使用场景：备份完成后调用 Notify() 发送汇总结果，
// 当前内置 HTTPNotifier 实现。
type Notifier interface {
	// Notify 发送备份汇总通知。
	// 参数 payload 包含本次备份的完整统计信息。
	// 返回 error 表示所有渠道发送失败的汇总错误。
	Notify(ctx context.Context, payload Payload) error
}

// Payload 是通知载荷结构，包含备份任务的完整汇总信息。
// 可用于单个快照的失败通知（使用 SnapshotsID/DBType/Schema 等字段），
// 也可用于批量备份的汇总通知（使用 TotalDB/SuccessCount/FailedCount 等字段）。
// 使用场景：构建通知内容后传递给 Notifier.Notify() 发送。
type Payload struct {
	SnapshotsID     string         `json:"snapshots_id"`     // 快照任务 ID（单个快照失败通知时使用）
	DBType          string         `json:"db_type"`          // 数据库类型（单个快照失败通知时使用）
	Schema          string         `json:"schema"`           // 数据库名（单个快照失败通知时使用）
	FileName        string         `json:"file_name"`        // 备份文件名（单个快照通知时使用）
	FileHash        string         `json:"file_hash"`        // 文件哈希值（单个快照通知时使用）
	StartedAt       time.Time      `json:"started_at"`       // 备份开始时间
	Duration        float64        `json:"duration_s"`       // 单个备份耗时（秒）
	Status          string         `json:"status"`           // 通知状态（"success" 或 "partial_failed"）
	Message         string         `json:"message"`          // 通知消息摘要（人类可读的汇总文本）
	TotalDB         int            `json:"total_db"`         // 参与备份的数据库总数
	AsyncSnapshots  int            `json:"async_snapshots"`  // 远端存储同步成功总数
	SuccessCount    int            `json:"success_count"`    // 快照成功数
	FailedCount     int            `json:"failed_count"`     // 快照失败数（含同步失败）
	SuccessItems    []string       `json:"success_items"`    // 成功的快照 ID 列表
	FailedItems     []string       `json:"failed_items"`     // 失败详情列表（含快照失败和同步失败）
	SnapshotResults []SnapshotItem `json:"snapshot_results"` // 每个快照任务的详细结果列表
	TotalDurationS  float64        `json:"total_duration_s"` // 整批备份的总耗时（秒）
}

// SnapshotItem 是单个快照任务的执行结果。
// 使用场景：在通知载荷中记录每个数据库快照的独立结果，
// 便于接收方了解哪些快照成功、哪些失败及失败原因。
type SnapshotItem struct {
	SnapshotID    string   `json:"snapshot_id"`              // 快照任务 ID（对应配置中的 snapshots.id）
	Status        string   `json:"status"`                   // 执行状态（"success" 或 "failed"）
	StorageOK     int      `json:"storage_ok"`               // 远端存储同步成功数
	Error         string   `json:"error,omitempty"`          // 快照失败的错误信息（成功时为空）
	FailureDetail []string `json:"failure_detail,omitempty"` // 存储同步失败的详情列表（如某个 S3 上传失败）
}
