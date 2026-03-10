// Package storages 提供备份文件的远端存储功能。
//
// 采用适配器模式，通过统一的 Adapter 接口支持多种存储后端：
//   - host (SFTP)：通过 SSH/SFTP 上传到远端 Linux 服务器
//   - s3 (S3/MinIO)：上传到 S3 兼容的对象存储服务
//   - webdav (WebDAV)：上传到支持 WebDAV 协议的存储服务
//
// 每种存储适配器均提供上传（Upload）、列表（List）和删除（Delete）三个操作。
// 使用 Registry 注册表按 storage.type 配置选择对应的适配器实现。
// 使用场景：备份文件在本地压缩后，上传到一个或多个远端存储位置进行灾备。
package storages

import (
	"context"
	"fmt"
	"strings"

	"dbkeeper-core/internal/config"
)

// UploadRequest 是远端上传请求，包含快照配置、存储配置和本地文件信息。
// 使用场景：将本地备份文件上传到远端存储时，封装所有必要参数。
type UploadRequest struct {
	Snapshots     config.SnapshotsSpec // 快照任务配置（数据库信息）
	Storage       config.StorageSpec   // 存储目标配置（类型、地址、认证等）
	LocalFile     string               // 本地备份文件路径（压缩后的 .zst 文件）
	LocalFileHash string               // 本地文件的 SHA256 哈希值
}

// UploadResult 是远端上传结果，包含文件在远端存储中的位置信息。
// 使用场景：上传成功后返回，用于记录远端存储元数据。
type UploadResult struct {
	StorageWorkDir string // 远端存储工作目录
	FileName       string // 远端文件名
	FileHash       string // 文件哈希值（与本地一致）
}

// Adapter 定义远端存储适配器接口。
// 所有存储类型（host/s3/webdav）必须实现此接口。
// 使用场景：通过接口多态实现不同存储后端的统一调用。
//
// 扩展方式：实现此接口并通过 Registry 注册，即可支持新的存储类型。
type Adapter interface {
	// Type 返回适配器类型标识（如 "host"、"s3"、"webdav"）。
	Type() string
	// Upload 上传本地文件到远端存储。
	Upload(ctx context.Context, req UploadRequest) (UploadResult, error)
	// List 列出远端存储目录中的文件。
	List(ctx context.Context, req ListRequest) ([]StorageFile, error)
	// Delete 删除远端存储中的指定文件。
	Delete(ctx context.Context, req DeleteRequest) error
}

// Registry 是远端存储适配器注册表，按类型名映射到具体适配器实例。
// 使用场景：应用启动时注册所有支持的存储适配器，运行时按配置类型查找。
type Registry struct {
	items map[string]Adapter // 类型名（小写）到适配器的映射
}

// NewRegistry 创建存储适配器注册表并注册所有传入的适配器。
// 使用场景：应用启动时调用，传入所有支持的存储适配器实例。
func NewRegistry(adapters ...Adapter) *Registry {
	m := make(map[string]Adapter)
	for _, a := range adapters {
		m[strings.ToLower(a.Type())] = a
	}
	return &Registry{items: m}
}

// Get 获取指定类型的存储适配器。
// 参数 t 为存储类型名（不区分大小写），如 "host"、"s3"、"webdav"。
// 使用场景：备份上传时按配置的 storage.type 查找对应适配器。
func (r *Registry) Get(t string) (Adapter, error) {
	key := strings.ToLower(t)
	adapter, ok := r.items[key]
	if !ok {
		return nil, fmt.Errorf("不支持的远端类型: %s", t)
	}
	return adapter, nil
}

// StorageFile 是远端存储文件的简要信息。
// 使用场景：保留策略时按修改时间排序，决定哪些文件需要删除。
type StorageFile struct {
	Name    string // 文件名（或对象键名）
	ModTime int64  // 最后修改时间的 Unix 时间戳（秒）
}

// ListRequest 是远端文件列表请求。
// 使用场景：保留策略执行前，列出远端存储目录中的所有文件。
type ListRequest struct {
	Storage config.StorageSpec // 存储配置（包含连接信息和路径）
	Prefix  string             // 文件名前缀过滤（空字符串表示不过滤）
}

// DeleteRequest 是远端文件删除请求。
// 使用场景：保留策略清理过期文件时，指定要删除的文件。
type DeleteRequest struct {
	Storage config.StorageSpec // 存储配置（包含连接信息和路径）
	Name    string             // 要删除的文件名（或对象键名）
}

// normalizeSlashPath 将路径中的反斜杠统一转换为正斜杠，并去除首尾空格。
// 使用场景：统一处理来自 Windows（反斜杠）和 Linux/macOS（正斜杠）的路径输入，
// 避免在远端存储操作中出现路径格式不一致的问题。
func normalizeSlashPath(p string) string {
	return strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
}
