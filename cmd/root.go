package cmd

import (
	"fmt"

	"github.com/fanhuadesenlinnn/RepoForge/internal/version"
	"github.com/spf13/cobra"
)

const longDescription = `RepoForge 是 Linux 离线软件源构建与分发工具。

常用命令：
  repoforge init
  repoforge check
  repoforge make
  repoforge make-upgrade
  repoforge list
  repoforge use
  repoforge server start
  repoforge server enable
  repoforge server status

自动匹配当前系统 profile，也可用 --profile 指定。`

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
