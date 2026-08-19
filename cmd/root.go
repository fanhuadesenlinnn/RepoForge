package cmd

import (
	"fmt"

	"github.com/fanhuadesenlinnn/RepoForge/internal/version"
	"github.com/spf13/cobra"
)

const longDescription = `RepoForge 是跨平台（Windows/Linux/macOS）Linux 离线软件源构建与分发工具。

新式命令（单文件 repo.yaml 配置，纯 Go，不依赖本机源）：
  repoforge sync      全量镜像上游仓库
  repoforge install   按需制作离线源（指定软件 + 自动依赖求解）
  repoforge client    生成客户端 yum/apt 源配置

传统命令（多文件 profile 配置）：
  repoforge init
  repoforge check
  repoforge make
  repoforge make-upgrade
  repoforge use
  repoforge server start
`

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "repoforge",
		Short:         "Linux 离线软件源构建与分发工具",
		Long:          longDescription,
		Version:       version.String(),
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetVersionTemplate("RepoForge {{.Version}}\n")

	root.AddCommand(
		newInitCommand(),
		newCheckCommand(),
		newMakeCommand(),
		newMakeUpgradeCommand(),
		newSyncCommand(),
		newInstallCommand(),
		newClientCommand(),
		newListCommand(),
		newUseCommand(),
		newServerCommand(),
	)
	return root
}

// Execute runs the RepoForge command line interface.
func Execute() error {
	return newRootCommand().Execute()
}

func noArgs(command *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf("%s 不接受位置参数：%v", command.CommandPath(), args)
}
