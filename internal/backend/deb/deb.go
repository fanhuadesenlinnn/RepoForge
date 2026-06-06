package deb

import (
	"context"
	"fmt"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
)

// Backend implements DEB repository operations.
type Backend struct {
	runner executor.Runner
}

// New returns a DEB backend.
func New(runner executor.Runner) *Backend {
	return &Backend{runner: runner}
}

func (b *Backend) Name() string {
	return "deb"
}

// Check verifies the DEB toolchain.
func (b *Backend) Check(_ context.Context, profile *config.ProfileConfig) error {
	for _, name := range []string{"apt-get", "apt-cache", "gzip"} {
		if _, err := b.runner.LookPath(name); err != nil {
			return fmt.Errorf("未找到 %s 命令\n\n解决建议：请先安装 %s", name, name)
		}
	}
	tool := profile.Repository.MetadataTool
	if tool == "" {
		tool = "dpkg-scanpackages"
	}
	if _, err := b.runner.LookPath(tool); err != nil {
		return fmt.Errorf(`未找到 %s 命令

解决建议：
1. 请先安装 dpkg-dev；
2. Debian/Ubuntu 可执行：
   apt-get install dpkg-dev`, tool)
	}
	return nil
}
