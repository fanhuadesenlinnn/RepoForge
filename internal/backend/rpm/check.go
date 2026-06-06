package rpm

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
)

// VerifyRepo checks the RPM metadata entry point.
func (b *Backend) VerifyRepo(profile *config.ProfileConfig) error {
	index := filepath.Join(profile.Repository.PackageDir, "repodata", "repomd.xml")
	if _, err := os.Stat(index); err != nil {
		return fmt.Errorf("RPM 软件源索引不可用 %s: %w\n\n解决建议：请先执行 repoforge make --profile %s", index, err, profile.Profile)
	}
	return nil
}
