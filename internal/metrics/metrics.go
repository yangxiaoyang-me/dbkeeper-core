// Package metrics 提供备份任务的运行时指标统计功能。
//
// 使用全局单例模式，通过 GetGlobal() 获取指标实例。
// 所有操作均为线程安全，支持并发备份场景下的指标累计。
// 使用场景：监控备份任务的成功率、失败率、耗时等关键指标，
// 外部可通过 pkg/dbkeeper.GetMetrics() 获取指标快照。
package metrics

import (
	"sync"
	"time"
)

// Metrics 是备份任务的运行时指标收集器（线程安全）。
// 记录整体运行次数、快照次数、存储上传次数等核心指标。
// 使用场景：在备份流程的关键节点调用 Record 方法累计指标，
// 外部调用方通过 Snapshot() 获取只读快照。
type Metrics struct {
	mu                   sync.RWMutex // 读写锁，保证并发安全
	TotalRuns            int64        // 总运行次数（每次调用 Run() 计一次）
	SuccessfulRuns       int64        // 成功运行次数（所有快照和存储同步均成功）
	FailedRuns           int64        // 失败运行次数（任一快照或同步失败即算失败）
	TotalSnapshots       int64        // 总快照次数（每个数据库快照任务计一次）
	SuccessfulSnapshots  int64        // 成功快照次数
	FailedSnapshots      int64        // 失败快照次数
	TotalStorageUploads  int64        // 总存储上传次数（每次上传到远端存储计一次）
	FailedStorageUploads int64        // 失败存储上传次数
	LastRunTime          time.Time    // 最近一次运行的开始时间
	LastRunDurationS     float64      // 最近一次运行的总耗时（秒）
	LastRunStatus        string       // 最近一次运行的状态（"success" 或 "failed"）
}

// globalMetrics 是全局指标单例，应用生命周期内始终使用同一个实例。
var globalMetrics = &Metrics{}

// GetGlobal 获取全局指标实例。
// 使用场景：在备份流程中调用，记录各阶段的指标数据。
func GetGlobal() *Metrics {
	return globalMetrics
}

// RecordRunStart 记录一次备份运行开始。
// 主要逻辑：递增总运行次数并记录开始时间。
// 使用场景：在 SnapshotsService.Run() 开始时调用。
func (m *Metrics) RecordRunStart() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalRuns++
	m.LastRunTime = time.Now()
}

// RecordRunEnd 记录一次备份运行结束。
// 参数 success 表示本次运行是否全部成功，durationS 为总耗时（秒）。
// 使用场景：在 SnapshotsService.Run() 结束时调用。
func (m *Metrics) RecordRunEnd(success bool, durationS float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if success {
		m.SuccessfulRuns++
		m.LastRunStatus = "success"
	} else {
		m.FailedRuns++
		m.LastRunStatus = "failed"
	}
	m.LastRunDurationS = durationS
}

// RecordSnapshot 记录单个数据库快照任务的执行结果。
// 参数 success 表示快照是否成功（导出+压缩+哈希全部完成）。
// 使用场景：每个快照任务执行完毕后调用。
func (m *Metrics) RecordSnapshot(success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalSnapshots++
	if success {
		m.SuccessfulSnapshots++
	} else {
		m.FailedSnapshots++
	}
}

// RecordStorageUpload 记录单次远端存储上传的执行结果。
// 参数 success 表示上传是否成功。
// 使用场景：每次上传到远端存储（SFTP/S3/WebDAV）后调用。
func (m *Metrics) RecordStorageUpload(success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TotalStorageUploads++
	if !success {
		m.FailedStorageUploads++
	}
}

// Snapshot 获取当前指标的只读快照（线程安全）。
// 返回 MetricsSnapshot 结构体，是当前指标的深拷贝。
// 使用场景：外部调用方读取指标时使用，不会与写入操作产生竞争。
func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return MetricsSnapshot{
		TotalRuns:            m.TotalRuns,
		SuccessfulRuns:       m.SuccessfulRuns,
		FailedRuns:           m.FailedRuns,
		TotalSnapshots:       m.TotalSnapshots,
		SuccessfulSnapshots:  m.SuccessfulSnapshots,
		FailedSnapshots:      m.FailedSnapshots,
		TotalStorageUploads:  m.TotalStorageUploads,
		FailedStorageUploads: m.FailedStorageUploads,
		LastRunTime:          m.LastRunTime,
		LastRunDurationS:     m.LastRunDurationS,
		LastRunStatus:        m.LastRunStatus,
	}
}

// MetricsSnapshot 是指标的只读快照结构，是 Metrics 某一时刻的深拷贝。
// 使用场景：通过 Metrics.Snapshot() 获取，用于外部读取指标数据而不持有锁。
type MetricsSnapshot struct {
	TotalRuns            int64     // 总运行次数
	SuccessfulRuns       int64     // 成功运行次数
	FailedRuns           int64     // 失败运行次数
	TotalSnapshots       int64     // 总快照次数
	SuccessfulSnapshots  int64     // 成功快照次数
	FailedSnapshots      int64     // 失败快照次数
	TotalStorageUploads  int64     // 总存储上传次数
	FailedStorageUploads int64     // 失败存储上传次数
	LastRunTime          time.Time // 最近运行开始时间
	LastRunDurationS     float64   // 最近运行耗时（秒）
	LastRunStatus        string    // 最近运行状态
}
