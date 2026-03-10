// Package snapshots 提供多数据库备份适配器功能。
//
// 采用适配器模式，通过统一的 Adapter 接口支持多种数据库的备份导出：
//   - mysql：使用 mysqldump 导出 SQL 文件
//   - dm：使用 dexp 导出达梦数据库
//   - pg：使用 pg_dump 导出 PostgreSQL 数据库
//
// 导出后的文件会自动压缩（zstd）并计算哈希（SHA256）。
// 使用 Registry 注册表按 db_type 配置选择对应的适配器实现。
// 使用场景：备份流程中根据配置的数据库类型，选择对应适配器执行导出。
//
// 扩展方式：实现 Adapter 接口并通过 Registry 注册，即可支持新的数据库类型。
package snapshots

import (
	"context"
	"fmt"
	"strings"

	"dbkeeper-core/internal/config"
)

// SnapshotsRequest 是数据库备份请求，封装快照配置和工作目录。
// 使用场景：传递给具体数据库适配器的 Snapshots() 方法，触发备份导出。
type SnapshotsRequest struct {
	Spec     config.SnapshotsSpec // 快照任务配置（数据库连接信息、认证等）
	WorkPath string               // 工作目录（备份文件的输出目录）
}

// SnapshotsResult 是数据库备份执行结果，包含压缩后的文件路径和哈希值。
// 使用场景：备份导出成功后返回，用于后续上传到远端存储和记录元数据。
type SnapshotsResult struct {
	FilePath string // 压缩后的备份文件路径（如 /work/mysql_127.0.0.1_3306_test.sql.zst）
	FileHash string // 文件 SHA256 哈希值（小写十六进制）
}

// Adapter 定义数据库备份适配器接口。
// 所有数据库类型（mysql/dm/pg）必须实现此接口。
// 使用场景：通过接口多态实现不同数据库的统一备份调用。
//
// 扩展方式：实现此接口并通过 Registry 注册，即可支持新的数据库类型。
type Adapter interface {
	// Type 返回适配器类型标识（如 "mysql"、"dm"、"pg"）。
	Type() string
	// Snapshots 执行数据库备份导出。
	// 主要流程：调用备份命令 → 导出文件 → 压缩 → 计算哈希 → 返回结果。
	Snapshots(ctx context.Context, req SnapshotsRequest) (SnapshotsResult, error)
}

// Registry 是数据库备份适配器注册表，按类型名映射到具体适配器实例。
// 使用场景：应用启动时注册所有支持的数据库适配器，运行时按配置类型查找。
type Registry struct {
	items map[string]Adapter // 类型名（小写）到适配器的映射
}

// NewRegistry 创建备份适配器注册表并注册所有传入的适配器。
// 使用场景：应用启动时调用，传入所有支持的数据库适配器实例。
func NewRegistry(adapters ...Adapter) *Registry {
	m := make(map[string]Adapter)
	for _, a := range adapters {
		m[strings.ToLower(a.Type())] = a
	}
	return &Registry{items: m}
}

// Get 获取指定数据库类型的备份适配器。
// 参数 dbType 为数据库类型名（不区分大小写），如 "mysql"、"dm"、"pg"。
// 使用场景：每个快照任务执行时，按配置的 db_type 查找对应适配器。
func (r *Registry) Get(dbType string) (Adapter, error) {
	key := strings.ToLower(dbType)
	adapter, ok := r.items[key]
	if !ok {
		return nil, fmt.Errorf("不支持的数据库类型: %s", dbType)
	}
	return adapter, nil
}
