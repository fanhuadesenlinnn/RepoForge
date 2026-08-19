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

下一步：
1. 编辑 %s/config/repo.yaml，配置你的仓库（含注释示例，改改即可用）
2. 执行 repoforge sync   全量镜像上游仓库
   或   repoforge make   按需制作离线源（input 点名）
3. 需要时：repoforge client / use / server 分发
`, homeDir, homeDir)
			return nil
		},
	}
	command.Flags().BoolVar(&force, "force", false, "覆盖默认配置和模板文件，不删除已有软件包")
	return command
}
