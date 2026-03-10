// Package compress 提供文件压缩功能。
//
// 当前支持 Zstandard (zstd) 压缩算法，具有高压缩比和高速度的特点。
// 使用场景：数据库导出文件通常较大，压缩后可显著减少存储空间和上传时间。
package compress

import (
	"fmt"
	"io"
	"os"

	"github.com/klauspost/compress/zstd"
)

// ZstdFile 将源文件压缩为 Zstandard (.zst) 格式并写入目标路径。
// 主要逻辑：以流式方式读取源文件并压缩写入，不会将整个文件加载到内存。
// 使用场景：数据库导出完成后立即压缩，减少后续上传和存储的体积。
// 典型压缩比：SQL 文件约 5:1~10:1，二进制 dump 文件约 2:1~5:1。
func ZstdFile(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("创建压缩文件失败: %w", err)
	}
	defer dst.Close()

	encoder, err := zstd.NewWriter(dst)
	if err != nil {
		return fmt.Errorf("创建Zstd编码器失败: %w", err)
	}
	defer encoder.Close()

	if _, err := io.Copy(encoder, src); err != nil {
		return fmt.Errorf("压缩文件失败: %w", err)
	}
	return nil
}
