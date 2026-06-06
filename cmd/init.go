package cmd

import (
	"fmt"

	"github.com/fanhuadesenlinnn/RepoForge/internal/home"
	"github.com/fanhuadesenlinnn/RepoForge/internal/initialize"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	var force bool
	command := &cobra.Command{
		Use:   "init",
		Short: "初始化 RepoForge 目录",
		Args:  noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			homeDir, err := home.Detect(true)
			if err != nil {
				return err
			}
			if err := initialize.Run(homeDir, force); err != nil {
				return err
			}

			fmt.Fprintf(command.OutOrStdout(), `RepoForge 初始化完成。

Home：%s
配置目录：%s/config
软件源目录：%s/repos

下一步：
1. 编辑 %s/config/packages.yaml
2. 执行 repoforge make --profile kylin-v10-sp3-x86_64
`, homeDir, homeDir, homeDir, homeDir)
			return nil
		},
	}
	command.Flags().BoolVar(&force, "force", false, "覆盖默认配置和模板文件，不删除已有软件包")
	return command
}
