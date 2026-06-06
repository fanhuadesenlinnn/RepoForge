package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/detect"
	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
	"github.com/fanhuadesenlinnn/RepoForge/internal/home"
	"github.com/fanhuadesenlinnn/RepoForge/internal/server"
	"github.com/spf13/cobra"
)

func newCheckCommand() *cobra.Command {
	var profileName string
	command := &cobra.Command{
		Use:   "check",
		Short: "检查环境和仓库状态",
		RunE: func(command *cobra.Command, _ []string) error {
			homeDir, err := home.Detect(false)
			if err != nil {
				return err
			}
			output := command.OutOrStdout()
			fmt.Fprintf(output, "[OK] RepoForge Home: %s\n", homeDir)

			required := []string{
				filepath.Join(homeDir, "config", "config.yaml"),
				filepath.Join(homeDir, "config", "packages.yaml"),
				filepath.Join(homeDir, "repos"),
				filepath.Join(homeDir, "cache"),
			}
			for _, path := range required {
				if _, err := os.Stat(path); err != nil {
					return fmt.Errorf("必需路径不可用 %s: %w", path, err)
				}
				fmt.Fprintf(output, "[OK] 路径存在: %s\n", path)
			}

			cfg, err := config.Load(homeDir)
			if err != nil {
				return err
			}
			fmt.Fprintf(output, "[OK] 配置文件有效: schema_version=%d\n", cfg.SchemaVersion)

			runner := executor.New(false)
			system, err := detect.Current(context.Background(), runner)
			if err != nil {
				return err
			}
			fmt.Fprintf(output, "[OK] 当前系统: %s %s %s (backend=%s)\n",
				system.PrettyName, system.VersionID, system.RawArch, system.Backend)

			if profileName != "" {
				profile, err := config.LoadProfile(homeDir, profileName)
				if err != nil {
					return err
				}
				fmt.Fprintf(output, "[OK] profile 配置有效: %s (%s)\n", profile.Profile, profile.Backend)
				if err := detect.CheckCompatibility(system, profile); err != nil {
					return err
				}
				fmt.Fprintf(output, "[OK] profile 匹配: %s\n", profile.Profile)
				checkCommands(output, runner, profile)
				checkRepository(output, cfg, profile)
			}
			return nil
		},
	}
	command.Flags().StringVar(&profileName, "profile", "", "要检查的 profile 名称")
	return command
}

func checkCommands(output interface{ Write([]byte) (int, error) }, runner executor.Runner, profile *config.ProfileConfig) {
	var groups [][]string
	if profile.Backend == "rpm" {
		groups = [][]string{{"dnf", "yum"}, {"rpm"}, {profile.Repository.MetadataTool}}
	} else {
		groups = [][]string{{"apt-get"}, {"apt-cache"}, {profile.Repository.MetadataTool}, {"gzip"}}
	}
	for _, group := range groups {
		name, err := detect.FindAny(runner, group...)
		if err != nil {
			fmt.Fprintf(output, "[ERROR] 未找到命令: %v\n", group)
			continue
		}
		fmt.Fprintf(output, "[OK] 命令可用: %s\n", name)
	}
}

func checkRepository(output interface{ Write([]byte) (int, error) }, cfg *config.Config, profile *config.ProfileConfig) {
	index := filepath.Join(profile.Repository.PackageDir, "repodata", "repomd.xml")
	if profile.Backend == "deb" {
		index = filepath.Join(profile.Repository.PackageDir, "Packages.gz")
	}
	if _, err := os.Stat(index); err == nil {
		fmt.Fprintf(output, "[OK] 软件源索引存在: %s\n", index)
	} else {
		fmt.Fprintf(output, "[WARN] 软件源索引不存在: %s\n", index)
	}
	if _, err := os.Stat(profile.LocalRepo.RepoFile); err == nil {
		fmt.Fprintf(output, "[OK] 本机源已启用: %s\n", profile.LocalRepo.RepoFile)
	} else {
		fmt.Fprintf(output, "[WARN] 本机源未启用: %s\n", profile.LocalRepo.RepoFile)
	}
	if err := server.CheckProfile(cfg.Server, profile); err != nil {
		fmt.Fprintf(output, "[WARN] HTTP 服务检查失败: %v\n", err)
	} else {
		fmt.Fprintln(output, "[OK] HTTP 软件源可访问")
	}
}
