package compress

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// TarFiles 将多个文件打包为 tar 归档。
// 只保留文件名，不保留目录结构。
func TarFiles(tarPath string, files []string) error {
	dst, err := os.Create(tarPath)
	if err != nil {
		return fmt.Errorf("create tar file failed: %w", err)
	}
	defer dst.Close()

	tw := tar.NewWriter(dst)
	defer tw.Close()

	for _, f := range files {
		if err := addFileToTar(tw, f); err != nil {
			return err
		}
	}
	return nil
}

func addFileToTar(tw *tar.Writer, filePath string) error {
	fi, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("stat file failed: %w", err)
	}

	header := &tar.Header{
		Name: filepath.Base(filePath),
		Size: fi.Size(),
		Mode: int64(fi.Mode()),
	}

	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("write tar header failed: %w", err)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file failed: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("write tar body failed: %w", err)
	}
	return nil
}
