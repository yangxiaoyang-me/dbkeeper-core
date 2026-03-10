// Package retention 提供备份文件的保留策略功能。
//
// 支持本地和远端两种保留策略：
//   - 本地保留：按文件修改时间排序，保留最新的 N 个文件，删除多余文件
//   - 远端保留：通过存储适配器列出远端文件，按修改时间排序后删除多余文件
//
// 保留策略按文件前缀分组（前缀格式：{db_type}_{ip}_{port}_{schema}_），
// 确保不同数据库的备份文件独立管理。
// 使用场景：每次备份完成后自动清理过期文件，防止磁盘/存储空间耗尽。
package retention

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dbkeeper-core/internal/config"
	"dbkeeper-core/internal/infrastructure/storages"
)

// Manager 是保留策略管理器，实现 application.RetentionPolicy 接口。
// 使用场景：备份上传完成后调用，清理本地工作目录和远端存储中的过期文件。
type Manager struct{}

// New 创建保留策略管理器实例。
func New() *Manager { return &Manager{} }

// ApplyLocal 执行本地保留策略。
// 主要逻辑：扫描工作目录中匹配前缀的文件（.zst 压缩文件和 .log 日志文件），
// 按修改时间降序排列，保留最新的 retentionCount 个文件，删除其余文件。
// 使用场景：备份完成后清理本地工作目录中的过期文件。
func (m *Manager) ApplyLocal(workPath string, retentionCount int, spec config.SnapshotsSpec) error {
	if retentionCount <= 0 {
		return nil
	}

	prefix := buildPrefix(spec)
	entries, err := os.ReadDir(workPath)
	if err != nil {
		return fmt.Errorf("读取本地目录失败: %w", err)
	}

	applyLocalRetention(workPath, entries, prefix, ".zst", retentionCount)
	applyLocalRetention(workPath, entries, prefix, ".log", retentionCount)
	return nil
}

// ApplyStorage 执行远端存储保留策略。
// 主要逻辑：通过存储适配器列出远端所有 .zst 文件，
// 按前缀过滤出当前数据库的文件，按修改时间降序排列，
// 保留最新的 RetentionCount 个文件，通过适配器删除多余文件。
// 使用场景：备份上传到远端存储后，自动清理远端的过期备份文件。
func (m *Manager) ApplyStorage(ctx context.Context, spec config.StorageSpec, adapter storages.Adapter, snapshotsSpec config.SnapshotsSpec) error {
	if spec.RetentionCount <= 0 {
		return nil
	}
	prefix := buildPrefix(snapshotsSpec)
	files, err := adapter.List(ctx, storages.ListRequest{Storage: spec, Prefix: ""})
	if err != nil {
		return err
	}

	filtered := make([]storages.StorageFile, 0, len(files))
	allZst := make([]storages.StorageFile, 0, len(files))
	for _, f := range files {
		base := path.Base(f.Name)
		if !strings.HasSuffix(base, ".zst") {
			continue
		}
		allZst = append(allZst, f)
		if !strings.HasPrefix(base, prefix) {
			continue
		}
		filtered = append(filtered, f)
	}

	candidates := filtered
	if len(candidates) == 0 {
		candidates = allZst
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ModTime > candidates[j].ModTime })
	if len(candidates) <= spec.RetentionCount {
		return nil
	}

	for _, f := range candidates[spec.RetentionCount:] {
		if err := adapter.Delete(ctx, storages.DeleteRequest{Storage: spec, Name: f.Name}); err != nil {
			return fmt.Errorf("删除远端备份失败(%s): %w", f.Name, err)
		}
	}
	return nil
}

// buildPrefix 构建文件前缀，用于匹配属于同一数据库的备份文件。
// 格式：{db_type}_{ip}_{port}_{schema}_
// 使用场景：保留策略按前缀分组，确保不同数据库的备份文件独立管理。
func buildPrefix(spec config.SnapshotsSpec) string {
	return fmt.Sprintf("%s_%s_%d_%s_", spec.DBType, spec.IP, spec.Port, spec.Schema)
}

// applyLocalRetention 按文件前缀和后缀筛选文件，保留最新的 keepCount 个，删除其余文件。
// 主要逻辑：过滤匹配前缀和后缀的文件，按修改时间降序排序，删除排名在 keepCount 之后的文件。
// 使用场景：ApplyLocal 内部调用，分别处理 .zst 和 .log 文件的保留。
func applyLocalRetention(workPath string, entries []os.DirEntry, prefix, suffix string, keepCount int) {
	var files []fileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{Name: name, ModTime: info.ModTime()})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].ModTime.After(files[j].ModTime) })
	if len(files) <= keepCount {
		return
	}
	for _, f := range files[keepCount:] {
		_ = os.Remove(filepath.Join(workPath, f.Name))
	}
}

// fileInfo 是本地文件的简要信息，用于保留策略排序。
type fileInfo struct {
	Name    string    // 文件名
	ModTime time.Time // 最后修改时间
}
