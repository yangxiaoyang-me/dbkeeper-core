package snapshots

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"unicode/utf8"

	"dbkeeper-core/internal/appmeta"
	"dbkeeper-core/internal/infrastructure/compress"
	"dbkeeper-core/internal/infrastructure/hash"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// stdoutMode 定义命令输出模式。
// 使用场景：控制备份命令的输出重定向方式。
type stdoutMode int

const (
	stdoutToFile   stdoutMode = iota // 输出到文件（mysqldump）
	stdoutToLog                      // 输出到日志（pg_dump）
	stdoutToLogGBK                   // 输出到日志并转码 GBK（dexp）
)

// runCommand 执行备份命令并记录日志。
// 主要逻辑：执行命令、记录开始/结束时间、处理输出编码。
// 使用场景：所有数据库备份命令的统一执行入口。
func runCommand(cmd *exec.Cmd, outPath string, mode stdoutMode) error {
	logPath := outPath + ".log"
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("create log file failed: %w", err)
	}
	defer logFile.Close()

	start := time.Now()
	if _, err := fmt.Fprintf(logFile, "start: %s\ncmd: %s\n", start.Format(time.RFC3339), sanitizeCommand(cmd)); err != nil {
		return fmt.Errorf("write log failed: %w", err)
	}

	var outFile *os.File
	if mode == stdoutToFile {
		outFile, err = os.Create(outPath)
		if err != nil {
			return fmt.Errorf("create output file failed: %w", err)
		}
		defer outFile.Close()
		cmd.Stdout = outFile
		cmd.Stderr = logFile
	} else if mode == stdoutToLog {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	} else {
		var combined bytes.Buffer
		cmd.Stdout = &combined
		cmd.Stderr = &combined
		err = cmd.Run()

		reader := transform.NewReader(bytes.NewReader(combined.Bytes()), simplifiedchinese.GBK.NewDecoder())
		if utf8.Valid(combined.Bytes()) {
			_, _ = logFile.Write(combined.Bytes())
		} else if _, copyErr := io.Copy(logFile, reader); copyErr != nil {
			_, _ = logFile.Write(combined.Bytes())
		}

		finish := time.Now()
		if _, wErr := fmt.Fprintf(logFile, "\nend: %s\nduration: %.3fs\n", finish.Format(time.RFC3339), finish.Sub(start).Seconds()); wErr != nil {
			return fmt.Errorf("write log failed: %w", wErr)
		}
		if err != nil {
			_, _ = fmt.Fprintf(logFile, "status: failed\nerror: %v\n", err)
			return fmt.Errorf("run export command failed: %w, log: %s", err, logPath)
		}
		_, _ = fmt.Fprintln(logFile, "status: success")
		return nil
	}

	err = cmd.Run()
	finish := time.Now()
	if _, wErr := fmt.Fprintf(logFile, "\nend: %s\nduration: %.3fs\n", finish.Format(time.RFC3339), finish.Sub(start).Seconds()); wErr != nil {
		return fmt.Errorf("write log failed: %w", wErr)
	}
	if err != nil {
		_, _ = fmt.Fprintf(logFile, "status: failed\nerror: %v\n", err)
		return fmt.Errorf("run export command failed: %w, log: %s", err, logPath)
	}
	_, _ = fmt.Fprintln(logFile, "status: success")
	return nil
}

// sanitizeCommand 脱敏命令参数中的密码。
// 使用场景：记录日志时隐藏敏感信息。
func sanitizeCommand(cmd *exec.Cmd) string {
	if cmd == nil {
		return ""
	}
	args := make([]string, 0, len(cmd.Args))
	for _, arg := range cmd.Args {
		switch {
		case strings.HasPrefix(arg, "-p") && len(arg) > 2:
			args = append(args, "-p******")
		case strings.HasPrefix(strings.ToLower(arg), "userid="):
			args = append(args, sanitizeUserIDArg(arg))
		default:
			args = append(args, arg)
		}
	}
	return strings.Join(args, " ")
}

// sanitizeUserIDArg 脱敏达梦数据库的 userid 参数。
// 使用场景：隐藏 userid=user/password@host 中的密码部分。
func sanitizeUserIDArg(arg string) string {
	parts := strings.SplitN(arg, "=", 2)
	if len(parts) != 2 {
		return arg
	}
	raw := parts[1]
	at := strings.Index(raw, "@")
	slash := strings.Index(raw, "/")
	if slash == -1 || at == -1 || slash > at {
		return "userid=******"
	}
	user := raw[:slash]
	addr := raw[at+1:]
	return fmt.Sprintf("userid=%s/******@%s", user, addr)
}

// tarCompressAndHash 将导出文件和日志打包为 tar，再压缩为 tar.zst，并计算哈希。
// 主要逻辑：收集导出文件和日志 → tar 打包 → zstd 压缩 → SHA256 哈希 → 清理中间文件。
// 使用场景：备份文件导出后立即打包压缩。
func tarCompressAndHash(outPath string) (string, string, error) {
	logPath := outPath + ".log"

	// 收集需要打包的文件
	files := []string{outPath}
	if _, err := os.Stat(logPath); err == nil {
		files = append(files, logPath)
	}

	// tar 路径：去掉原扩展名，替换为 .tar
	basePath := strings.TrimSuffix(outPath, filepath.Ext(outPath))
	tarPath := basePath + ".tar"

	if err := compress.TarFiles(tarPath, files); err != nil {
		return "", "", err
	}

	// 压缩 tar → tar.zst
	zstPath := tarPath + ".zst"
	if err := compress.ZstdFile(tarPath, zstPath); err != nil {
		return "", "", err
	}

	sum, err := hash.SHA256File(zstPath)
	if err != nil {
		return "", "", err
	}

	// 清理中间文件
	_ = os.Remove(outPath)
	_ = os.Remove(logPath)
	_ = os.Remove(tarPath)

	return zstPath, sum, nil
}

// buildFileName 构建备份文件名。
// 格式：dbtype_ip_port_schema_version_timestamp.ext
// 使用场景：生成唯一且可识别的备份文件名。
func buildFileName(dbType, ip string, port int, schema, ext string) string {
	ts := time.Now().Format("20060102150405")
	return fmt.Sprintf("%s_%s_%d_%s_%s_%s.%s", dbType, ip, port, schema, appmeta.Version, ts, ext)
}

// joinPath 创建工作目录并拼接文件路径。
// 使用场景：确保备份目录存在。
func joinPath(workPath, fileName string) (string, error) {
	if err := os.MkdirAll(workPath, 0o755); err != nil {
		return "", fmt.Errorf("create work dir failed: %w", err)
	}
	return filepath.Join(workPath, fileName), nil
}
