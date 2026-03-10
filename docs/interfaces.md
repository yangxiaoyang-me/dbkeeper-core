# 接口清单

本文档说明 `dbkeeper-core` 当前可替换的核心接口，方便你在其他项目中通过依赖注入方式复用。

## 1. 应用层接口（`internal/application`）

### `SnapshotsRepository`
- 作用：持久化快照元数据。
- 方法：
  - `InitSchema(ctx context.Context) error`
  - `InsertSnapshotsAttachment(ctx context.Context, att domain.SnapshotAttachment) error`
  - `InsertStorageSnapshotsAttachment(ctx context.Context, att domain.StorageSnapshotAttachment) error`
- 默认实现：`internal/infrastructure/persistence.SQLiteRepository`

### `IDGenerator`
- 作用：生成业务 ID。
- 方法：
  - `NextIDString() string`
- 默认实现：`internal/id.Snowflake`

### `Logger`
- 作用：结构化日志输出。
- 方法：
  - `Debug(msg string, args ...any)`
  - `Info(msg string, args ...any)`
  - `Warn(msg string, args ...any)`
  - `Error(msg string, args ...any)`
- 默认实现：`internal/logging.Logger`

### `SnapshotsRegistry`
- 作用：按 `snapshot.db_type` 获取数据库快照适配器。
- 方法：
  - `Get(dbType string) (snapshots.Adapter, error)`
- 默认实现：`internal/infrastructure/snapshots.Registry`

### `StorageRegistry`
- 作用：按 `snapshot.storages[].type` 获取存储适配器。
- 方法：
  - `Get(t string) (storages.Adapter, error)`
- 默认实现：`internal/infrastructure/storages.Registry`

### `RetentionPolicy`
- 作用：执行本地与存储端保留策略。
- 方法：
  - `ApplyLocal(workPath string, retentionCount int, snapshotsSpec config.SnapshotsSpec) error`
  - `ApplyStorage(ctx context.Context, spec config.StorageSpec, adapter storages.Adapter, snapshotsSpec config.SnapshotsSpec) error`
- 默认实现：`internal/infrastructure/retention.Manager`

## 2. 通知接口（`internal/infrastructure/notify`）

### `Notifier`
- 作用：统一通知抽象。
- 方法：
  - `Notify(ctx context.Context, payload Payload) error`
- 默认构造：
  - `notify.NewNotifier(cfg config.Notify) Notifier`
  - 若没有可用渠道，返回 `nil`（表示不通知）

## 3. 适配器接口

### `snapshots.Adapter`
- 作用：数据库快照导出实现。
- 方法：
  - `Type() string`
  - `Snapshots(ctx context.Context, req SnapshotsRequest) (SnapshotsResult, error)`
- 内置实现：MySQL / DM / PG

### `storages.Adapter`
- 作用：存储上传与保留策略实现。
- 方法：
  - `Type() string`
  - `Upload(ctx context.Context, req UploadRequest) (UploadResult, error)`
  - `List(ctx context.Context, req ListRequest) ([]StorageFile, error)`
  - `Delete(ctx context.Context, req DeleteRequest) error`
- 内置实现：host / s3 / webdav
- 内置本地策略：`local`（由应用层直接处理本地复制与保留，不经过 `storages.Adapter`）

## 4. 对外 SDK（`pkg/dbkeeper`）

你可以只依赖 `pkg/dbkeeper`，无需直接访问 `internal` 目录：

- 配置加载：`LoadConfig(path string) (*Config, error)`
- 运行时构造：`NewRuntime(cfg *Config, deps *Dependencies) (*Runtime, error)`
- 执行入口：`(*Runtime).Run(ctx context.Context) error`
- 资源释放：`(*Runtime).Close() error`
- 自定义注册表：
  - `NewSnapshotsRegistry(adapters ...SnapshotAdapter) SnapshotsRegistry`
  - `NewStorageRegistry(adapters ...StorageAdapter) StorageRegistry`

`pkg/dbkeeper` 中的 `Config/Application/SnapshotsSpec/StorageSpec` 等类型已导出，可在代码中直接逐项赋值，不必依赖 YAML 文件读入。
