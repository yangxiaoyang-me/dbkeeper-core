// Package hash 提供文件哈希计算功能。
//
// 使用场景：备份文件压缩后计算 SHA256 哈希值，
// 用于文件完整性校验和元数据记录。
package hash

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

// SHA256File 计算指定文件的 SHA-256 哈希值，返回小写十六进制字符串。
// 主要逻辑：以流式方式读取文件内容并计算哈希，不会将整个文件加载到内存。
// 使用场景：备份文件压缩后调用，哈希值存入元数据库用于完整性校验。
func SHA256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open file failed: %w", err)
	}
	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", fmt.Errorf("compute file hash failed: %w", err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
