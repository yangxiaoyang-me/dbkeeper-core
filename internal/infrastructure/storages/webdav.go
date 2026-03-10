package storages

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"dbkeeper-core/internal/config"

	"github.com/studio-b12/gowebdav"
)

// WebDAVAdapter 是基于 WebDAV 协议的远端存储适配器。
// 通过 WebDAV 客户端上传、列出和删除备份文件。
// 使用场景：将备份文件上传到支持 WebDAV 的存储服务（如坚果云、Nextcloud、群晖 WebDAV 等）。
type WebDAVAdapter struct{}

// Type 返回适配器类型标识 "webdav"。
func (a *WebDAVAdapter) Type() string { return "webdav" }

// Upload 通过 WebDAV 上传本地备份文件到远端存储。
// 主要逻辑：创建 WebDAV 客户端、创建远端目录、以流式方式写入文件。
// 使用场景：备份文件压缩完成后，上传到 WebDAV 服务保存。
func (a *WebDAVAdapter) Upload(ctx context.Context, req UploadRequest) (UploadResult, error) {
	client, workPath, err := connectWebDAV(req.Storage)
	if err != nil {
		return UploadResult{}, err
	}

	fileName := filepath.Base(req.LocalFile)
	storagePath := path.Join(workPath, fileName)

	file, err := os.Open(req.LocalFile)
	if err != nil {
		return UploadResult{}, fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer file.Close()

	if err := client.MkdirAll(workPath, 0o755); err != nil {
		return UploadResult{}, fmt.Errorf("创建WebDAV目录失败: %w", err)
	}

	if err := client.WriteStream(storagePath, file, 0o644); err != nil {
		return UploadResult{}, fmt.Errorf("上传WebDAV失败: %w", err)
	}

	return UploadResult{StorageWorkDir: workPath, FileName: fileName, FileHash: req.LocalFileHash}, nil
}

// List 列出 WebDAV 目录下的文件。
// 主要逻辑：读取远端目录内容，过滤前缀匹配的文件，按修改时间降序排列。
// 使用场景：保留策略执行时，列出远端已有的备份文件。
func (a *WebDAVAdapter) List(ctx context.Context, req ListRequest) ([]StorageFile, error) {
	client, workPath, err := connectWebDAV(req.Storage)
	if err != nil {
		return nil, err
	}

	entries, err := client.ReadDir(workPath)
	if err != nil {
		return nil, fmt.Errorf("读取WebDAV目录失败: %w", err)
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

// Delete 删除 WebDAV 目录中的指定文件。
// 使用场景：保留策略清理过期备份文件时调用。
func (a *WebDAVAdapter) Delete(ctx context.Context, req DeleteRequest) error {
	client, workPath, err := connectWebDAV(req.Storage)
	if err != nil {
		return err
	}

	storagePath := path.Join(workPath, req.Name)
	if err := client.Remove(storagePath); err != nil {
		return fmt.Errorf("删除WebDAV文件失败: %w", err)
	}
	return nil
}

// connectWebDAV 创建 WebDAV 客户端并规范化工作路径。
// 主要逻辑：使用配置中的 server_url、用户名和密码创建客户端。
// 使用场景：所有 WebDAV 操作前建立连接。
func connectWebDAV(spec config.StorageSpec) (*gowebdav.Client, string, error) {
	if spec.ServerURL == "" {
		return nil, "", fmt.Errorf("WebDAV server_url 不能为空")
	}
	client := gowebdav.NewClient(spec.ServerURL, spec.Username, spec.Password)
	workPath := strings.TrimRight(normalizeSlashPath(spec.WorkPath), "/")
	if workPath == "" {
		workPath = "/"
	}
	return client, workPath, nil
}
