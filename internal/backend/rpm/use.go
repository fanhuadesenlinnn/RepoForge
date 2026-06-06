package rpm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/detect"
	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
	"github.com/fanhuadesenlinnn/RepoForge/internal/fileutil"
	"github.com/fanhuadesenlinnn/RepoForge/internal/render"
)

type repoTemplateData struct {
	RepoID   string
	RepoName string
	BaseURL  string
}

// EnableLocalRepo installs the RPM file:// repository configuration.
func (b *Backend) EnableLocalRepo(ctx context.Context, cfg *config.Config, profile *config.ProfileConfig) error {
	if err := b.VerifyRepo(profile); err != nil {
		return err
	}
	content, err := render.File(filepath.Join(cfg.Paths.TemplateDir, "rpm-local.repo.tpl"), repoTemplateData{
		RepoID:   profile.LocalRepo.RepoID,
		RepoName: profile.LocalRepo.RepoName,
		BaseURL:  profile.LocalRepo.BaseURL,
	})
	if err != nil {
		return err
	}
	if err := fileutil.WriteFile(profile.LocalRepo.RepoFile, content, 0o644, true); err != nil {
		return err
	}
	if !profile.LocalRepo.MakecacheAfterEnable {
		return nil
	}
	manager, err := detect.FindAny(b.runner, "dnf", "yum")
	if err != nil {
		return err
	}
	_, err = b.runner.Run(ctx, executor.Command{
		Name: manager,
		Args: []string{
			"makecache",
			"--disablerepo=*",
			"--enablerepo=" + profile.LocalRepo.RepoID,
		},
		Timeout: 30 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("刷新 RPM 本机源缓存失败: %w", err)
	}
	return nil
}

// DisableLocalRepo disables or removes the RPM file:// repository configuration.
func (b *Backend) DisableLocalRepo(_ context.Context, _ *config.Config, profile *config.ProfileConfig, remove bool) error {
	path := profile.LocalRepo.RepoFile
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取 RPM 本机源配置 %s 失败: %w", path, err)
	}
	if remove {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除 RPM 本机源配置 %s 失败: %w", path, err)
		}
		return nil
	}
	disabled := strings.ReplaceAll(string(data), "enabled=1", "enabled=0")
	return fileutil.WriteFile(path, []byte(disabled), 0o644, true)
}

// GenerateClientRepo writes a LAN RPM repository configuration.
func (b *Backend) GenerateClientRepo(cfg *config.Config, profile *config.ProfileConfig, publicURL string) error {
	content, err := render.File(filepath.Join(cfg.Paths.TemplateDir, "rpm-client.repo.tpl"), repoTemplateData{
		RepoID:   profile.ClientRepo.RepoID,
		RepoName: profile.ClientRepo.RepoName,
		BaseURL:  strings.ReplaceAll(profile.ClientRepo.BaseURL, "${server.public_url}", strings.TrimRight(publicURL, "/")),
	})
	if err != nil {
		return err
	}
	return fileutil.WriteFile(profile.ClientRepo.Output, content, 0o644, true)
}
