package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnsureDir creates a directory and its parents when needed.
func EnsureDir(path string, mode os.FileMode) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return fmt.Errorf("创建目录 %s 失败: %w", path, err)
	}
	return nil
}

// WriteFile writes a managed file. Existing files are preserved unless force is true.
func WriteFile(path string, data []byte, mode os.FileMode, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("检查文件 %s 失败: %w", path, err)
		}
	}
	if err := EnsureDir(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("创建临时文件 %s 失败: %w", path, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return fmt.Errorf("设置文件权限 %s 失败: %w", path, err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("写入文件 %s 失败: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭文件 %s 失败: %w", path, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("更新文件 %s 失败: %w", path, err)
	}
	return nil
}
