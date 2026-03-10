// Package main 是 dbkeeper-core 的启动入口。
//
// dbkeeper-core 是一个多数据库备份管理工具，支持 MySQL、达梦（DM）、PostgreSQL 三种数据库，
// 支持本地、SFTP、S3、WebDAV 四种存储方式，具备保留策略和通知功能。
//
// 启动流程：
//  1. 解析命令行参数（-config 指定配置文件路径，-version 打印版本号）
//  2. 确保配置文件存在（不存在时从备份或嵌入资源恢复）
//  3. 加载并校验配置
//  4. 初始化日志、元数据仓储、ID 生成器、通知器等组件
//  5. 注册数据库快照适配器和存储适配器
//  6. 构建服务实例并执行一次完整备份流程
//  7. 监听系统信号（SIGINT/SIGTERM）实现优雅退出
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"dbkeeper-core/internal/application"
	"dbkeeper-core/internal/appmeta"
	"dbkeeper-core/internal/config"
	"dbkeeper-core/internal/id"
	"dbkeeper-core/internal/infrastructure/notify"
	"dbkeeper-core/internal/infrastructure/persistence"
	"dbkeeper-core/internal/infrastructure/retention"
	"dbkeeper-core/internal/infrastructure/snapshots"
	"dbkeeper-core/internal/infrastructure/storages"
	"dbkeeper-core/internal/logging"
)

// embeddedConfigBackup 是编译时嵌入的 config-backup.yaml 文件内容。
// 使用场景：当运行目录下不存在 config.yaml 且没有 config-backup.yaml 时，
// 从二进制中提取默认配置文件，确保首次运行时自动生成配置。
//
//go:embed config-backup.yaml
var embeddedConfigBackup []byte

// main 是程序入口函数。
// 主要逻辑：依次完成参数解析、配置加载、组件初始化、服务执行和信号监听。
// 使用场景：作为独立二进制运行，通常由 cron 或 systemd 定时调度执行。
func main() {
	cfgPath := flag.String("config", "./config.yaml", "config file path")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("version=%s build_time=%s\n", appmeta.Version, appmeta.BuildTime)
		return
	}

	if err := ensureConfigFromBackup(*cfgPath); err != nil {
		fmt.Fprintf(os.Stderr, "[dbkeeper-core] ensure config failed: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[dbkeeper-core] load config failed: %v\n", err)
		os.Exit(1)
	}

	logger, err := logging.New(cfg.Application.Log.Dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[dbkeeper-core] init logger failed: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = logger.Close()
	}()

	logger.Info("service starting", "version", appmeta.Version, "build_time", appmeta.BuildTime, "config", *cfgPath)
	logger.Info("config loaded",
		"snapshots", len(cfg.Application.Snapshots),
		"work_path", cfg.Application.WorkPath,
		"concurrency", cfg.Application.Concurrency,
		"log_dir", cfg.Application.Log.Dir,
	)

	repo, err := persistence.NewSQLiteRepository(cfg.Application.Database)
	if err != nil {
		logger.Error("init metadata repository failed", "err", err)
		os.Exit(1)
	}
	logger.Info("metadata repository initialized",
		"db_type", cfg.Application.Database.DBType,
		"file_path", cfg.Application.Database.FilePath,
	)

	idGen, err := id.New(1, 1)
	if err != nil {
		logger.Error("init id generator failed", "err", err)
		os.Exit(1)
	}

	notifier := notify.NewNotifier(cfg.Application.Notify)
	retentionMgr := retention.New()

	snapshotsRegistry := snapshots.NewRegistry(
		&snapshots.MySQLAdapter{},
		&snapshots.DMAdapter{},
		&snapshots.PGAdapter{},
	)

	storageRegistry := storages.NewRegistry(
		&storages.HostAdapter{},
		&storages.S3Adapter{},
		&storages.WebDAVAdapter{},
	)

	service := application.NewSnapshotsService(cfg, logger, repo, idGen, notifier, retentionMgr, snapshotsRegistry, storageRegistry)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		logger.Warn("shutdown signal received")
		cancel()
	}()

	if err := service.Run(ctx); err != nil {
		logger.Error("service run failed", "err", err)
		os.Exit(1)
	}
	logger.Info("service finished")
}

// ensureConfigFromBackup 确保配置文件存在，不存在时自动从备份恢复。
// 恢复优先级：config-backup.yaml（同目录） > 二进制嵌入的 config-backup.yaml。
// 注意：仅对默认文件名 config.yaml 生效，自定义配置名不会触发自动恢复，避免意外覆盖。
// 使用场景：首次部署或配置文件被误删时自动恢复默认配置。
func ensureConfigFromBackup(cfgPath string) error {
	// 仅对 config.yaml 进行自动恢复，避免对自定义配置名产生意外行为
	if !strings.EqualFold(filepath.Base(cfgPath), "config.yaml") {
		return nil
	}

	if _, err := os.Stat(cfgPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	backupPath := filepath.Join(filepath.Dir(cfgPath), "config-backup.yaml")
	if data, err := os.ReadFile(backupPath); err == nil {
		return writeConfigFile(cfgPath, data)
	} else if !os.IsNotExist(err) {
		return err
	}

	if len(embeddedConfigBackup) == 0 {
		return fmt.Errorf("config.yaml not found and no backup available in file or binary: %s", backupPath)
	}
	return writeConfigFile(cfgPath, embeddedConfigBackup)
}

// writeConfigFile 将配置数据写入指定路径，自动创建父目录。
// 使用场景：ensureConfigFromBackup 恢复配置时调用。
func writeConfigFile(cfgPath string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return fmt.Errorf("create config directory failed: %w", err)
	}
	dst, err := os.OpenFile(cfgPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create config file failed: %w", err)
	}
	if _, err := dst.Write(data); err != nil {
		_ = dst.Close()
		return fmt.Errorf("write config file failed: %w", err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("close config file failed: %w", err)
	}
	return nil
}
