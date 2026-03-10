// Package application 是 dbkeeper-core 的核心业务逻辑层。
//
// 定义了备份流程的核心接口和服务实现，协调以下功能模块：
//   - 数据库快照导出（通过 SnapshotsRegistry 适配器）
//   - 远端存储上传（通过 StorageRegistry 适配器）
//   - 元数据持久化（通过 SnapshotsRepository）
//   - 保留策略执行（通过 RetentionPolicy）
//   - 通知发送（通过 Notifier）
//
// 采用依赖注入模式，所有外部依赖通过接口传入，便于测试和扩展。
package application

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dbkeeper-core/internal/config"
	"dbkeeper-core/internal/domain"
	"dbkeeper-core/internal/infrastructure/notify"
	"dbkeeper-core/internal/infrastructure/retry"
	"dbkeeper-core/internal/infrastructure/snapshots"
	storagepkg "dbkeeper-core/internal/infrastructure/storages"
	"dbkeeper-core/internal/metrics"
	"dbkeeper-core/internal/tracing"
)

// SnapshotsRepository 定义快照元数据持久化接口。
// 负责初始化数据库结构和插入备份记录。
// 使用场景：存储本地和远程备份的元数据信息，支持历史查询和审计。
// 默认实现：internal/infrastructure/persistence.SQLiteRepository
type SnapshotsRepository interface {
	// InitSchema 初始化数据库表结构（幂等操作，可重复调用）。
	InitSchema(ctx context.Context) error
	// InsertSnapshotsAttachment 插入本地备份元数据记录。
	InsertSnapshotsAttachment(ctx context.Context, att domain.SnapshotAttachment) error
	// InsertStorageSnapshotsAttachment 插入远端存储备份元数据记录。
	InsertStorageSnapshotsAttachment(ctx context.Context, att domain.StorageSnapshotAttachment) error
}

// IDGenerator 定义 ID 生成器接口。
// 使用场景：为本地备份记录和远端存储记录生成全局唯一的主键 ID。
// 默认实现：internal/id.Snowflake（雪花算法）
type IDGenerator interface {
	// NextIDString 生成下一个唯一 ID 并返回字符串形式。
	NextIDString() string
}

// Logger 定义结构化日志接口。
// 使用场景：在备份流程各阶段输出运行日志，支持 key-value 格式。
// 默认实现：internal/logging.Logger（JSON 格式输出到 info.log 和 error.log）
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// SnapshotsRegistry 定义快照适配器注册表接口。
// 使用场景：根据配置中的 db_type 字段（如 mysql/dm/pg）选择对应的数据库备份实现。
// 默认实现：internal/infrastructure/snapshots.Registry
type SnapshotsRegistry interface {
	// Get 根据数据库类型获取对应的备份适配器。
	Get(dbType string) (snapshots.Adapter, error)
}

// StorageRegistry 定义存储适配器注册表接口。
// 使用场景：根据配置中的 storage.type 字段（如 host/s3/webdav）选择对应的上传实现。
// 默认实现：internal/infrastructure/storages.Registry
type StorageRegistry interface {
	// Get 根据存储类型获取对应的存储适配器。
	Get(t string) (storagepkg.Adapter, error)
}

// RetentionPolicy 定义备份文件保留策略接口。
// 使用场景：备份和上传完成后，清理本地和远程的过期备份文件，防止存储空间耗尽。
// 默认实现：internal/infrastructure/retention.Manager
type RetentionPolicy interface {
	// ApplyLocal 执行本地保留策略，保留最新的 retentionCount 个文件。
	ApplyLocal(workPath string, retentionCount int, snapshotsSpec config.SnapshotsSpec) error
	// ApplyStorage 执行远端保留策略，通过存储适配器删除过期文件。
	ApplyStorage(ctx context.Context, spec config.StorageSpec, adapter storagepkg.Adapter, snapshotsSpec config.SnapshotsSpec) error
}

// SnapshotsService 是快照服务核心实现，协调整个备份流程。
// 主要职责：
//   - 并发执行多个数据库的快照任务
//   - 将备份文件上传到配置的存储目标
//   - 记录本地和远端的备份元数据
//   - 执行保留策略清理过期文件
//   - 发送备份结果汇总通知
//
// 使用场景：作为核心服务被 main.go 或 pkg/dbkeeper.Runtime 调用。
type SnapshotsService struct {
	cfg           *config.Config      // 应用配置
	logger        Logger              // 日志记录器
	repo          SnapshotsRepository // 元数据仓储
	idGen         IDGenerator         // ID 生成器
	notifier      notify.Notifier     // 通知发送器（可为 nil 表示禁用通知）
	retention     RetentionPolicy     // 保留策略管理器
	snapshotsRegs SnapshotsRegistry   // 快照适配器注册表
	storageRegs   StorageRegistry     // 存储适配器注册表
}

// NewSnapshotsService 创建快照服务实例。
// 通过依赖注入接收所有外部组件，实现松耦合。
// 使用场景：应用启动时初始化，传入配置和各组件实例。
func NewSnapshotsService(
	cfg *config.Config,
	logger Logger,
	repo SnapshotsRepository,
	idGen IDGenerator,
	notifier notify.Notifier,
	retention RetentionPolicy,
	snapshotsRegs SnapshotsRegistry,
	storageRegs StorageRegistry,
) *SnapshotsService {
	return &SnapshotsService{
		cfg:           cfg,
		logger:        logger,
		repo:          repo,
		idGen:         idGen,
		notifier:      notifier,
		retention:     retention,
		snapshotsRegs: snapshotsRegs,
		storageRegs:   storageRegs,
	}
}

// Run 执行一次完整的数据库备份流程（核心入口方法）。
// 主要流程：
//  1. 初始化元数据库表结构
//  2. 启动 worker goroutine 池（数量由 concurrency 配置决定）
//  3. 将所有快照任务分发到 worker 并发执行
//  4. 每个任务：数据库导出 → 压缩 → 哈希 → 上传 → 保留策略
//  5. 收集所有任务结果，发送汇总通知
//  6. 记录运行指标
//
// 使用场景：定时任务（cron）或手动触发一次备份操作。
func (s *SnapshotsService) Run(ctx context.Context) error {
	batchStart := time.Now()
	m := metrics.GetGlobal()
	m.RecordRunStart()

	s.logger.Info("run started",
		"snapshots", len(s.cfg.Application.Snapshots),
		"concurrency", s.cfg.Application.Concurrency,
		"workspace", s.cfg.Application.WorkPath,
	)
	if err := s.repo.InitSchema(ctx); err != nil {
		return err
	}
	s.logger.Info("metadata schema initialized")

	tasks := make(chan config.SnapshotsSpec)
	var wg sync.WaitGroup
	var mu sync.Mutex

	totalDB := len(s.cfg.Application.Snapshots)
	snapshotsSuccessCount := 0
	snapshotsFailedCount := 0
	asyncSnapshots := 0
	successItems := make([]string, 0, totalDB)
	snapshotsFailedItems := make([]string, 0)
	syncFailedCount := 0
	syncFailedItems := make([]string, 0)
	snapshotResults := make([]notify.SnapshotItem, 0, totalDB)

	worker := func() {
		defer wg.Done()
		for spec := range tasks {
			traceID := tracing.GenerateTraceID()
			taskCtx := tracing.WithTraceID(ctx, traceID)

			if s.cfg.Application.TaskTimeoutS > 0 {
				var cancel context.CancelFunc
				taskCtx, cancel = context.WithTimeout(taskCtx, time.Duration(s.cfg.Application.TaskTimeoutS)*time.Second)
				defer cancel()
			}

			result, err := s.handleOne(taskCtx, spec)

			mu.Lock()
			asyncSnapshots += result.StorageSuccessCount
			if err != nil {
				snapshotsFailedCount++
				m.RecordSnapshot(false)
				snapshotsFailedItems = append(snapshotsFailedItems, fmt.Sprintf("snapshot failed [%s]: %v", spec.ID, err))
				snapshotResults = append(snapshotResults, notify.SnapshotItem{
					SnapshotID:    spec.ID,
					Status:        "failed",
					StorageOK:     result.StorageSuccessCount,
					Error:         err.Error(),
					FailureDetail: append([]string(nil), result.FailedDetails...),
				})
			} else {
				snapshotsSuccessCount++
				m.RecordSnapshot(true)
				successItems = append(successItems, spec.ID)
				snapshotResults = append(snapshotResults, notify.SnapshotItem{
					SnapshotID:    spec.ID,
					Status:        "success",
					StorageOK:     result.StorageSuccessCount,
					FailureDetail: append([]string(nil), result.FailedDetails...),
				})
			}
			if len(result.FailedDetails) > 0 {
				syncFailedCount += len(result.FailedDetails)
				for _, detail := range result.FailedDetails {
					syncFailedItems = append(syncFailedItems, fmt.Sprintf("sync failed [%s]: %s", spec.ID, detail))
				}
			}
			mu.Unlock()

			if err != nil {
				s.logger.Error("snapshot failed", "snapshot_id", spec.ID, "err", err)
			} else {
				s.logger.Info("snapshot success", "snapshot_id", spec.ID, "storage_success", result.StorageSuccessCount)
			}
		}
	}

	for i := 0; i < s.cfg.Application.Concurrency; i++ {
		wg.Add(1)
		go worker()
	}

	for _, spec := range s.cfg.Application.Snapshots {
		tasks <- spec
	}
	close(tasks)
	wg.Wait()

	if s.notifier != nil {
		s.sendNotification(ctx, batchStart, totalDB, asyncSnapshots, snapshotsSuccessCount, snapshotsFailedCount, syncFailedCount, successItems, snapshotsFailedItems, syncFailedItems, snapshotResults)
	}

	durationS := time.Since(batchStart).Seconds()
	runSuccess := snapshotsFailedCount == 0 && syncFailedCount == 0
	m.RecordRunEnd(runSuccess, durationS)

	s.logger.Info("run finished",
		"duration_s", durationS,
		"success_count", snapshotsSuccessCount,
		"failed_count", snapshotsFailedCount,
		"sync_failed_count", syncFailedCount,
	)

	return nil
}

// sendNotification 发送备份汇总通知（带重试）。
// 主要逻辑：构建通知载荷（成功/失败统计），通过重试机制发送。
// 使用场景：所有快照任务完成后调用，通知管理员备份结果。
func (s *SnapshotsService) sendNotification(ctx context.Context, batchStart time.Time, totalDB, asyncSnapshots, snapshotsSuccessCount, snapshotsFailedCount, syncFailedCount int, successItems, snapshotsFailedItems, syncFailedItems []string, snapshotResults []notify.SnapshotItem) {
	totalDurationS := time.Since(batchStart).Seconds()
	status := "success"
	if snapshotsFailedCount > 0 || syncFailedCount > 0 {
		status = "partial_failed"
	}

	lines := make([]string, 0, 1+len(snapshotsFailedItems)+len(syncFailedItems))
	lines = append(lines, fmt.Sprintf("success=%d, failed=%d, sync_failed=%d", snapshotsSuccessCount, snapshotsFailedCount, syncFailedCount))
	if len(successItems) > 0 {
		lines = append(lines, "success_ids="+strings.Join(successItems, ","))
	}
	lines = append(lines, snapshotsFailedItems...)
	lines = append(lines, syncFailedItems...)
	message := strings.Join(lines, "; ")

	payload := notify.Payload{
		Status:          status,
		Message:         message,
		TotalDB:         totalDB,
		AsyncSnapshots:  asyncSnapshots,
		SuccessCount:    snapshotsSuccessCount,
		FailedCount:     snapshotsFailedCount + syncFailedCount,
		SuccessItems:    append([]string(nil), successItems...),
		FailedItems:     append(snapshotsFailedItems, syncFailedItems...),
		SnapshotResults: append([]notify.SnapshotItem(nil), snapshotResults...),
		TotalDurationS:  totalDurationS,
	}

	retryCfg := s.getRetryConfig()
	err := retry.Do(ctx, retryCfg, func() error {
		return s.notifier.Notify(ctx, payload)
	})
	if err != nil {
		s.logger.Error("notify failed after retries", "err", err, "attempts", retryCfg.MaxAttempts)
	} else {
		s.logger.Info("notification sent successfully")
	}
}

// getRetryConfig 获取重试配置。
// 主要逻辑：优先使用配置文件中的重试参数，未配置时使用默认值。
// 使用场景：远端存储上传和通知发送前获取重试策略。
func (s *SnapshotsService) getRetryConfig() retry.Config {
	if s.cfg.Application.Retry.MaxAttempts > 0 {
		return retry.Config{
			MaxAttempts:  s.cfg.Application.Retry.MaxAttempts,
			InitialDelay: time.Duration(s.cfg.Application.Retry.InitialDelayMS) * time.Millisecond,
			MaxDelay:     time.Duration(s.cfg.Application.Retry.MaxDelayMS) * time.Millisecond,
		}
	}
	return retry.DefaultConfig()
}

// handleOne 处理单个快照任务的完整生命周期。
// 主要流程：
//  1. 生成 trace ID 并注入 context（用于日志追踪）
//  2. 设置任务超时（如果配置了 task_timeout_s）
//  3. 执行数据库备份导出（executeSnapshot）
//  4. 同步到各存储位置（syncToStorages）
//  5. 清理工作目录中的过期文件（cleanupWorkspace）
//
// 使用场景：worker goroutine 中调用，每个并发任务处理一个数据库的备份。
func (s *SnapshotsService) handleOne(ctx context.Context, spec config.SnapshotsSpec) (handleResult, error) {
	start := time.Now()
	result := handleResult{}
	snapshotWorkPath := filepath.Join(s.cfg.Application.WorkPath, spec.ID)
	traceID := tracing.GetTraceID(ctx)

	s.logger.Info("snapshot started",
		"trace_id", traceID,
		"snapshot_id", spec.ID,
		"db_type", spec.DBType,
		"ip", spec.IP,
		"port", spec.Port,
		"schema", spec.Schema,
		"workspace", snapshotWorkPath,
		"storages", len(spec.Storages),
	)

	localResult, attachmentID, err := s.executeSnapshot(ctx, spec, snapshotWorkPath, start, traceID)
	if err != nil {
		return result, err
	}

	result = s.syncToStorages(ctx, spec, localResult, attachmentID)

	s.cleanupWorkspace(ctx, spec, snapshotWorkPath)

	s.logger.Info("snapshot finished",
		"trace_id", traceID,
		"snapshot_id", spec.ID,
		"duration_s", time.Since(start).Seconds(),
		"storage_success", result.StorageSuccessCount,
		"failed_details", len(result.FailedDetails),
	)

	return result, nil
}

// executeSnapshot 执行数据库备份导出并保存本地元数据。
// 主要流程：
//  1. 根据 db_type 获取对应的备份适配器
//  2. 调用适配器的 Snapshots() 方法执行导出（导出 → 压缩 → 哈希）
//  3. 生成唯一 ID 并构建本地备份元数据
//  4. 将元数据插入 SQLite 数据库
//
// 使用场景：handleOne() 内部调用，是快照任务的第一步。
func (s *SnapshotsService) executeSnapshot(ctx context.Context, spec config.SnapshotsSpec, workPath string, start time.Time, traceID string) (snapshots.SnapshotsResult, string, error) {
	adapter, err := s.snapshotsRegs.Get(spec.DBType)
	if err != nil {
		return snapshots.SnapshotsResult{}, "", err
	}

	localResult, err := adapter.Snapshots(ctx, snapshots.SnapshotsRequest{
		Spec:     spec,
		WorkPath: workPath,
	})
	if err != nil {
		return snapshots.SnapshotsResult{}, "", err
	}
	s.logger.Info("snapshot exported",
		"trace_id", traceID,
		"snapshot_id", spec.ID,
		"file", localResult.FilePath,
		"hash", localResult.FileHash,
	)

	attachmentID := s.idGen.NextIDString()
	att := domain.SnapshotAttachment{
		ID:                 attachmentID,
		DBIP:               spec.IP,
		DBPort:             spec.Port,
		DBSchema:           spec.Schema,
		WorkDir:            filepath.Dir(localResult.FilePath),
		FileName:           filepath.Base(localResult.FilePath),
		FileHash:           localResult.FileHash,
		SnapshotsStartTime: start,
		SnapshotsDurationS: int(time.Since(start).Seconds()),
		CreatedAt:          time.Now(),
	}
	if err := s.repo.InsertSnapshotsAttachment(ctx, att); err != nil {
		return snapshots.SnapshotsResult{}, "", err
	}
	s.logger.Info("local metadata saved",
		"trace_id", traceID,
		"snapshot_id", spec.ID,
		"attachment_id", attachmentID,
		"file", att.FileName,
	)

	return localResult, attachmentID, nil
}

// syncToStorages 将备份文件同步到各个存储位置。
// 主要逻辑：遍历快照配置中的所有存储目标，
// 区分本地存储（复制文件）和远端存储（通过适配器上传）分别处理。
// 使用场景：handleOne() 内部调用，是快照任务的第二步。
func (s *SnapshotsService) syncToStorages(ctx context.Context, spec config.SnapshotsSpec, localResult snapshots.SnapshotsResult, attachmentID string) handleResult {
	result := handleResult{}

	for _, storage := range spec.Storages {
		if isLocalStorage(storage) {
			s.handleLocalStorage(spec, storage, localResult, &result)
		} else {
			s.handleRemoteStorage(ctx, spec, storage, localResult, attachmentID, &result)
		}
	}

	return result
}

// handleLocalStorage 处理本地存储类型的备份同步。
// 主要逻辑：将压缩后的备份文件和日志文件复制到本地目标目录，
// 并在目标目录执行保留策略清理过期文件。
// 使用场景：storage.type 为 "local" 或空时调用。
func (s *SnapshotsService) handleLocalStorage(spec config.SnapshotsSpec, storage config.StorageSpec, localResult snapshots.SnapshotsResult, result *handleResult) {
	s.logger.Info("local storages processing",
		"snapshot_id", spec.ID,
		"storage_id", storage.ID,
		"path", storage.WorkPath,
		"retention_count", storage.RetentionCount,
	)

	if _, err := copyLocalArtifacts(localResult.FilePath, storage.WorkPath); err != nil {
		s.logger.Warn("local copy failed", "snapshot_id", spec.ID, "storage_id", storage.ID, "err", err)
		result.FailedDetails = append(result.FailedDetails, fmt.Sprintf("local copy failed (id=%s): %v", storage.ID, err))
		return
	}
	s.logger.Info("local copy completed", "snapshot_id", spec.ID, "storage_id", storage.ID, "path", storage.WorkPath)

	if storage.RetentionCount > 0 {
		if err := s.retention.ApplyLocal(storage.WorkPath, storage.RetentionCount, spec); err != nil {
			s.logger.Warn("local retention failed", "snapshot_id", spec.ID, "storage_id", storage.ID, "err", err)
			result.FailedDetails = append(result.FailedDetails, fmt.Sprintf("local retention failed (id=%s): %v", storage.ID, err))
		} else {
			s.logger.Info("local retention applied", "snapshot_id", spec.ID, "storage_id", storage.ID, "retention_count", storage.RetentionCount)
		}
	}
}

// handleRemoteStorage 处理远端存储类型的备份同步（带重试）。
// 主要流程：
//  1. 根据 storage.type 获取对应的存储适配器
//  2. 通过重试机制上传文件到远端存储
//  3. 记录上传指标
//  4. 保存远端存储元数据
//  5. 执行远端保留策略
//
// 使用场景：storage.type 为 "host"、"s3" 或 "webdav" 时调用。
func (s *SnapshotsService) handleRemoteStorage(ctx context.Context, spec config.SnapshotsSpec, storage config.StorageSpec, localResult snapshots.SnapshotsResult, attachmentID string, result *handleResult) {
	storageAdapter, err := s.storageRegs.Get(storage.Type)
	if err != nil {
		s.logger.Warn("unsupported storages type", "snapshot_id", spec.ID, "storage_id", storage.ID, "type", storage.Type, "err", err)
		result.FailedDetails = append(result.FailedDetails, fmt.Sprintf("unsupported storages type (id=%s, type=%s)", storage.ID, storage.Type))
		return
	}

	s.logger.Info("storages upload started",
		"snapshot_id", spec.ID,
		"storage_id", storage.ID,
		"type", storage.Type,
		"path", storage.WorkPath,
	)

	var storageResult storagepkg.UploadResult
	retryCfg := s.getRetryConfig()
	err = retry.Do(ctx, retryCfg, func() error {
		var uploadErr error
		storageResult, uploadErr = storageAdapter.Upload(ctx, storagepkg.UploadRequest{
			Snapshots:     spec,
			Storage:       storage,
			LocalFile:     localResult.FilePath,
			LocalFileHash: localResult.FileHash,
		})
		return uploadErr
	})

	if err != nil {
		s.logger.Warn("storages upload failed", "snapshot_id", spec.ID, "storage_id", storage.ID, "type", storage.Type, "err", err)
		metrics.GetGlobal().RecordStorageUpload(false)
		result.FailedDetails = append(result.FailedDetails, fmt.Sprintf("storages upload failed (id=%s, type=%s): %v", storage.ID, storage.Type, err))
		return
	}

	s.logger.Info("storages upload completed",
		"snapshot_id", spec.ID,
		"storage_id", storage.ID,
		"type", storage.Type,
		"storage_work_dir", storageResult.StorageWorkDir,
		"file", storageResult.FileName,
	)

	metrics.GetGlobal().RecordStorageUpload(true)
	result.StorageSuccessCount++
	s.saveStorageMetadata(ctx, spec, storage, storageResult, attachmentID)
	s.applyStorageRetention(ctx, spec, storage, storageAdapter, result)
}

// saveStorageMetadata 保存远端存储上传的元数据记录。
// 主要逻辑：构建 StorageSnapshotAttachment 实体并插入数据库。
// 使用场景：文件上传到远端存储成功后调用。
func (s *SnapshotsService) saveStorageMetadata(ctx context.Context, spec config.SnapshotsSpec, storage config.StorageSpec, storageResult storagepkg.UploadResult, attachmentID string) {
	storageAtt := domain.StorageSnapshotAttachment{
		ID:                    s.idGen.NextIDString(),
		SnapshotsAttachmentID: attachmentID,
		StorageName:           storage.ID,
		StorageType:           storage.Type,
		StorageIP:             storage.IP,
		StoragePort:           storage.Port,
		StorageWorkDir:        storageResult.StorageWorkDir,
		FileName:              storageResult.FileName,
		FileHash:              storageResult.FileHash,
		CreatedAt:             time.Now(),
	}
	if err := s.repo.InsertStorageSnapshotsAttachment(ctx, storageAtt); err != nil {
		s.logger.Warn("save storages metadata failed", "snapshot_id", spec.ID, "storage_id", storage.ID, "err", err)
	} else {
		s.logger.Info("storages metadata saved", "snapshot_id", spec.ID, "storage_id", storage.ID, "storage_attachment_id", storageAtt.ID)
	}
}

// applyStorageRetention 应用远端存储保留策略。
// 主要逻辑：通过存储适配器列出远端文件，删除超过 retention_count 的过期文件。
// 使用场景：文件上传到远端存储成功后调用，防止远端存储空间耗尽。
func (s *SnapshotsService) applyStorageRetention(ctx context.Context, spec config.SnapshotsSpec, storage config.StorageSpec, adapter storagepkg.Adapter, result *handleResult) {
	if err := s.retention.ApplyStorage(ctx, storage, adapter, spec); err != nil {
		s.logger.Warn("storages retention failed", "snapshot_id", spec.ID, "storage_id", storage.ID, "type", storage.Type, "err", err)
		result.FailedDetails = append(result.FailedDetails, fmt.Sprintf("storages retention failed (id=%s, type=%s): %v", storage.ID, storage.Type, err))
	} else if storage.RetentionCount > 0 {
		s.logger.Info("storages retention applied", "snapshot_id", spec.ID, "storage_id", storage.ID, "retention_count", storage.RetentionCount)
	}
}

// cleanupWorkspace 清理快照任务的本地工作目录。
// 主要逻辑：对工作目录执行本地保留策略，默认保留 1 个文件（可通过 workspace_retention 配置调整）。
// 使用场景：单个快照任务的所有存储同步完成后调用，清理临时备份文件。
func (s *SnapshotsService) cleanupWorkspace(ctx context.Context, spec config.SnapshotsSpec, workPath string) {
	retentionCount := 1
	if s.cfg.Application.WorkspaceRetention > 0 {
		retentionCount = s.cfg.Application.WorkspaceRetention
	}
	if err := s.retention.ApplyLocal(workPath, retentionCount, spec); err != nil {
		s.logger.Warn("workspace retention failed", "snapshot_id", spec.ID, "workspace", workPath, "err", err)
	} else {
		s.logger.Info("workspace retention applied", "snapshot_id", spec.ID, "workspace", workPath, "retention_count", retentionCount)
	}
}

// BuildFailurePayload 构建单个快照失败的通知载荷。
// 使用场景：快速构建失败通知内容，包含快照 ID、数据库类型、数据库名和错误信息。
// 注意：此函数用于单个快照级别的失败通知，与 sendNotification 的批量汇总通知不同。
func BuildFailurePayload(spec config.SnapshotsSpec, err error) notify.Payload {
	return notify.Payload{
		SnapshotsID: spec.ID,
		DBType:      spec.DBType,
		Schema:      spec.Schema,
		Status:      "failed",
		Message:     fmt.Sprintf("snapshots failed: %v", err),
	}
}

// handleResult 是单个快照任务的处理结果汇总。
// 使用场景：在 handleOne() 中收集存储同步的成功数和失败详情，
// 返回给 Run() 进行整体统计。
type handleResult struct {
	StorageSuccessCount int      // 远端存储同步成功数
	FailedDetails       []string // 失败详情列表（每个失败存储一条描述）
}

// isLocalStorage 判断存储配置是否为本地存储类型。
// 判断规则：type 为空或 "local" 时视为本地存储。
// 使用场景：区分本地复制和远程上传的处理逻辑。
func isLocalStorage(spec config.StorageSpec) bool {
	t := strings.ToLower(strings.TrimSpace(spec.Type))
	return t == "" || t == "local"
}

// copyLocalArtifacts 复制备份产物到本地目标目录。
// 主要逻辑：复制 .tar.zst 文件（已包含导出文件和日志）。
// 使用场景：本地存储类型（local）的备份文件同步。
func copyLocalArtifacts(srcCompressedPath, targetWorkPath string) (string, error) {
	if err := os.MkdirAll(targetWorkPath, 0o755); err != nil {
		return "", fmt.Errorf("create local target dir failed: %w", err)
	}

	dstCompressedPath := filepath.Join(targetWorkPath, filepath.Base(srcCompressedPath))
	if err := copyFile(srcCompressedPath, dstCompressedPath); err != nil {
		return "", err
	}

	return dstCompressedPath, nil
}

// copyFile 使用缓冲区复制单个文件。
// 主要逻辑：打开源文件、创建目标文件、使用 32KB 缓冲区流式复制。
// 如果源路径和目标路径相同则跳过（避免自拷贝）。
// 使用场景：copyLocalArtifacts 内部调用。
func copyFile(srcPath, dstPath string) error {
	if srcPath == dstPath {
		return nil
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source file failed: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create target file failed: %w", err)
	}
	defer dst.Close()

	buf := make([]byte, 32*1024)
	if _, err := io.CopyBuffer(dst, src, buf); err != nil {
		return fmt.Errorf("copy file failed: %w", err)
	}
	return nil
}
