package storages

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"dbkeeper-core/internal/config"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Adapter 是 S3/MinIO 存储适配器。
// 使用场景：将备份文件上传到 S3 兼容对象存储。
type S3Adapter struct{}

// Type 返回适配器类型标识。
func (a *S3Adapter) Type() string { return "s3" }

// Upload 上传本地备份文件到 S3/MinIO。
// 主要逻辑：连接 S3，上传文件到指定 bucket 和路径。
func (a *S3Adapter) Upload(ctx context.Context, req UploadRequest) (UploadResult, error) {
	client, bucket, prefix, err := connectS3(req.Storage)
	if err != nil {
		return UploadResult{}, err
	}

	fileName := filepath.Base(req.LocalFile)
	objectName := path.Join(prefix, fileName)

	_, err = client.FPutObject(ctx, bucket, objectName, req.LocalFile, minio.PutObjectOptions{})
	if err != nil {
		return UploadResult{}, fmt.Errorf("上传对象失败: %w", err)
	}

	return UploadResult{StorageWorkDir: req.Storage.WorkPath, FileName: fileName, FileHash: req.LocalFileHash}, nil
}

// List 列出 S3/MinIO 中的对象。
// 使用场景：保留策略时列出所有备份文件。
func (a *S3Adapter) List(ctx context.Context, req ListRequest) ([]StorageFile, error) {
	client, bucket, prefix, err := connectS3(req.Storage)
	if err != nil {
		return nil, err
	}

	if req.Prefix != "" {
		prefix = path.Join(prefix, req.Prefix)
	}

	var files []StorageFile
	for obj := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("列出对象失败: %w", obj.Err)
		}
		files = append(files, StorageFile{Name: obj.Key, ModTime: obj.LastModified.Unix()})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].ModTime > files[j].ModTime })
	return files, nil
}

// Delete 删除 S3/MinIO 中的对象。
// 使用场景：保留策略清理过期备份。
func (a *S3Adapter) Delete(ctx context.Context, req DeleteRequest) error {
	client, bucket, prefix, err := connectS3(req.Storage)
	if err != nil {
		return err
	}

	objectName := req.Name
	if !strings.Contains(req.Name, "/") {
		objectName = path.Join(prefix, req.Name)
	}
	if err := client.RemoveObject(ctx, bucket, objectName, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("删除对象失败: %w", err)
	}
	return nil
}

// connectS3 创建 S3 客户端并解析 bucket 和 prefix。
// 主要逻辑：解析 endpoint 协议，从 path 中提取 bucket 和 prefix。
// 使用场景：所有 S3 操作前建立连接。
func connectS3(spec config.StorageSpec) (*minio.Client, string, string, error) {
	endpoint := strings.TrimSpace(spec.Endpoint)
	if endpoint == "" {
		return nil, "", "", fmt.Errorf("S3 endpoint 不能为空")
	}
	secure := false
	if strings.HasPrefix(endpoint, "https://") {
		secure = true
		endpoint = strings.TrimPrefix(endpoint, "https://")
	}
	if strings.HasPrefix(endpoint, "http://") {
		endpoint = strings.TrimPrefix(endpoint, "http://")
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(spec.Username, spec.Password, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, "", "", fmt.Errorf("创建S3客户端失败: %w", err)
	}

	work := strings.Trim(normalizeSlashPath(spec.WorkPath), "/")
	parts := strings.SplitN(work, "/", 2)
	bucket := parts[0]
	prefix := ""
	if len(parts) == 2 {
		prefix = parts[1]
	}
	if bucket == "" {
		return nil, "", "", fmt.Errorf("S3 path 必须包含 bucket")
	}

	return client, bucket, prefix, nil
}
