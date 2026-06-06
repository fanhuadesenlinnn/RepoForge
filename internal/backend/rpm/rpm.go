package rpm

import (
	"context"
	"fmt"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/detect"
	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
)

// Backend implements RPM repository operations.
type Backend struct {
	runner executor.Runner
}

// New returns an RPM backend.
func New(runner executor.Runner) *Backend {
	return &Backend{runner: runner}
}

func (b *Backend) Name() string {
	return "rpm"
}

// Check verifies the RPM toolchain.
func (b *Backend) Check(_ context.Context, profile *config.ProfileConfig) error {
	if _, err := detect.FindAny(b.runner, "dnf", "yum"); err != nil {
		return fmt.Errorf("未找到 dnf 或 yum 命令\n\n解决建议：请先安装可用的 RPM 包管理器")
	}
	if _, err := b.runner.LookPath("rpm"); err != nil {
		return fmt.Errorf("未找到 rpm 命令\n\n解决建议：请先安装 rpm")
	}
	tool := profile.Repository.MetadataTool
	if tool == "" {
		tool = "createrepo_c"
	}
	if _, err := b.runner.LookPath(tool); err != nil {
		return fmt.Errorf(`未找到 %s 命令

解决建议：
1. 请先在在线机器上安装 createrepo_c；
2. 麒麟/RHEL 系系统可尝试：
   dnf install createrepo_c
   或：
   yum install createrepo_c`, tool)
	}
	return nil
}
