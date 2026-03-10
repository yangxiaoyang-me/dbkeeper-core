package dbkeeper

import (
	"dbkeeper-core/internal/infrastructure/snapshots"
	"dbkeeper-core/internal/infrastructure/storages"
)

// NewSnapshotsRegistry 创建快照适配器注册表。
// 使用场景：外部项目需要注册自定义数据库快照适配器时调用。
// 例如：支持 Oracle、SQL Server 等非内置数据库类型时，
// 实现 SnapshotAdapter 接口并通过此函数注册。
//
// 示例：
//
//	registry := dbkeeper.NewSnapshotsRegistry(
//	    &MyOracleAdapter{},
//	    &snapshots.MySQLAdapter{},
//	)
func NewSnapshotsRegistry(adapters ...SnapshotAdapter) SnapshotsRegistry {
	return snapshots.NewRegistry(adapters...)
}

// NewStorageRegistry 创建存储适配器注册表。
// 使用场景：外部项目需要注册自定义存储适配器时调用。
// 例如：支持 FTP、Azure Blob 等非内置存储类型时，
// 实现 StorageAdapter 接口并通过此函数注册。
func NewStorageRegistry(adapters ...StorageAdapter) StorageRegistry {
	return storages.NewRegistry(adapters...)
}
