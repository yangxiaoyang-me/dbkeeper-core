package storages

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dbkeeper-core/internal/config"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// HostAdapter 是基于 SFTP 协议的远端存储适配器。
// 通过 SSH 连接远程主机，使用 SFTP 协议上传、列出和删除备份文件。
// 使用场景：将备份文件上传到远端 Linux 服务器，如 NAS、文件服务器等。
type HostAdapter struct{}

// Type 返回适配器类型标识 "host"。
func (a *HostAdapter) Type() string { return "host" }

// Upload 通过 SFTP 上传本地备份文件到远端主机。
// 主要逻辑：建立 SFTP 连接、创建远端目录、读取本地文件并写入远端。
// 使用场景：备份文件压缩完成后，上传到远端服务器保存。
func (a *HostAdapter) Upload(ctx context.Context, req UploadRequest) (UploadResult, error) {
	client, err := connectSFTP(req.Storage)
	if err != nil {
		return UploadResult{}, err
	}
	defer client.Close()

	storageDir := strings.TrimRight(normalizeSlashPath(req.Storage.WorkPath), "/")
	if storageDir == "" {
		storageDir = "/"
	}
	if err := client.MkdirAll(storageDir); err != nil {
		return UploadResult{}, fmt.Errorf("创建远端目录失败: %w", err)
	}

	fileName := filepath.Base(req.LocalFile)
	storagePath := path.Join(storageDir, fileName)

	src, err := os.Open(req.LocalFile)
	if err != nil {
		return UploadResult{}, fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer src.Close()

	dst, err := client.Create(storagePath)
	if err != nil {
		return UploadResult{}, fmt.Errorf("创建远端文件失败: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return UploadResult{}, fmt.Errorf("上传文件失败: %w", err)
	}

	return UploadResult{StorageWorkDir: storageDir, FileName: fileName, FileHash: req.LocalFileHash}, nil
}

// List 列出远端主机指定目录下的文件。
// 主要逻辑：通过 SFTP 读取目录内容，过滤前缀，按修改时间降序排列。
// 使用场景：保留策略执行时，列出远端已有的备份文件。
func (a *HostAdapter) List(ctx context.Context, req ListRequest) ([]StorageFile, error) {
	client, err := connectSFTP(req.Storage)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	storageDir := strings.TrimRight(normalizeSlashPath(req.Storage.WorkPath), "/")
	if storageDir == "" {
		storageDir = "/"
	}
	entries, err := client.ReadDir(storageDir)
	if err != nil {
		return nil, fmt.Errorf("读取远端目录失败: %w", err)
	}

	var files []StorageFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if req.Prefix != "" && !strings.HasPrefix(e.Name(), req.Prefix) {
			continue
		}
		files = append(files, StorageFile{Name: e.Name(), ModTime: e.ModTime().Unix()})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].ModTime > files[j].ModTime })
	return files, nil
}

// Delete 删除远端主机上的指定文件。
// 使用场景：保留策略清理过期备份文件时调用。
func (a *HostAdapter) Delete(ctx context.Context, req DeleteRequest) error {
	client, err := connectSFTP(req.Storage)
	if err != nil {
		return err
	}
	defer client.Close()

	storageDir := strings.TrimRight(normalizeSlashPath(req.Storage.WorkPath), "/")
	if storageDir == "" {
		storageDir = "/"
	}
	storagePath := path.Join(storageDir, req.Name)
	if err := client.Remove(storagePath); err != nil {
		return fmt.Errorf("删除远端文件失败: %w", err)
	}
	return nil
}

// connectSFTP 建立 SFTP 连接。
// 主要逻辑：使用密码认证建立 SSH 连接，然后创建 SFTP 客户端。
// 注意：使用 InsecureIgnoreHostKey 跳过主机密钥验证（适用于内网环境）。
// 使用场景：所有 SFTP 操作前建立连接。
func connectSFTP(spec config.StorageSpec) (*sftp.Client, error) {
	cfg := &ssh.ClientConfig{
		User:            spec.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(spec.Password)},
		Timeout:         10 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := fmt.Sprintf("%s:%d", spec.IP, spec.Port)
	conn, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("连接远端主机失败: %w", err)
	}
	client, err := sftp.NewClient(conn)
	if err != nil {
		return nil, fmt.Errorf("创建SFTP客户端失败: %w", err)
	}
	return client, nil
}
