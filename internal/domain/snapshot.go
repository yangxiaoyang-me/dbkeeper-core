package domain

import "time"

// SnapshotAttachment 是本地备份附件实体，代表一次数据库备份操作的完整元数据。
// 每次执行数据库导出后，会生成一条 SnapshotAttachment 记录并持久化到 SQLite 元数据库中。
// 使用场景：记录每一次备份文件的来源数据库、文件路径、哈希校验值、耗时等信息，
// 便于后续查询备份历史、校验文件完整性、审计备份记录。
type SnapshotAttachment struct {
	ID                 string    // 备份记录唯一标识，由雪花算法生成
	DBIP               string    // 源数据库 IP 地址
	DBPort             int       // 源数据库端口号
	DBSchema           string    // 源数据库名称（schema/database）
	WorkDir            string    // 备份文件所在的工作目录路径
	FileName           string    // 备份文件名（含压缩后缀，如 mysql_127.0.0.1_3306_test_v0.1.0_20240101120000.sql.zst）
	FileHash           string    // 压缩后文件的 SHA256 哈希值（小写十六进制），用于完整性校验
	SnapshotsStartTime time.Time // 备份开始时间
	SnapshotsDurationS int       // 备份耗时（秒）
	CreatedAt          time.Time // 记录创建时间
}

// StorageSnapshotAttachment 是远端备份附件实体，记录备份文件上传到远端存储后的元数据。
// 与 SnapshotAttachment 通过 SnapshotsAttachmentID 关联，形成一对多关系
// （一个本地备份可以上传到多个远端存储位置）。
// 使用场景：记录备份文件在远端存储的具体位置（存储类型、IP、路径、文件名），
// 便于追踪备份文件的分发情况和灾备恢复。
type StorageSnapshotAttachment struct {
	ID                    string    // 远端备份记录唯一标识，由雪花算法生成
	SnapshotsAttachmentID string    // 关联的本地备份记录 ID（外键，指向 SnapshotAttachment.ID）
	StorageName           string    // 存储名称（配置中的 storage.id，如 "nas-backup"）
	StorageType           string    // 存储类型（host/s3/webdav）
	StorageIP             string    // 远端存储 IP 地址（host 类型时使用）
	StoragePort           int       // 远端存储端口号（host 类型时使用）
	StorageWorkDir        string    // 远端存储工作目录路径
	FileName              string    // 远端存储中的文件名
	FileHash              string    // 文件 SHA256 哈希值，与本地文件一致用于校验
	CreatedAt             time.Time // 记录创建时间
}
