// Package appmeta 存储应用的元信息（版本号和构建时间）。
//
// 版本号在代码中硬编码维护，构建时间可通过 go build -ldflags 注入。
// 使用场景：启动日志、通知消息和命令行 -version 参数中展示版本信息。
//
// 构建示例：
//
//	go build -ldflags "-X dbkeeper-core/internal/appmeta.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
package appmeta

// Version 是软件版本号（硬编码维护，每次发版时手动更新）。
const Version = "v0.2.0"

// BuildTime 是构建时间，默认 "unknown"。
// 可在 go build 时通过 -ldflags "-X dbkeeper-core/internal/appmeta.BuildTime=..." 注入实际构建时间。
var BuildTime = "unknown"
