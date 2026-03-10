package dbkeeper

import (
	"context"
	"errors"

	"dbkeeper-core/internal/application"
	"dbkeeper-core/internal/config"
	"dbkeeper-core/internal/id"
	"dbkeeper-core/internal/infrastructure/notify"
	"dbkeeper-core/internal/infrastructure/persistence"
	"dbkeeper-core/internal/infrastructure/retention"
	"dbkeeper-core/internal/infrastructure/snapshots"
	"dbkeeper-core/internal/infrastructure/storages"
	"dbkeeper-core/internal/logging"
	"dbkeeper-core/internal/metrics"
)

// Dependencies 定义 Runtime 的可替换依赖组件。
// 字段为 nil 时使用内置默认实现（SQLite、文件日志、雪花ID等）；
// 非 nil 时使用调用方注入的自定义实现。
// 使用场景：外部项目需要自定义日志输出、元数据存储或通知方式时，
// 通过此结构体注入自定义实现。
type Dependencies struct {
	// Logger 用于输出运行日志。为 nil 时使用默认文件日志（info.log + error.log）。
	Logger Logger
	// Repository 用于存储快照元数据。为 nil 时使用默认 SQLite 存储。
	Repository SnapshotsRepository
	// IDGenerator 用于生成元数据主键。为 nil 时使用默认雪花算法。
	IDGenerator IDGenerator
	// Notifier 用于发送任务汇总通知。为 nil 时根据配置自动创建 HTTP 通知器。
	Notifier Notifier
	// RetentionPolicy 用于执行本地/远端保留策略。为 nil 时使用默认保留策略管理器。
	RetentionPolicy RetentionPolicy
	// SnapshotsRegistry 用于选择数据库快照实现。为 nil 时注册默认的 MySQL/DM/PG 适配器。
	SnapshotsRegistry SnapshotsRegistry
	// StorageRegistry 用于选择存储上传实现。为 nil 时注册默认的 Host/S3/WebDAV 适配器。
	StorageRegistry StorageRegistry
}

// Runtime 是 SDK 对外运行时入口，封装完整的备份执行能力。
// 可在任意 Go 项目中作为库调用，无需作为独立进程运行。
// 使用场景：外部项目通过 NewRuntime() 创建实例，调用 Run() 执行备份，
// 最后调用 Close() 释放资源。
type Runtime struct {
	service *application.SnapshotsService // 核心业务服务
	closers []func() error                // 资源清理函数列表（如日志文件句柄）
}

// LoadConfig 从 YAML 文件读取并校验配置。
// 使用场景：外部项目加载配置文件时调用，等同于 config.Load()。
func LoadConfig(path string) (*Config, error) {
	return config.Load(path)
}

// NewRuntime 构建可执行的备份运行时。
// 主要逻辑：优先使用 deps 中注入的组件，缺失部分自动补充默认实现。
// 默认组件包括：文件日志、SQLite 仓储、雪花 ID、HTTP 通知器、
// MySQL/DM/PG 快照适配器、Host/S3/WebDAV 存储适配器。
// 使用场景：外部项目初始化备份功能时调用。
func NewRuntime(cfg *Config, deps *Dependencies) (*Runtime, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	var d Dependencies
	if deps != nil {
		d = *deps
	}

	closers := make([]func() error, 0, 1)

	if d.Logger == nil {
		lg, err := logging.New(cfg.Application.Log.Dir)
		if err != nil {
			return nil, err
		}
		d.Logger = lg
		closers = append(closers, lg.Close)
	}
	if d.Repository == nil {
		repo, err := persistence.NewSQLiteRepository(cfg.Application.Database)
		if err != nil {
			return nil, err
		}
		d.Repository = repo
	}
	if d.IDGenerator == nil {
		idGen, err := id.New(1, 1)
		if err != nil {
			return nil, err
		}
		d.IDGenerator = idGen
	}
	if d.Notifier == nil {
		d.Notifier = notify.NewNotifier(cfg.Application.Notify)
	}
	if d.RetentionPolicy == nil {
		d.RetentionPolicy = retention.New()
	}
	if d.SnapshotsRegistry == nil {
		d.SnapshotsRegistry = snapshots.NewRegistry(
			&snapshots.MySQLAdapter{},
			&snapshots.DMAdapter{},
			&snapshots.PGAdapter{},
		)
	}
	if d.StorageRegistry == nil {
		d.StorageRegistry = storages.NewRegistry(
			&storages.HostAdapter{},
			&storages.S3Adapter{},
			&storages.WebDAVAdapter{},
		)
	}

	svc := application.NewSnapshotsService(
		cfg,
		d.Logger,
		d.Repository,
		d.IDGenerator,
		d.Notifier,
		d.RetentionPolicy,
		d.SnapshotsRegistry,
		d.StorageRegistry,
	)
	return &Runtime{service: svc, closers: closers}, nil
}

// Run 执行一次完整的数据库备份流程。
// 包括：并发执行所有快照任务、上传到远端存储、应用保留策略、发送通知。
// 使用场景：外部项目触发备份时调用（可由定时任务或手动触发）。
func (r *Runtime) Run(ctx context.Context) error {
	if r == nil || r.service == nil {
		return errors.New("runtime is not initialized")
	}
	return r.service.Run(ctx)
}

// Close 关闭 Runtime 持有的内部资源。
// 主要逻辑：依次调用所有资源清理函数（如关闭默认 Logger 的文件句柄）。
// 使用场景：备份完成后调用（通常在 defer 中），确保资源被正确释放。
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	var firstErr error
	for _, closer := range r.closers {
		if err := closer(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// GetMetrics 获取全局备份任务指标的只读快照。
// 返回的 MetricsSnapshot 包含总运行次数、成功/失败次数、上传次数等统计信息。
// 使用场景：外部项目需要监控备份状态时调用（如健康检查接口）。
func GetMetrics() MetricsSnapshot {
	return metrics.GetGlobal().Snapshot()
}
