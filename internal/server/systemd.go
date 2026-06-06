package server

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

type systemdTemplateData struct {
	Home       string
	Executable string
	Restart    string
	RepoDir    string
}

// Manager manages the RepoForge systemd service.
type Manager struct {
	runner executor.Runner
}

// NewManager returns a systemd manager.
func NewManager(runner executor.Runner) *Manager {
	return &Manager{runner: runner}
}

// Enable installs, enables, and restarts the systemd service.
func (m *Manager) Enable(ctx context.Context, cfg *config.Config, executable string) error {
	content, err := render.File(filepath.Join(cfg.Paths.TemplateDir, "repoforge-server.service.tpl"), systemdTemplateData{
		Home:       cfg.Paths.HomeDir,
		Executable: executable,
		Restart:    cfg.Server.Systemd.Restart,
		RepoDir:    cfg.Paths.RepoDir,
	})
	if err != nil {
		return err
	}
	if err := fileutil.WriteFile(cfg.Server.Systemd.ServiceFile, content, 0o644, true); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"daemon-reload"},
		{"enable", cfg.Server.Systemd.ServiceName},
		{"restart", cfg.Server.Systemd.ServiceName},
	} {
		if _, err := m.runner.Run(ctx, executor.Command{
			Name:    "systemctl",
			Args:    args,
			Timeout: time.Minute,
		}); err != nil {
			return fmt.Errorf("管理 systemd 服务失败: %w", err)
		}
	}
	return nil
}

// Stop stops the managed service when it exists.
func (m *Manager) Stop(ctx context.Context, cfg *config.Config) error {
	if _, err := os.Stat(cfg.Server.Systemd.ServiceFile); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("检查 systemd 服务文件失败: %w", err)
	}
	_, err := m.runner.Run(ctx, executor.Command{
		Name:    "systemctl",
		Args:    []string{"stop", cfg.Server.Systemd.ServiceName},
		Timeout: time.Minute,
	})
	return err
}

// Disable stops, disables, and removes the managed service.
func (m *Manager) Disable(ctx context.Context, cfg *config.Config) error {
	if _, err := os.Stat(cfg.Server.Systemd.ServiceFile); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("检查 systemd 服务文件失败: %w", err)
	}
	for _, args := range [][]string{
		{"stop", cfg.Server.Systemd.ServiceName},
		{"disable", cfg.Server.Systemd.ServiceName},
	} {
		if _, err := m.runner.Run(ctx, executor.Command{Name: "systemctl", Args: args, Timeout: time.Minute}); err != nil {
			return err
		}
	}
	if err := os.Remove(cfg.Server.Systemd.ServiceFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 systemd 服务文件失败: %w", err)
	}
	_, err := m.runner.Run(ctx, executor.Command{Name: "systemctl", Args: []string{"daemon-reload"}, Timeout: time.Minute})
	return err
}

// Status returns a short systemd status without treating inactivity as an error.
func (m *Manager) Status(ctx context.Context, cfg *config.Config) string {
	if _, err := os.Stat(cfg.Server.Systemd.ServiceFile); os.IsNotExist(err) {
		return "未安装"
	}
	result, err := m.runner.Run(ctx, executor.Command{
		Name:    "systemctl",
		Args:    []string{"is-active", cfg.Server.Systemd.ServiceName},
		Timeout: 10 * time.Second,
	})
	if err != nil {
		if value := strings.TrimSpace(result.Stdout); value != "" {
			return value
		}
		return "未运行"
	}
	return strings.TrimSpace(result.Stdout)
}
