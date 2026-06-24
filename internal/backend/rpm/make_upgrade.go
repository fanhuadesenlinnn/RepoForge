package rpm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/detect"
	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
	"github.com/fanhuadesenlinnn/RepoForge/internal/fileutil"
)

// MakeUpgrade downloads packages required to upgrade the current RPM system and regenerates repository metadata.
func (b *Backend) MakeUpgrade(ctx context.Context, cfg *config.Config, profile *config.ProfileConfig) error {
	if err := b.Check(ctx, profile); err != nil {
		return err
	}
	if err := fileutil.EnsureDir(profile.Repository.PackageDir, 0o755); err != nil {
		return err
	}

	manager, err := detect.FindAny(b.runner, "dnf", "yum")
	if err != nil {
		return err
	}
	args := rpmUpgradeDownloadArgs(manager, profile)
	if _, err := b.runner.Run(ctx, executor.Command{
		Name:        manager,
		Args:        args,
		Timeout:     2 * time.Hour,
		ProgressOut: os.Stdout,
		ProgressErr: os.Stderr,
	}); err != nil {
		return fmt.Errorf("下载当前系统升级所需 RPM 包失败: %w", err)
	}

	return b.generateMetadata(ctx, profile)
}

func rpmUpgradeDownloadArgs(manager string, profile *config.ProfileConfig) []string {
	args := []string{}
	if profile.Online.Releasever != "" {
		args = append(args, "--releasever="+profile.Online.Releasever)
	}
	for _, name := range profile.Online.DisableRepos {
		args = append(args, "--disablerepo="+name)
	}
	for _, name := range profile.Online.EnableRepos {
		args = append(args, "--enablerepo="+name)
	}

	action := "upgrade"
	if filepath.Base(manager) == "yum" {
		action = "update"
	}
	args = append(args,
		action,
		"-y",
		"--downloadonly",
		"--downloaddir", profile.Repository.PackageDir,
		fmt.Sprintf("--setopt=install_weak_deps=%t", profile.Online.IncludeWeakDeps),
	)
	return args
}
