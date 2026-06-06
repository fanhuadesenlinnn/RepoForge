package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const longDescription = `RepoForge 是 Linux 离线软件源构建与分发工具。

常用命令：
  repoforge init
  repoforge check --profile xxx
  repoforge make --profile xxx
  repoforge use --profile xxx
  repoforge server start
  repoforge server enable`

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "repoforge",
		Short:         "Linux 离线软件源构建与分发工具",
		Long:          longDescription,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(
		newInitCommand(),
		newCheckCommand(),
		newMakeCommand(),
		newUseCommand(),
		newServerCommand(),
	)
	return root
}

// Execute runs the RepoForge command line interface.
func Execute() error {
	return newRootCommand().Execute()
}

func notImplemented(name string) error {
	return fmt.Errorf("%s 命令将在后续实现阶段提供", name)
}
