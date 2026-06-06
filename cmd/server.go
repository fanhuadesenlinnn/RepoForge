package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
	"github.com/fanhuadesenlinnn/RepoForge/internal/home"
	"github.com/fanhuadesenlinnn/RepoForge/internal/privilege"
	"github.com/fanhuadesenlinnn/RepoForge/internal/server"
	"github.com/spf13/cobra"
)

func newServerCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "server",
		Short: "管理只读 HTTP 软件源服务",
	}
	command.AddCommand(
		newServerStartCommand(),
		newServerEnableCommand(),
		newServerStopCommand(),
		newServerDisableCommand(),
		newServerStatusCommand(),
	)
	return command
}

func newServerStartCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "前台启动 HTTP 服务",
		Args:  noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, cfg, err := loadServerConfig()
			if err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "RepoForge HTTP 服务启动：%s\n软件源根目录：%s\n", cfg.Server.Listen, cfg.Server.Root)
			ctx, stop := signal.NotifyContext(command.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return server.Serve(ctx, cfg.Server.Listen, cfg.Server.Root, cfg.Server.DirectoryListing)
		},
	}
}

func newServerEnableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "enable",
		Short: "安装并启用 systemd 服务",
		Args:  noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := privilege.RequireRoot("server enable 需要写入 systemd 服务目录", "sudo repoforge server enable"); err != nil {
				return err
			}
			homeDir, cfg, err := loadServerConfig()
			if err != nil {
				return err
			}
			executable, err := os.Executable()
			if err != nil {
				return fmt.Errorf("获取 repoforge 可执行文件路径失败: %w", err)
			}
			executable, err = filepath.EvalSymlinks(executable)
			if err != nil {
				return fmt.Errorf("解析 repoforge 可执行文件路径失败: %w", err)
			}
			manager := server.NewManager(executor.New(false))
			if err := manager.Enable(command.Context(), cfg, executable); err != nil {
				return err
			}
			publicURL, candidates, err := server.ResolvePublicURL(cfg.Server)
			if err != nil {
				return err
			}
			if len(candidates) > 1 {
				fmt.Fprintf(command.OutOrStdout(), "[WARN] 检测到多个 IPv4 地址 %v，客户端配置使用 %s\n", candidates, publicURL)
			}
			if err := generateClientRepos(homeDir, cfg, publicURL); err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "HTTP 服务已启用：%s\n客户端配置目录：%s\n", publicURL, cfg.Paths.ClientDir)
			return nil
		},
	}
}

func newServerStopCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "停止 systemd 服务",
		Args:  noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := privilege.RequireRoot("server stop 需要管理 systemd 服务", "sudo repoforge server stop"); err != nil {
				return err
			}
			_, cfg, err := loadServerConfig()
			if err != nil {
				return err
			}
			if err := server.NewManager(executor.New(false)).Stop(command.Context(), cfg); err != nil {
				return err
			}
			fmt.Fprintln(command.OutOrStdout(), "HTTP 服务已停止。")
			return nil
		},
	}
}

func newServerDisableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "禁用并删除 systemd 服务",
		Args:  noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := privilege.RequireRoot("server disable 需要管理 systemd 服务", "sudo repoforge server disable"); err != nil {
				return err
			}
			_, cfg, err := loadServerConfig()
			if err != nil {
				return err
			}
			if err := server.NewManager(executor.New(false)).Disable(command.Context(), cfg); err != nil {
				return err
			}
			fmt.Fprintln(command.OutOrStdout(), "HTTP 服务已禁用。")
			return nil
		},
	}
}

func newServerStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看 HTTP 服务状态",
		Args:  noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			homeDir, cfg, err := loadServerConfig()
			if err != nil {
				return err
			}
			publicURL, candidates, publicErr := server.ResolvePublicURL(cfg.Server)
			if publicErr != nil {
				publicURL = "无法判断：" + publicErr.Error()
			}
			profiles, err := config.LoadProfiles(homeDir)
			if err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "服务状态：%s\n监听地址：%s\n软件源根目录：%s\n局域网访问 URL：%s\n",
				server.NewManager(executor.New(false)).Status(command.Context(), cfg),
				cfg.Server.Listen,
				cfg.Server.Root,
				publicURL,
			)
			if len(candidates) > 1 {
				fmt.Fprintf(command.OutOrStdout(), "候选 IPv4：%v\n", candidates)
			}
			fmt.Fprintln(command.OutOrStdout(), "当前可用 profile：")
			available := 0
			for _, profile := range profiles {
				if !repositoryAvailable(profile) {
					continue
				}
				available++
				fmt.Fprintf(command.OutOrStdout(), "  - %s，客户端配置：%s\n", profile.Profile, profile.ClientRepo.Output)
			}
			if available == 0 {
				fmt.Fprintln(command.OutOrStdout(), "  （无，请先执行 repoforge make）")
			}
			return nil
		},
	}
}

func loadServerConfig() (string, *config.Config, error) {
	homeDir, err := home.Detect(false)
	if err != nil {
		return "", nil, err
	}
	cfg, err := config.Load(homeDir)
	return homeDir, cfg, err
}

func generateClientRepos(homeDir string, cfg *config.Config, publicURL string) error {
	profiles, err := config.LoadProfiles(homeDir)
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		selected, err := selectBackend(profile.Backend, executor.New(false))
		if err != nil {
			return err
		}
		if err := selected.GenerateClientRepo(cfg, profile, publicURL); err != nil {
			return err
		}
	}
	return nil
}

func repositoryAvailable(profile *config.ProfileConfig) bool {
	index := filepath.Join(profile.Repository.PackageDir, "Packages.gz")
	if profile.Backend == "rpm" {
		index = filepath.Join(profile.Repository.PackageDir, "repodata", "repomd.xml")
	}
	_, err := os.Stat(index)
	return err == nil
}
