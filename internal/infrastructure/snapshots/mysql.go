package snapshots

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// MySQLAdapter 是 MySQL 数据库备份适配器。
// 使用场景：对 MySQL 数据库执行 mysqldump 导出。
type MySQLAdapter struct{}

// Type 返回适配器类型标识。
func (a *MySQLAdapter) Type() string { return "mysql" }

// Snapshots 执行 MySQL 备份。
// 主要逻辑：调用 mysqldump 导出 SQL 文件，然后压缩并计算哈希。
func (a *MySQLAdapter) Snapshots(ctx context.Context, req SnapshotsRequest) (SnapshotsResult, error) {
	spec := req.Spec
	fileName := buildFileName(spec.DBType, spec.IP, spec.Port, spec.Schema, "sql")
	outPath, err := joinPath(req.WorkPath, fileName)
	if err != nil {
		return SnapshotsResult{}, err
	}

	cmdPath := spec.CmdPath
	if cmdPath == "" {
		cmdPath = "mysqldump"
	}

	args := []string{
		"-h", spec.IP,
		"-P", fmt.Sprintf("%d", spec.Port),
		"-u", spec.Username,
		fmt.Sprintf("-p%s", spec.Password),
		"--default-character-set=utf8mb4",
		"--single-transaction",
		"--routines",
		"--triggers",
		"--no-tablespaces",
		"--complete-insert",
		"--add-drop-table",
		"--hex-blob",
		"--databases", spec.Schema,
	}

	cmd := exec.CommandContext(ctx, cmdPath, args...)
	if err := runCommand(cmd, outPath, stdoutToFile); err != nil {
		return SnapshotsResult{}, err
	}

	compressedPath, hash, err := tarCompressAndHash(outPath)
	if err != nil {
		return SnapshotsResult{}, err
	}

	return SnapshotsResult{FilePath: compressedPath, FileHash: strings.ToLower(hash)}, nil
}
