// Package persistence 提供备份元数据的持久化存储功能。
//
// 当前实现基于 SQLite（使用纯 Go 驱动 modernc.org/sqlite），
// 存储两张核心表：
//   - snapshots_attachment：本地备份元数据（数据库信息、文件路径、哈希、耗时等）
//   - storage_snapshots_attachment：远端存储元数据（存储类型、IP、路径等，与本地备份关联）
//
// 使用场景：记录每次备份和上传的完整元数据，支持历史查询、完整性校验和审计。
package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dbkeeper-core/internal/config"
	"dbkeeper-core/internal/domain"

	_ "modernc.org/sqlite"
)

// SQLiteRepository 是基于 SQLite 的元数据仓储实现。
// 实现 application.SnapshotsRepository 接口，提供元数据的初始化、插入功能。
// 使用场景：应用启动时创建，贯穿整个备份流程用于持久化元数据。
type SQLiteRepository struct {
	cfg config.Database // 数据库配置（文件路径、连接参数等）
	db  *sql.DB         // 底层数据库连接池
}

// initSchemaSQL 是数据库表初始化 SQL，包含两张表的建表语句和索引。
// snapshots_attachment 表记录本地备份元数据；
// storage_snapshots_attachment 表记录远端存储元数据，通过外键关联。
const initSchemaSQL = `
CREATE TABLE IF NOT EXISTS snapshots_attachment (
  id                CHAR(32)      NOT NULL PRIMARY KEY,
  db_ip             VARCHAR(45)   NOT NULL,
  db_port           INTEGER       NOT NULL,
  db_schema         VARCHAR(128)  NOT NULL,
  work_dir          VARCHAR(1024) NOT NULL,
  file_name         VARCHAR(512)  NOT NULL,
  file_hash         CHAR(64)      NOT NULL,
  snapshots_start_time TIMESTAMP     NOT NULL,
  snapshots_duration_s INTEGER       NOT NULL,
  created_at        TIMESTAMP     NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_snapshots_attachment_db_time
  ON snapshots_attachment (db_ip, db_port, db_schema, snapshots_start_time);

CREATE INDEX IF NOT EXISTS idx_snapshots_attachment_hash
  ON snapshots_attachment (file_hash);

CREATE TABLE IF NOT EXISTS storage_snapshots_attachment (
  id                    CHAR(32)      NOT NULL PRIMARY KEY,
  snapshots_attachment_id  CHAR(32)      NOT NULL,
  storage_name           VARCHAR(128)  NOT NULL,
  storage_type           VARCHAR(32)   NOT NULL,
  storage_ip             VARCHAR(45)   NULL,
  storage_port           INTEGER       NULL,
  storage_work_dir       VARCHAR(1024) NOT NULL,
  file_name             VARCHAR(512)  NOT NULL,
  file_hash             CHAR(64)      NOT NULL,
  created_at            TIMESTAMP     NOT NULL,
  FOREIGN KEY (snapshots_attachment_id) REFERENCES snapshots_attachment(id)
);

CREATE INDEX IF NOT EXISTS idx_storage_snapshots_attachment_snapshots_id
  ON storage_snapshots_attachment (snapshots_attachment_id);

CREATE UNIQUE INDEX IF NOT EXISTS uk_storage_snapshots_attachment_dedup
  ON storage_snapshots_attachment (snapshots_attachment_id, storage_name, storage_work_dir, file_name);
`

// NewSQLiteRepository 创建 SQLite 元数据仓储实例。
// 主要逻辑：创建数据库文件目录、打开 SQLite 连接、配置连接池参数、应用 PRAGMA 优化。
// 使用场景：应用启动时调用，传入数据库配置。
func NewSQLiteRepository(cfg config.Database) (*SQLiteRepository, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.FilePath), 0o755); err != nil {
		return nil, fmt.Errorf("创建数据库目录失败: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_foreign_keys=on", cfg.FilePath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开SQLite失败: %w", err)
	}

	maxConns := cfg.MaxOpenConns
	if maxConns <= 0 {
		maxConns = 10
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)

	repo := &SQLiteRepository{cfg: cfg, db: db}
	if err := repo.applyPragmas(); err != nil {
		return nil, err
	}
	return repo, nil
}

// InitSchema 初始化数据库表结构。
// 使用内置 SQL 语句创建表和索引（IF NOT EXISTS），无需外部 migration 文件。
// 使用场景：每次运行开始时调用，确保表结构存在（幂等操作）。
func (r *SQLiteRepository) InitSchema(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, initSchemaSQL)
	if err != nil {
		return fmt.Errorf("执行初始化脚本失败: %w", err)
	}
	return nil
}

// InsertSnapshotsAttachment 插入一条本地备份元数据记录。
// 记录内容包括：数据库地址、文件路径、哈希值、备份耗时等。
// 使用场景：数据库导出并压缩完成后调用，记录本次备份的元信息。
func (r *SQLiteRepository) InsertSnapshotsAttachment(ctx context.Context, att domain.SnapshotAttachment) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO snapshots_attachment (
	id, db_ip, db_port, db_schema, work_dir, file_name, file_hash,
	snapshots_start_time, snapshots_duration_s, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, att.ID, att.DBIP, att.DBPort, att.DBSchema, att.WorkDir, att.FileName, att.FileHash, att.SnapshotsStartTime, att.SnapshotsDurationS, att.CreatedAt)
	if err != nil {
		return fmt.Errorf("写入本地元数据失败: %w", err)
	}
	return nil
}

// InsertStorageSnapshotsAttachment 插入一条远端存储备份元数据记录。
// 通过 snapshots_attachment_id 外键关联到对应的本地备份记录。
// 使用场景：备份文件上传到远端存储后调用，记录存储位置信息。
func (r *SQLiteRepository) InsertStorageSnapshotsAttachment(ctx context.Context, att domain.StorageSnapshotAttachment) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO storage_snapshots_attachment (
	id, snapshots_attachment_id, storage_name, storage_type, storage_ip, storage_port,
	storage_work_dir, file_name, file_hash, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, att.ID, att.SnapshotsAttachmentID, att.StorageName, att.StorageType, att.StorageIP, att.StoragePort, att.StorageWorkDir, att.FileName, att.FileHash, att.CreatedAt)
	if err != nil {
		return fmt.Errorf("写入远端元数据失败: %w", err)
	}
	return nil
}

// applyPragmas 根据配置应用 SQLite PRAGMA 优化参数。
// 支持的参数：journal_mode（WAL 模式提升并发性能）、synchronous（同步级别）、
// busy_timeout（忙等待超时）。
// 使用场景：创建仓储实例后立即调用，优化 SQLite 性能。
func (r *SQLiteRepository) applyPragmas() error {
	var stmts []string
	if r.cfg.JournalMode != "" {
		stmts = append(stmts, fmt.Sprintf("PRAGMA journal_mode=%s;", r.cfg.JournalMode))
	}
	if r.cfg.Synchronous != "" {
		stmts = append(stmts, fmt.Sprintf("PRAGMA synchronous=%s;", r.cfg.Synchronous))
	}
	if r.cfg.BusyTimeout > 0 {
		stmts = append(stmts, fmt.Sprintf("PRAGMA busy_timeout=%d;", r.cfg.BusyTimeout))
	}

	if len(stmts) == 0 {
		return nil
	}

	query := strings.Join(stmts, "\n")
	_, err := r.db.Exec(query)
	if err != nil {
		return fmt.Errorf("设置SQLite参数失败: %w", err)
	}
	return nil
}
