package server

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
	"github.com/fanhuadesenlinnn/RepoForge/internal/fileutil"
	"github.com/fanhuadesenlinnn/RepoForge/internal/render"
	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/fanhuadesenlinnn/RepoForge/templates"
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
func (m *Manager) Enable(ctx context.Context, cfg *repo.Config, executable string) error {
	content, err := renderSystemd(cfg, executable)
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

// renderSystemd renders the embedded systemd unit with the current config.
// The unit is compiled into the binary, so server enable works without a
// prior `repoforge init` and without any config/templates directory.
func renderSystemd(cfg *repo.Config, executable string) ([]byte, error) {
	raw, err := templates.ReadSystemdService()
	if err != nil {
		return nil, fmt.Errorf("读取内置 systemd 模板失败: %w", err)
	}
	return render.Text(string(raw), systemdTemplateData{
		Home:       cfg.Paths.HomeDir,
		Executable: executable,
		Restart:    cfg.Server.Systemd.Restart,
		RepoDir:    cfg.Paths.RepoDir,
	})
}

// Stop stops the managed service when it exists.
func (m *Manager) Stop(ctx context.Context, cfg *repo.Config) error {
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
func (m *Manager) Disable(ctx context.Context, cfg *repo.Config) error {
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
func (m *Manager) Status(ctx context.Context, cfg *repo.Config) string {
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
