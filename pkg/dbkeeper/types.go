// Package dbkeeper 是 dbkeeper-core 的 SDK 公共包，提供对外 API。
//
// 外部项目可以通过此包将 dbkeeper-core 作为库嵌入使用，无需作为独立进程运行。
// 主要功能：
//   - 类型别名：将内部类型重新导出，避免外部直接依赖 internal 包
//   - Runtime：提供简单的初始化和执行接口
//   - 依赖注入：通过 Dependencies 支持替换默认组件
//   - 扩展注册：通过 NewSnapshotsRegistry/NewStorageRegistry 注册自定义适配器
//
// 快速开始示例：
//
//	cfg, _ := dbkeeper.LoadConfig("config.yaml")
//	rt, _ := dbkeeper.NewRuntime(cfg, nil)  // nil 使用全部默认组件
//	defer rt.Close()
//	rt.Run(context.Background())
package dbkeeper

import (
	"dbkeeper-core/internal/application"
	"dbkeeper-core/internal/config"
	"dbkeeper-core/internal/infrastructure/notify"
	"dbkeeper-core/internal/infrastructure/snapshots"
	"dbkeeper-core/internal/infrastructure/storages"
	"dbkeeper-core/internal/metrics"
)

// ── 配置类型别名 ──────────────────────────────────────────────────────────

// Config 是 SDK 侧总配置对象，对应 config.yaml 的根结构。
// 使用场景：通过 LoadConfig() 加载配置文件后传递给 NewRuntime()。
type Config = config.Config

// Application 是 application 节点配置，控制并发、工作目录、日志、元数据库和快照列表。
// 使用场景：编程方式构建配置时使用。
type Application = config.Application

// LogConfig 是日志配置，通常只需设置日志目录。
type LogConfig = config.LogConfig

// Database 是元数据库配置（当前默认使用 SQLite）。
type Database = config.Database

// Notify 是通知配置，可配置全局类型和多渠道。
type Notify = config.Notify

// NotifyChannel 是单个通知渠道配置。
type NotifyChannel = config.NotifyChannel

// SnapshotsSpec 是单个快照任务配置（每个数据库实例对应一条）。
// 使用场景：定义要备份的数据库连接信息和存储目标。
type SnapshotsSpec = config.SnapshotsSpec

// StorageSpec 是单个存储配置，定义本地/远端存储目标及保留策略。
type StorageSpec = config.StorageSpec

// ── 接口类型别名 ──────────────────────────────────────────────────────────

// SnapshotsRepository 是快照元数据持久化接口。
// 使用场景：外部项目可实现此接口替换默认的 SQLite 存储。
type SnapshotsRepository = application.SnapshotsRepository

// IDGenerator 是 ID 生成器接口。
// 使用场景：外部项目可实现此接口替换默认的雪花算法。
type IDGenerator = application.IDGenerator

// Logger 是日志输出接口。
// 使用场景：外部项目可实现此接口替换默认的文件日志。
type Logger = application.Logger

// SnapshotsRegistry 是快照适配器注册表接口。
// 使用场景：根据 db_type 选择快照导出适配器。
type SnapshotsRegistry = application.SnapshotsRegistry

// StorageRegistry 是存储适配器注册表接口。
// 使用场景：根据 storage.type 选择存储适配器。
type StorageRegistry = application.StorageRegistry

// RetentionPolicy 是保留策略接口。
// 使用场景：执行本地与远端文件保留策略，清理过期备份。
type RetentionPolicy = application.RetentionPolicy

// Notifier 是通知发送接口。
// 使用场景：备份完成后发送汇总通知。
type Notifier = notify.Notifier

// NotifyPayload 是通知载荷结构。
type NotifyPayload = notify.Payload

// NotifySnapshotItem 是单个快照结果结构。
type NotifySnapshotItem = notify.SnapshotItem

// ── 适配器扩展点 ──────────────────────────────────────────────────────────

// SnapshotAdapter 是数据库快照适配器接口（扩展点）。
// 使用场景：外部项目实现此接口并通过 NewSnapshotsRegistry 注册，支持新的数据库类型。
type SnapshotAdapter = snapshots.Adapter

// StorageAdapter 是存储上传与清理适配器接口（扩展点）。
// 使用场景：外部项目实现此接口并通过 NewStorageRegistry 注册，支持新的存储类型。
type StorageAdapter = storages.Adapter

// ── 指标类型 ──────────────────────────────────────────────────────────────

// Metrics 是备份任务运行时指标收集器。
type Metrics = metrics.Metrics

// MetricsSnapshot 是指标的只读快照，通过 GetMetrics() 获取。
type MetricsSnapshot = metrics.MetricsSnapshot
