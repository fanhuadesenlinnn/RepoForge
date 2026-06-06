package backend

import (
	"context"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
)

// Backend builds, verifies, and exposes one repository format.
type Backend interface {
	Name() string
	Check(context.Context, *config.ProfileConfig) error
	Make(context.Context, *config.Config, *config.ProfileConfig, []string) error
	EnableLocalRepo(context.Context, *config.Config, *config.ProfileConfig) error
	DisableLocalRepo(context.Context, *config.Config, *config.ProfileConfig, bool) error
	GenerateClientRepo(*config.Config, *config.ProfileConfig, string) error
	VerifyRepo(*config.ProfileConfig) error
}
