package cmd

import (
	"context"
	"fmt"

	"github.com/fanhuadesenlinnn/RepoForge/internal/backend"
	"github.com/fanhuadesenlinnn/RepoForge/internal/backend/deb"
	"github.com/fanhuadesenlinnn/RepoForge/internal/backend/rpm"
	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/detect"
	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
	"github.com/fanhuadesenlinnn/RepoForge/internal/home"
	"github.com/spf13/cobra"
)

func newMakeCommand() *cobra.Command {
	var profileName string
	command := &cobra.Command{
		Use:   "make",
		Short: "制作离线软件源",
		RunE: func(command *cobra.Command, _ []string) error {
			homeDir, cfg, profile, packages, runner, err := loadMakeInputs(command.Context(), profileName)
			if err != nil {
				return err
			}
			selected, err := selectBackend(profile.Backend, runner)
			if err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), `开始制作离线软件源。

profile: %s
backend: %s
软件包数量: %d
软件源目录: %s

正在下载软件包及依赖并生成索引...
`, profile.Profile, selected.Name(), len(packages), profile.Repository.PackageDir)
			if err := selected.Make(command.Context(), cfg, profile, packages); err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "\n完成。\n软件源目录：%s\nRepoForge Home：%s\n", profile.Repository.PackageDir, homeDir)
			return nil
		},
	}
	command.Flags().StringVar(&profileName, "profile", "", "要制作的 profile 名称")
	_ = command.MarkFlagRequired("profile")
	return command
}

func loadMakeInputs(ctx context.Context, profileName string) (string, *config.Config, *config.ProfileConfig, []string, executor.Runner, error) {
	homeDir, err := home.Detect(false)
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	cfg, err := config.Load(homeDir)
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	profile, err := config.LoadProfile(homeDir, profileName)
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	packages, err := config.PackagesForProfile(homeDir, profileName)
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	runner := executor.New(false)
	system, err := detect.Current(ctx, runner)
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	if err := detect.CheckCompatibility(system, profile); err != nil {
		return "", nil, nil, nil, nil, err
	}
	return homeDir, cfg, profile, packages, runner, nil
}

func selectBackend(name string, runner executor.Runner) (backend.Backend, error) {
	switch name {
	case "rpm":
		return rpm.New(runner), nil
	case "deb":
		return deb.New(runner), nil
	default:
		return nil, fmt.Errorf("不支持的 backend: %s", name)
	}
}
