package initialize

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fanhuadesenlinnn/RepoForge/internal/fileutil"
	"github.com/fanhuadesenlinnn/RepoForge/templates"
)

var directories = []string{
	"bin",
	"config/templates",
	"repos",
	"cache",
	"client",
	"logs",
}

// Run initializes a self-contained RepoForge home.
func Run(home string, force bool) error {
	for _, relative := range directories {
		if err := fileutil.EnsureDir(filepath.Join(home, relative), 0o755); err != nil {
			return err
		}
	}
	if err := fileutil.WriteFile(filepath.Join(home, ".repoforge-home"), []byte("RepoForge home\n"), 0o644, false); err != nil {
		return err
	}
	for _, asset := range templates.Assets() {
		data, err := templates.Read(asset.Source)
		if err != nil {
			return fmt.Errorf("读取内置模板 %s 失败: %w", asset.Source, err)
		}
		if err := fileutil.WriteFile(filepath.Join(home, asset.Destination), data, os.FileMode(asset.Mode), force); err != nil {
			return err
		}
	}
	return nil
}
