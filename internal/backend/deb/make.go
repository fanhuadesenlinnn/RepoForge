package deb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/RepoForge/internal/backend"
	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
	"github.com/fanhuadesenlinnn/RepoForge/internal/fileutil"
)

// Make downloads DEB packages and regenerates Packages indexes.
func (b *Backend) Make(ctx context.Context, cfg *config.Config, profile *config.ProfileConfig, packages []string) error {
	if err := b.Check(ctx, profile); err != nil {
		return err
	}

	// Collect input package files from configured package_dirs.
	inputPkgs, err := backend.CollectPackageFiles(profile.Input.PackageDirs, ".deb", profile.Input.Recursive)
	if err != nil {
		return err
	}

	// Require at least one source of packages.
	if len(packages) == 0 && len(inputPkgs) == 0 {
		return fmt.Errorf("profile %q 没有配置软件包，也没有找到输入 DEB 文件", profile.Profile)
	}

	if err := prepareAPT(profile); err != nil {
		return err
	}

	// Copy input packages into the repository package directory.
	if _, err := backend.CopyPackagesToRepo(inputPkgs, profile.Repository.PackageDir); err != nil {
		return err
	}

	// Extract package names from input .deb files.
	debNames, err := debPackageNames(ctx, b.runner, inputPkgs)
	if err != nil {
		return err
	}

	// Merge configured packages and input .deb package names.
	allPackages := append([]string{}, packages...)
	allPackages = append(allPackages, debNames...)

	options := aptOptions(profile)
	if profile.Online.RunAPTUpdateBeforeMake {
		if _, err := b.runner.Run(ctx, executor.Command{
			Name:    "apt-get",
			Args:    append([]string{"update"}, options...),
			Timeout: time.Hour,
		}); err != nil {
			return fmt.Errorf("更新隔离 apt 软件包列表失败: %w", err)
		}
	}
	args := []string{"install", "--download-only", "--reinstall", "-y"}
	args = append(args, options...)
	args = append(args,
		"-o", fmt.Sprintf("APT::Install-Recommends=%t", profile.Online.IncludeRecommends),
		"-o", fmt.Sprintf("APT::Install-Suggests=%t", profile.Online.IncludeSuggests),
	)
	args = append(args, allPackages...)
	if _, err := b.runner.Run(ctx, executor.Command{
		Name:        "apt-get",
		Args:        args,
		Timeout:     2 * time.Hour,
		ProgressErr: os.Stderr,
	}); err != nil {
		return fmt.Errorf("下载 DEB 软件包及依赖失败: %w", err)
	}

	tool := profile.Repository.MetadataTool
	if tool == "" {
		tool = "dpkg-scanpackages"
	}
	index, err := b.runner.Run(ctx, executor.Command{
		Name:    tool,
		Args:    []string{".", "/dev/null"},
		Dir:     profile.Repository.PackageDir,
		Timeout: time.Hour,
	})
	if err != nil {
		return fmt.Errorf("生成 DEB Packages 索引失败: %w", err)
	}
	packagesPath := filepath.Join(profile.Repository.PackageDir, "Packages")
	if err := fileutil.WriteFile(packagesPath, []byte(index.Stdout), 0o644, true); err != nil {
		return err
	}
	compressed, err := b.runner.Run(ctx, executor.Command{
		Name:    "gzip",
		Args:    []string{"-9c", "Packages"},
		Dir:     profile.Repository.PackageDir,
		Timeout: time.Hour,
	})
	if err != nil {
		return fmt.Errorf("生成 DEB Packages.gz 索引失败: %w", err)
	}
	if err := fileutil.WriteFile(filepath.Join(profile.Repository.PackageDir, "Packages.gz"), []byte(compressed.Stdout), 0o644, true); err != nil {
		return err
	}
	return b.VerifyRepo(profile)
}

// debPackageNames extracts Debian package names from .deb files using dpkg-deb.
func debPackageNames(ctx context.Context, runner executor.Runner, files []string) ([]string, error) {
	if len(files) == 0 {
		return nil, nil
	}
	var names []string
	seen := make(map[string]struct{})
	for _, f := range files {
		result, err := runner.Run(ctx, executor.Command{
			Name:    "dpkg-deb",
			Args:    []string{"-f", f, "Package"},
			Timeout: 30 * time.Second,
		})
		if err != nil {
			return nil, fmt.Errorf("提取 DEB 包名失败 %s: %w", f, err)
		}
		name := strings.TrimSpace(result.Stdout)
		if name == "" {
			return nil, fmt.Errorf("DEB 文件中未找到 Package 字段: %s", f)
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names, nil
}

func prepareAPT(profile *config.ProfileConfig) error {
	directories := []string{
		filepath.Join(profile.Online.APTRoot, "etc", "apt", "sources.list.d"),
		filepath.Join(profile.Online.APTRoot, "var", "lib", "dpkg"),
		filepath.Join(profile.Online.APTState, "lists", "partial"),
		filepath.Join(profile.Online.APTCache, "archives", "partial"),
		filepath.Join(profile.Repository.PackageDir, "partial"),
		profile.Repository.PackageDir,
	}
	for _, path := range directories {
		if err := fileutil.EnsureDir(path, 0o755); err != nil {
			return err
		}
	}
	for _, path := range []string{
		filepath.Join(profile.Online.APTRoot, "var", "lib", "dpkg", "status"),
		filepath.Join(profile.Online.APTState, "status"),
	} {
		if err := fileutil.WriteFile(path, nil, 0o644, false); err != nil {
			return err
		}
	}
	if profile.Online.APTSourcesMode == "copy_from_host" {
		return copyAPTSources(profile.Online.APTRoot)
	}
	return nil
}

func copyAPTSources(aptRoot string) error {
	target := filepath.Join(aptRoot, "etc", "apt")
	if data, err := os.ReadFile("/etc/apt/sources.list"); err == nil {
		if err := fileutil.WriteFile(filepath.Join(target, "sources.list"), data, 0o644, true); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读取主机 apt sources.list 失败: %w", err)
	}

	entries, err := os.ReadDir("/etc/apt/sources.list.d")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取主机 apt sources.list.d 失败: %w", err)
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/etc/apt/sources.list.d", entry.Name()))
		if err != nil {
			return fmt.Errorf("读取主机 apt 源文件 %s 失败: %w", entry.Name(), err)
		}
		if err := fileutil.WriteFile(filepath.Join(target, "sources.list.d", entry.Name()), data, 0o644, true); err != nil {
			return err
		}
	}
	return nil
}

func aptOptions(profile *config.ProfileConfig) []string {
	return []string{
		"-o", "Dir=" + profile.Online.APTRoot,
		"-o", "Dir::Cache=" + profile.Online.APTCache,
		"-o", "Dir::State=" + profile.Online.APTState,
		"-o", "Dir::Cache::archives=" + profile.Repository.PackageDir,
	}
}
