package deb

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
)

// VerifyRepo checks the DEB metadata entry point.
func (b *Backend) VerifyRepo(profile *config.ProfileConfig) error {
	index := filepath.Join(profile.Repository.PackageDir, "Packages.gz")
	if _, err := os.Stat(index); err != nil {
		return fmt.Errorf("DEB 软件源索引不可用 %s: %w\n\n解决建议：请先执行 repoforge make --profile %s", index, err, profile.Profile)
	}
	return nil
}
