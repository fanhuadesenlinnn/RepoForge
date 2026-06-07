package backend

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CollectPackageFiles scans directories for files matching the given suffix (e.g. ".rpm", ".deb").
func CollectPackageFiles(dirs []string, suffix string, recursive bool) ([]string, error) {
	if len(dirs) == 0 {
		return nil, nil
	}
	var results []string
	seen := make(map[string]struct{})
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("输入目录不存在: %s", dir)
			}
			return nil, fmt.Errorf("读取输入目录失败 %s: %w", dir, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("输入路径不是目录: %s", dir)
		}
		if recursive {
			if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return fmt.Errorf("扫描路径失败 %s: %w", path, err)
				}
				if !info.Mode().IsRegular() || !strings.HasSuffix(path, suffix) {
					return nil
				}
				if _, ok := seen[path]; ok {
					return nil
				}
				seen[path] = struct{}{}
				results = append(results, path)
				return nil
			}); err != nil {
				return nil, err
			}
		} else {
			entries, err := os.ReadDir(dir)
			if err != nil {
				return nil, fmt.Errorf("读取目录失败 %s: %w", dir, err)
			}
			for _, entry := range entries {
				if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), suffix) {
					continue
				}
				path := filepath.Join(dir, entry.Name())
				if _, ok := seen[path]; ok {
					continue
				}
				seen[path] = struct{}{}
				results = append(results, path)
			}
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("输入目录中未找到任何 %s 文件: %v", suffix, dirs)
	}
	sort.Strings(results)
	return results, nil
}

// CopyPackagesToRepo copies package files into a repository directory.
// It returns the destination paths. Skips when destination exists with same size.
func CopyPackagesToRepo(files []string, repoDir string) ([]string, error) {
	var results []string
	for _, src := range files {
		dst := filepath.Join(repoDir, filepath.Base(src))

		srcInfo, err := os.Stat(src)
		if err != nil {
			return nil, fmt.Errorf("读取源文件失败 %s: %w", src, err)
		}

		if dstInfo, err := os.Stat(dst); err == nil {
			if dstInfo.Size() == srcInfo.Size() {
				results = append(results, dst)
				continue
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("检查目标文件失败 %s: %w", dst, err)
		}

		if err := copyFileContent(src, dst); err != nil {
			return nil, fmt.Errorf("复制文件失败 %s -> %s: %w", src, dst, err)
		}
		results = append(results, dst)
	}
	return results, nil
}

func copyFileContent(src, dst string) error {
	srcF, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcF.Close()

	dstF, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstF.Close()

	_, err = io.Copy(dstF, srcF)
	return err
}
