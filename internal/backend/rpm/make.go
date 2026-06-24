package rpm

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/fanhuadesenlinnn/RepoForge/internal/backend"
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

	// Collect input package files from configured package_dirs.
	inputPkgs, err := backend.CollectPackageFiles(profile.Input.PackageDirs, ".rpm", profile.Input.Recursive)
	if err != nil {
		return err
	}

	// Require at least one source of packages.
	if len(packages) == 0 && len(inputPkgs) == 0 {
		return fmt.Errorf("profile %q 没有配置软件包，也没有找到输入 RPM 文件", profile.Profile)
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

	// Copy input packages into the repository package directory.
	repoPkgs, err := backend.CopyPackagesToRepo(inputPkgs, profile.Repository.PackageDir)
	if err != nil {
		return err
	}

	manager, err := detect.FindAny(b.runner, "dnf", "yum")
	if err != nil {
		return err
	}
	args := rpmDownloadArgs(profile, packages, repoPkgs)
	if _, err := b.runner.Run(ctx, executor.Command{
		Name:        manager,
		Args:        args,
		Timeout:     2 * time.Hour,
		ProgressOut: os.Stdout,
		ProgressErr: os.Stderr,
	}); err != nil {
		return fmt.Errorf("下载 RPM 软件包及依赖失败: %w", err)
	}

	return b.generateMetadata(ctx, profile)
}

func (b *Backend) generateMetadata(ctx context.Context, profile *config.ProfileConfig) error {
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

func rpmDownloadArgs(profile *config.ProfileConfig, packages []string, rpmFiles []string) []string {
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
	args = append(args, packages...)
	args = append(args, rpmFiles...)
	return args
}
