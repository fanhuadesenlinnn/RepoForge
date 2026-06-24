package deb

import (
	"context"
	"fmt"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
)

// MakeUpgrade is not implemented for DEB repositories yet.
func (b *Backend) MakeUpgrade(_ context.Context, _ *config.Config, profile *config.ProfileConfig) error {
	return fmt.Errorf("make-upgrade 目前仅支持 RPM backend，当前 profile %q 使用 backend: %s", profile.Profile, profile.Backend)
}
