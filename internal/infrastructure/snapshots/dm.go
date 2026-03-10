package snapshots

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DMAdapter 是达梦数据库备份适配器。
// 使用场景：对达梦数据库执行 dexp 导出。
type DMAdapter struct{}

// Type 返回适配器类型。
func (a *DMAdapter) Type() string { return "dm" }

// Snapshots 执行达梦备份。
// 主要逻辑：调用 dexp 导出并压缩计算哈希。
func (a *DMAdapter) Snapshots(ctx context.Context, req SnapshotsRequest) (SnapshotsResult, error) {
	spec := req.Spec
	fileName := buildFileName(spec.DBType, spec.IP, spec.Port, spec.Schema, "dmp")
	outPath, err := joinPath(req.WorkPath, fileName)
	if err != nil {
		return SnapshotsResult{}, err
	}

	cmdPath := spec.CmdPath
	if cmdPath == "" {
		cmdPath = "dexp"
	}

	userid := fmt.Sprintf("%s/%s@%s:%d", spec.Username, spec.Password, spec.IP, spec.Port)
	workDir := filepath.Dir(outPath)
	fileArg := filepath.Base(outPath)
	args := []string{
		fmt.Sprintf("userid=%s", userid),
		fmt.Sprintf("schemas=%s", spec.Schema),
		fmt.Sprintf("file=%s", fileArg),
	}

	cmd := exec.CommandContext(ctx, cmdPath, args...)
	cmd.Dir = workDir
	if filepath.IsAbs(cmdPath) {
		libDir := filepath.Dir(cmdPath)
		ldPath := os.Getenv("LD_LIBRARY_PATH")
		if ldPath != "" {
			ldPath = libDir + ":" + ldPath
		} else {
			ldPath = libDir
		}
		cmd.Env = append(os.Environ(), "LD_LIBRARY_PATH="+ldPath)
	}
	_ = os.Remove(filepath.Join(cmd.Dir, "dexp.log"))
	if err := runCommand(cmd, outPath, stdoutToLogGBK); err != nil {
		return SnapshotsResult{}, err
	}

	compressedPath, hash, err := tarCompressAndHash(outPath)
	if err != nil {
		return SnapshotsResult{}, err
	}

	return SnapshotsResult{FilePath: compressedPath, FileHash: strings.ToLower(hash)}, nil
}
