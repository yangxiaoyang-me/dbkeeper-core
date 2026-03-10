package snapshots

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// PGAdapter 是 PostgreSQL 备份适配器。
// 使用场景：对 PG 数据库执行 pg_dump 导出。
type PGAdapter struct{}

// Type 返回适配器类型。
func (a *PGAdapter) Type() string { return "pg" }

// Snapshots 执行 PG 备份。
// 主要逻辑：调用 pg_dump 导出并压缩计算哈希。
func (a *PGAdapter) Snapshots(ctx context.Context, req SnapshotsRequest) (SnapshotsResult, error) {
	spec := req.Spec
	fileName := buildFileName(spec.DBType, spec.IP, spec.Port, spec.Schema, "dump")
	outPath, err := joinPath(req.WorkPath, fileName)
	if err != nil {
		return SnapshotsResult{}, err
	}

	cmdPath := spec.CmdPath
	if cmdPath == "" {
		cmdPath = "pg_dump"
	}

	args := []string{
		"-h", spec.IP,
		"-p", fmt.Sprintf("%d", spec.Port),
		"-U", spec.Username,
		"-F", "c",
		"-f", outPath,
		spec.Schema,
	}

	cmd := exec.CommandContext(ctx, cmdPath, args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", spec.Password))

	if err := runCommand(cmd, outPath, stdoutToLog); err != nil {
		return SnapshotsResult{}, err
	}

	compressedPath, hash, err := tarCompressAndHash(outPath)
	if err != nil {
		return SnapshotsResult{}, err
	}

	return SnapshotsResult{FilePath: compressedPath, FileHash: strings.ToLower(hash)}, nil
}
