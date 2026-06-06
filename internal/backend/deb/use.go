package deb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
	"github.com/fanhuadesenlinnn/RepoForge/internal/fileutil"
	"github.com/fanhuadesenlinnn/RepoForge/internal/render"
)

type listTemplateData struct {
	BaseURL string
	Suite   string
}

// EnableLocalRepo installs the DEB file: repository configuration.
func (b *Backend) EnableLocalRepo(ctx context.Context, cfg *config.Config, profile *config.ProfileConfig) error {
	if err := b.VerifyRepo(profile); err != nil {
		return err
	}
	content, err := render.File(filepath.Join(cfg.Paths.TemplateDir, "deb-local.list.tpl"), listTemplateData{
		BaseURL: profile.LocalRepo.BaseURL,
		Suite:   profile.LocalRepo.Suite,
	})
	if err != nil {
		return err
	}
	if err := fileutil.WriteFile(profile.LocalRepo.RepoFile, content, 0o644, true); err != nil {
		return err
	}
	_ = os.Remove(profile.LocalRepo.RepoFile + ".disabled")
	if !profile.LocalRepo.UpdateAfterEnable {
		return nil
	}
	if _, err := b.runner.Run(ctx, executor.Command{
		Name:    "apt-get",
		Args:    []string{"update"},
		Timeout: time.Hour,
	}); err != nil {
		return fmt.Errorf("刷新 DEB 本机源缓存失败: %w", err)
	}
	return nil
}

// DisableLocalRepo disables or removes the DEB file: repository configuration.
func (b *Backend) DisableLocalRepo(_ context.Context, _ *config.Config, profile *config.ProfileConfig, remove bool) error {
	path := profile.LocalRepo.RepoFile
	disabled := path + ".disabled"
	if remove {
		for _, managed := range []string{path, disabled} {
			if err := os.Remove(managed); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("删除 DEB 本机源配置 %s 失败: %w", managed, err)
			}
		}
		return nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("检查 DEB 本机源配置 %s 失败: %w", path, err)
	}
	_ = os.Remove(disabled)
	if err := os.Rename(path, disabled); err != nil {
		return fmt.Errorf("禁用 DEB 本机源配置 %s 失败: %w", path, err)
	}
	return nil
}

// GenerateClientRepo writes a LAN DEB repository configuration.
func (b *Backend) GenerateClientRepo(cfg *config.Config, profile *config.ProfileConfig, publicURL string) error {
	content, err := render.File(filepath.Join(cfg.Paths.TemplateDir, "deb-client.list.tpl"), listTemplateData{
		BaseURL: strings.ReplaceAll(profile.ClientRepo.BaseURL, "${server.public_url}", strings.TrimRight(publicURL, "/")),
		Suite:   profile.ClientRepo.Suite,
	})
	if err != nil {
		return err
	}
	return fileutil.WriteFile(profile.ClientRepo.Output, content, 0o644, true)
}
