package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newMakeUpgradeCommand() *cobra.Command {
	var profileName string
	command := &cobra.Command{
		Use:   "make-upgrade",
		Short: "下载当前系统升级所需 RPM 包并生成离线软件源",
		Long: `下载当前系统升级所需 RPM 包并生成离线软件源。

该命令基于当前系统已安装包状态计算可升级包，因此不会使用 RPM installroot。
制作升级源时，应在与目标机器相同发行版、版本、架构、已安装包状态尽量一致的在线机器上执行。`,
		Args: noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			homeDir, cfg, profile, runner, err := loadProfileInputs(command.Context(), profileName)
			if err != nil {
				return err
			}
			selected, err := selectBackend(profile.Backend, runner)
			if err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), `开始制作系统升级离线软件源。

profile: %s
backend: %s
软件源目录: %s
`, profile.Profile, selected.Name(), profile.Repository.PackageDir)
			if len(profile.Online.DisableRepos) > 0 {
				fmt.Fprintf(command.OutOrStdout(), "禁用仓库: %v\n", profile.Online.DisableRepos)
			}
			if len(profile.Online.EnableRepos) > 0 {
				fmt.Fprintf(command.OutOrStdout(), "启用仓库: %v\n", profile.Online.EnableRepos)
			}
			fmt.Fprintf(command.OutOrStdout(), "\n正在下载当前系统升级所需软件包并生成索引...\n")
			if err := selected.MakeUpgrade(command.Context(), cfg, profile); err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "\n完成。\n软件源目录：%s\nRepoForge Home：%s\n", profile.Repository.PackageDir, homeDir)
			return nil
		},
	}
	command.Flags().StringVar(&profileName, "profile", "", "要制作升级源的 profile 名称（留空自动匹配当前系统）")
	return command
}
