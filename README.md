# DbKeeper (数据库备份工具)

# 一、背景

在生产实际环境中，用户可能存在多种数据库、多实例数据库、多数据库的定时备份与恢复需求。例如：
- 存在多种数据库，如 MySQL、达梦、PG。
- 同一数据库存在多实例，如达梦在不同 IP/端口/实例。
- 同一 IP 与端口下也可能存在多个数据库。

因此需要一个统一的、多数据库的备份工具，并支持多份保留与多地备份。



# 二、工具说明

1. 名称：数据库备份工具（`DbKeeper`）。
2. 支持备份 MySQL、达梦、PG 数据库。
3. 支持多线程备份，线程池可配置。
4. 支持配置备份工作目录、数据库 IP、端口、用户、密码、数据库名、数据库类型（MySQL/达梦/PG）。
5. 支持将备份后的文件迁移到多地进行备份，内置存储支持 `local/host/s3/webdav`，并支持配置保留数量。
6. 备份完成后支持通知：目前支持发送指定 HTTP/HTTPS 请求进行通知。



# 三、软件架构要求

1. 使用 DDD 的模式进行开发，领域对象使用充血模型。
2. 所有文件格式均使用 UTF-8。
3. 每个文件、每个结构体、每个方法等均要详细备注，包括业务逻辑、方法内主要逻辑、以及使用场景等。
4. 所有备注、注释、异常提示、日志等均使用中文。
5. 数据库目前使用 SQLite，后续可能支持其他数据库，所以 SQL 需要写通用 SQL，确保切换数据库时没有切换成本。
6. 数据库初始化脚本单独文件夹存放，表名、字段名需要中文注释。
7. 配置文件使用 `yaml` 格式。
8. 所有业务组件的 ID 使用雪花算法生成，并转换成字符串。



# 四、功能点

1. 配置文件支持如下结构：

   [点击查看详细配置文件](./config-backup.yaml)

   说明：S3 的 `path` 需要包含 `bucket/前缀`，例如 `snapshots-bucket/mysql/97_snapshots/`。

2. 支持配置多个数据库备份，多个数据库可并发进行备份，受并发参数控制。

   补充：数据库导出工具需要预先安装。可选字段 `cmd_path` 用于指定导出命令路径（如 `mysqldump`、`pg_dump`、`dexp`）。

3. 本地使用 SQLite 保存备份过程中的元数据。


4. 所有备份文件在导出后立即压缩为 `.zst`（Zstandard）格式。

- 文件命名规则：`{db_type}_{ip}_{port}_{schema}_{version}_{timestamp}.xxx.zst`。
- 软件版本号在代码中硬编码维护：`internal/appmeta/version.go`。
- 构建时间通过编译参数写入：`go build -ldflags "-X dbkeeper-core/internal/appmeta.BuildTime=2026-03-03T16:30:00+08:00"`。
- 导出后先压缩，再计算哈希，然后执行远程备份。
- `xxx` 在不同数据库中不同：MySQL 导出 `.sql`，达梦导出 `.dmp`，PG 导出 `.dump`。



# 五、构建与使用

## 构建

**PowerShell**

```bat
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"; $bt=Get-Date -Format "yyyy-MM-ddTHH:mm:sszzz"; go build -trimpath -ldflags "-s -w -X dbkeeper-core/internal/appmeta.BuildTime=$bt" -o "dist\dbkeeper-core-linux-amd64" .
```



**bash**

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X dbkeeper-core/internal/appmeta.BuildTime=$(date '+%Y-%m-%dT%H:%M:%S%z')" -o dist/dbkeeper-core-linux-amd64 .
```

如果你想带时区冒号（如 +08:00）：

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
go build -trimpath \
-ldflags "-s -w -X dbkeeper-core/internal/appmeta.BuildTime=$(date '+%Y-%m-%dT%H:%M:%S%:z')" \
-o dist/dbkeeper-core-linux-amd64 .
```


## 特别说明

本软件不包含任何数据库客户端实用程序。

用户必须依据供应商许可安装数据库工具。

## 使用

1. 目前该工具仅支持`MySQL`、`dm`、`PG`

2. 使用对应数据库时，确保当前系统安装了对应系统的备份工具，如：`MySQL`使用`mysqldump`，`dm`使用`dexp`，`pg`使用`pg_dump`

3. 配置文件约定：运行时若不存在 `config.yaml`，程序会优先从同目录 `config-backup.yaml` 复制生成；若同目录也不存在，则使用二进制内嵌的 `config-backup.yaml` 模板生成 `config.yaml`。因此发布包可以只放二进制，也可以额外放 `config-backup.yaml` 作为外部可维护模板。

4. 使用命令

   ```sh
   # 设置执行权限
   chmod 111 ./dbkeeper-core-linux-amd64 
   
   # 执行
   ./dbkeeper-core-linux-amd64 -config ./config.yaml
   ```

   



## 解压

备份产物为 `.tar.zst` 格式（tar 打包 + zstd 压缩），包含数据库导出文件和执行日志。

**① 安装 `zstd`（如果没有）**

CentOS / RHEL / Stream 9：

```
sudo dnf install zstd
```

Ubuntu / Debian：

```
sudo apt install zstd
```

**② 解压**

一步完成解压和解包：

```
tar -I zstd -xf file.tar.zst
```

或分步执行：

```
zstd -d file.tar.zst   # 解压得到 file.tar
tar xf file.tar         # 解包得到原始文件
```
