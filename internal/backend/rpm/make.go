package rpm

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/detect"
	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
	"github.com/fanhuadesenlinnn/RepoForge/internal/fileutil"
)

// Make downloads RPM packages and regenerates repository metadata.
func (b *Backend) Make(ctx context.Context, cfg *config.Config, profile *config.ProfileConfig, packages []string) error {
	if err := b.Check(ctx, profile); err != nil {
		return err
	}
	if len(packages) == 0 {
		return fmt.Errorf("profile %q 的软件包列表为空", profile.Profile)
	}
	if err := fileutil.EnsureDir(profile.Repository.PackageDir, 0o755); err != nil {
		return err
	}
	if profile.Online.CleanInstallrootBeforeMake {
		if err := fileutil.RemoveAllWithin(cfg.Paths.CacheDir, profile.Online.Installroot); err != nil {
			return err
		}
	}
	if err := fileutil.EnsureDir(profile.Online.Installroot, 0o755); err != nil {
		return err
	}

	manager, err := detect.FindAny(b.runner, "dnf", "yum")
	if err != nil {
		return err
	}
	args := rpmDownloadArgs(profile, packages)
	if _, err := b.runner.Run(ctx, executor.Command{
		Name:        manager,
		Args:        args,
		Timeout:     2 * time.Hour,
		ProgressOut: os.Stdout,
		ProgressErr: os.Stderr,
	}); err != nil {
		return fmt.Errorf("下载 RPM 软件包及依赖失败: %w", err)
	}

	tool := profile.Repository.MetadataTool
	if tool == "" {
		tool = "createrepo_c"
	}
	metadataArgs := []string{profile.Repository.PackageDir}
	if profile.Repository.CreaterepoUpdate {
		metadataArgs = append([]string{"--update"}, metadataArgs...)
	}
	if _, err := b.runner.Run(ctx, executor.Command{
		Name:    tool,
		Args:    metadataArgs,
		Timeout: time.Hour,
	}); err != nil {
		return fmt.Errorf("生成 RPM 软件源索引失败: %w", err)
	}
	return b.VerifyRepo(profile)
}

func rpmDownloadArgs(profile *config.ProfileConfig, packages []string) []string {
	args := []string{"--installroot=" + profile.Online.Installroot}
	if profile.Online.Releasever != "" {
		args = append(args, "--releasever="+profile.Online.Releasever)
	}
	for _, name := range profile.Online.DisableRepos {
		args = append(args, "--disablerepo="+name)
	}
	for _, name := range profile.Online.EnableRepos {
		args = append(args, "--enablerepo="+name)
	}
	args = append(args,
		"install",
		"-y",
		"--downloadonly",
		"--downloaddir", profile.Repository.PackageDir,
		fmt.Sprintf("--setopt=install_weak_deps=%t", profile.Online.IncludeWeakDeps),
	)
	return append(args, packages...)
}
