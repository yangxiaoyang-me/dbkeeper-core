# SDK 使用说明（接口驱动）

`pkg/dbkeeper` 的核心用法是：
- 配置作为输入（来自 YAML 或代码）
- 运行逻辑通过 `Runtime`
- 扩展点通过 `Dependencies` 中的接口注入

## 1. 先看接口（你真正要依赖的能力）

内置存储能力：
- `local`（本地复制与本地保留策略）
- `host`（远端主机）
- `s3`
- `webdav`

可注入接口：
- `dbkeeper.Logger`
- `dbkeeper.SnapshotsRepository`
- `dbkeeper.IDGenerator`
- `dbkeeper.Notifier`
- `dbkeeper.RetentionPolicy`
- `dbkeeper.SnapshotsRegistry`
- `dbkeeper.StorageRegistry`

运行入口：
- `dbkeeper.NewRuntime(cfg, deps)`
- `(*Runtime).Run(ctx)`
- `(*Runtime).Close()`

## 2. 最小可运行示例（接口注入）

下面示例只替换通知接口，其他接口使用默认实现：

```go
package main

import (
	"context"
	"log"

	"dbkeeper-core/pkg/dbkeeper"
)

type customNotifier struct{}

func (n *customNotifier) Notify(ctx context.Context, payload dbkeeper.NotifyPayload) error {
	log.Printf("notify status=%s failed=%d", payload.Status, payload.FailedCount)
	return nil
}

func main() {
	cfg, err := dbkeeper.LoadConfig("./config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	deps := &dbkeeper.Dependencies{
		Notifier: &customNotifier{},
	}

	rt, err := dbkeeper.NewRuntime(cfg, deps)
	if err != nil {
		log.Fatal(err)
	}
	defer rt.Close()

	if err := rt.Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
```

## 3. 自定义适配器注册（接口扩展）

如果你有自定义数据库类型或存储类型，先实现接口，再放入注册表注入：

```go
deps := &dbkeeper.Dependencies{
	SnapshotsRegistry: dbkeeper.NewSnapshotsRegistry(
		&MySQLAdapter{},   // 内置或你自定义的实现
		&MyCustomDBAdapter{},
	),
	StorageRegistry: dbkeeper.NewStorageRegistry(
		&HostAdapter{},
		&MyObjectStorageAdapter{},
	),
}
```

说明：
- `NewSnapshotsRegistry` 参数类型是 `dbkeeper.SnapshotAdapter`
- `NewStorageRegistry` 参数类型是 `dbkeeper.StorageAdapter`

## 4. 配置来源（只是输入，不是扩展点）

配置有两种来源：
- `dbkeeper.LoadConfig("./config.yaml")`
- 代码直接构建 `dbkeeper.Config`

这两种方式最终都只是给 `NewRuntime` 提供 `cfg`，真正的可替换能力在 `Dependencies` 接口里。
