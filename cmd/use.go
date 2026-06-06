package cmd

import (
	"fmt"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
	"github.com/fanhuadesenlinnn/RepoForge/internal/home"
	"github.com/fanhuadesenlinnn/RepoForge/internal/privilege"
	"github.com/spf13/cobra"
)

func newUseCommand() *cobra.Command {
	var profileName string
	var disable bool
	var remove bool
	command := &cobra.Command{
		Use:   "use",
		Short: "启用或禁用本机 file:// 软件源",
		RunE: func(command *cobra.Command, _ []string) error {
			if err := privilege.RequireRoot(
				"repoforge use 需要写入系统软件源目录",
				"sudo repoforge use --profile "+profileName,
			); err != nil {
				return err
			}
			homeDir, err := home.Detect(false)
			if err != nil {
				return err
			}
			cfg, err := config.Load(homeDir)
			if err != nil {
				return err
			}
			profile, err := config.LoadProfile(homeDir, profileName)
			if err != nil {
				return err
			}
			selected, err := selectBackend(profile.Backend, executor.New(false))
			if err != nil {
				return err
			}
			if disable {
				if err := selected.DisableLocalRepo(command.Context(), cfg, profile, remove); err != nil {
					return err
				}
				fmt.Fprintf(command.OutOrStdout(), "本机软件源已禁用：%s\n", profile.LocalRepo.RepoFile)
				return nil
			}
			if err := selected.EnableLocalRepo(command.Context(), cfg, profile); err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "本机软件源已启用：%s\n", profile.LocalRepo.RepoFile)
			return nil
		},
	}
	command.Flags().StringVar(&profileName, "profile", "", "要使用的 profile 名称")
	command.Flags().BoolVar(&disable, "disable", false, "禁用本机软件源")
	command.Flags().BoolVar(&remove, "remove", false, "禁用时删除软件源配置文件")
	_ = command.MarkFlagRequired("profile")
	return command
}
