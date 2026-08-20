package cmd

import (
	"fmt"

	"github.com/fanhuadesenlinnn/RepoForge/internal/version"
	"github.com/spf13/cobra"
)

const longDescription = `RepoForge 是跨平台（Windows/Linux/macOS）Linux 离线软件源构建与分发工具。

单文件 repo.yaml 配置，引擎纯 Go，不依赖本机 dnf/apt/createrepo：
  repoforge init          初始化目录
  repoforge sync          全量镜像上游仓库
  repoforge make          按需制作离线源（点名 / 本地包 / 升级包）
  repoforge make-upgrade  对照本机已装包制作升级源（需在目标 Linux 上）
  repoforge list          列出本地离线源中的包
  repoforge check         检查环境和仓库状态
  repoforge client        生成客户端 yum/apt 源配置
  repoforge use           启用本机 file:// 源
  repoforge server        局域网 HTTP 分发
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
		newClientCommand(),
		newListCommand(),
		newUseCommand(),
		newServerCommand(),
		newGPGCommand(),
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
