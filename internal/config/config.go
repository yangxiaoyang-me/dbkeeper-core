// Package config 提供 dbkeeper-core 的配置加载和校验功能。
//
// 配置通过 YAML 文件定义，支持以下核心配置项：
//   - application.concurrency：并发备份任务数
//   - application.work_path：临时备份文件工作目录
//   - application.log.dir：日志文件目录
//   - application.database：SQLite 元数据库配置
//   - application.notify：通知配置（HTTP/多渠道）
//   - application.retry：重试配置（指数退避）
//   - application.snapshots：数据库快照任务列表（每个数据库一条）
//
// 使用场景：应用启动时通过 Load() 加载配置文件，自动校验配置完整性。
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config 是应用总配置结构，对应 config.yaml 根节点。
// 使用场景：从 YAML 文件加载并传递给 Runtime。
type Config struct {
	Application Application `yaml:"application"`
}

// Application 是应用配置节点，包含并发、工作目录、日志、数据库、通知和快照列表。
// 使用场景：控制备份任务的全局行为。
type Application struct {
	Concurrency        int             `yaml:"concurrency"`         // 并发备份任务数
	WorkPath           string          `yaml:"work_path"`           // 工作目录（临时备份文件存放位置）
	WorkspaceRetention int             `yaml:"workspace_retention"` // 工作目录保留文件数
	TaskTimeoutS       int             `yaml:"task_timeout_s"`      // 单个任务超时时间（秒）
	Log                LogConfig       `yaml:"log"`                 // 日志配置
	Database           Database        `yaml:"database"`            // 元数据库配置
	Notify             Notify          `yaml:"notify"`              // 通知配置
	Retry              RetryConfig     `yaml:"retry"`               // 重试配置
	Snapshots          []SnapshotsSpec `yaml:"snapshots"`           // 快照任务列表
}

// LogConfig 是日志配置。
// 使用场景：指定日志文件存放目录。
type LogConfig struct {
	Dir string `yaml:"dir"` // 日志目录
}

// Database 是元数据库配置（当前使用 SQLite）。
// 使用场景：存储备份元数据（文件路径、哈希、时间等）。
type Database struct {
	DBType       string `yaml:"db_type"`        // 数据库类型（sqlite）
	FilePath     string `yaml:"file_path"`      // 数据库文件路径
	JournalMode  string `yaml:"journal_mode"`   // SQLite journal 模式
	Synchronous  string `yaml:"synchronous"`    // SQLite 同步模式
	BusyTimeout  int    `yaml:"busy_timeout"`   // SQLite 忙等待超时（毫秒）
	MaxOpenConns int    `yaml:"max_open_conns"` // 最大连接数
}

// Notify 是通知配置，支持全局配置或多渠道配置。
// 使用场景：备份完成后发送汇总通知。
type Notify struct {
	Type        string            `yaml:"type"`         // 通知类型（http）
	URLs        []string          `yaml:"urls"`         // 通知 URL 列表（向后兼容）
	Method      string            `yaml:"method"`       // HTTP 方法（GET/POST）
	ChannelType string            `yaml:"channel_type"` // 渠道类型（chuckfang 等）
	Headers     map[string]string `yaml:"headers"`      // 自定义 HTTP 头
	TimeoutMS   int               `yaml:"timeout_ms"`   // 超时时间（毫秒）
	Channels    []NotifyChannel   `yaml:"channels"`     // 多渠道配置
}

// NotifyChannel 是单个通知渠道配置。
// 使用场景：支持多个不同的通知目标（如多个钉钉群）。
type NotifyChannel struct {
	Name        string            `yaml:"name"`         // 渠道名称
	Type        string            `yaml:"type"`         // 渠道类型（http）
	URLs        []string          `yaml:"urls"`         // 通知 URL 列表
	Method      string            `yaml:"method"`       // HTTP 方法
	ChannelType string            `yaml:"channel_type"` // 渠道子类型
	Headers     map[string]string `yaml:"headers"`      // 自定义 HTTP 头
	TimeoutMS   int               `yaml:"timeout_ms"`   // 超时时间（毫秒）
}

// RetryConfig 是重试配置。
// 使用场景：远程上传或通知失败时自动重试。
type RetryConfig struct {
	MaxAttempts    int `yaml:"max_attempts"`     // 最大重试次数
	InitialDelayMS int `yaml:"initial_delay_ms"` // 初始延迟（毫秒）
	MaxDelayMS     int `yaml:"max_delay_ms"`     // 最大延迟（毫秒）
}

// SnapshotsSpec 是单个数据库快照任务配置。
// 使用场景：定义一个数据库实例的备份参数和存储目标。
type SnapshotsSpec struct {
	ID       string        `yaml:"id"`       // 快照任务唯一标识
	DBType   string        `yaml:"db_type"`  // 数据库类型（mysql/dm/pg）
	IP       string        `yaml:"ip"`       // 数据库 IP
	Port     int           `yaml:"port"`     // 数据库端口
	Username string        `yaml:"username"` // 数据库用户名
	Password string        `yaml:"password"` // 数据库密码
	Schema   string        `yaml:"schema"`   // 数据库名/模式名
	CmdPath  string        `yaml:"cmd_path"` // 备份命令路径（可选，默认使用系统 PATH）
	Storages []StorageSpec `yaml:"storages"` // 存储目标列表
}

// StorageSpec 是单个存储配置。
// 使用场景：定义备份文件的存储位置（本地/远程）和保留策略。
type StorageSpec struct {
	ID             string `yaml:"id"`              // 存储唯一标识
	Type           string `yaml:"type"`            // 存储类型（local/host/s3/webdav）
	IP             string `yaml:"ip"`              // 远程主机 IP（host 类型）
	Port           int    `yaml:"port"`            // 远程主机端口（host 类型）
	WorkPath       string `yaml:"path"`            // 存储路径
	Username       string `yaml:"username"`        // 认证用户名
	Password       string `yaml:"password"`        // 认证密码
	Endpoint       string `yaml:"endpoint"`        // S3 端点（s3 类型）
	ServerURL      string `yaml:"server_url"`      // WebDAV 服务器 URL（webdav 类型）
	RetentionCount int    `yaml:"retention_count"` // 保留文件数量
}

// Load 从 YAML 文件加载配置并校验。
// 使用场景：应用启动时读取配置文件。
func Load(path string) (*Config, error) {
	if path == "" {
		return nil, errors.New("config path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config failed: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config failed: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate 校验配置完整性和合法性。
// 使用场景：加载配置后立即校验，避免运行时错误。
func (c *Config) Validate() error {
	if c.Application.Concurrency <= 0 {
		return errors.New("concurrency must be greater than 0")
	}
	if c.Application.Database.DBType == "" {
		return errors.New("database db_type is required")
	}
	if c.Application.WorkPath == "" {
		return errors.New("application work_path is required")
	}
	if c.Application.Log.Dir == "" {
		return errors.New("application log.dir is required")
	}
	if c.Application.Database.FilePath == "" {
		return errors.New("database file_path is required")
	}
	if len(c.Application.Snapshots) == 0 {
		return errors.New("at least one snapshot is required")
	}

	idSet := make(map[string]struct{}, len(c.Application.Snapshots))
	for i, b := range c.Application.Snapshots {
		if b.ID == "" {
			return fmt.Errorf("snapshot[%d] id is required", i)
		}
		if _, exists := idSet[b.ID]; exists {
			return fmt.Errorf("snapshot id must be unique: %s", b.ID)
		}
		idSet[b.ID] = struct{}{}
		if b.DBType == "" {
			return fmt.Errorf("snapshot[%s] db_type is required", b.ID)
		}
		if b.IP == "" {
			return fmt.Errorf("snapshot[%s] ip is required", b.ID)
		}
		if b.Port <= 0 {
			return fmt.Errorf("snapshot[%s] port must be greater than 0", b.ID)
		}
		if b.Schema == "" {
			return fmt.Errorf("snapshot[%s] schema is required", b.ID)
		}

		for j, st := range b.Storages {
			if err := validateStorage(b.ID, j, st); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateStorage 校验单个存储配置的合法性。
// 使用场景：根据存储类型校验必填字段。
func validateStorage(snapshotID string, index int, st StorageSpec) error {
	t := strings.ToLower(strings.TrimSpace(st.Type))
	if st.ID == "" {
		return fmt.Errorf("snapshot[%s] storage[%d] id is required", snapshotID, index)
	}
	if st.WorkPath == "" {
		return fmt.Errorf("snapshot[%s] storage[%s] path is required", snapshotID, st.ID)
	}

	switch t {
	case "", "local":
		return nil
	case "host":
		if st.IP == "" {
			return fmt.Errorf("snapshot[%s] storage[%s] host ip is required", snapshotID, st.ID)
		}
		if st.Port <= 0 {
			return fmt.Errorf("snapshot[%s] storage[%s] host port is required", snapshotID, st.ID)
		}
		if st.Username == "" {
			return fmt.Errorf("snapshot[%s] storage[%s] host username is required", snapshotID, st.ID)
		}
	case "s3":
		if st.Endpoint == "" {
			return fmt.Errorf("snapshot[%s] storage[%s] s3 endpoint is required", snapshotID, st.ID)
		}
		if st.Username == "" {
			return fmt.Errorf("snapshot[%s] storage[%s] s3 username is required", snapshotID, st.ID)
		}
	case "webdav":
		if st.ServerURL == "" {
			return fmt.Errorf("snapshot[%s] storage[%s] webdav server_url is required", snapshotID, st.ID)
		}
	default:
		return fmt.Errorf("snapshot[%s] storage[%s] unsupported type: %s", snapshotID, st.ID, t)
	}
	return nil
}
