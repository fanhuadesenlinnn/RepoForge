package cmd

import (
	"fmt"

	"github.com/fanhuadesenlinnn/RepoForge/internal/engine"
	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/spf13/cobra"
)

func newMakeUpgradeCommand() *cobra.Command {
	var repoName string
	command := &cobra.Command{
		Use:   "make-upgrade",
		Short: "对照本机已安装包，从上游下载可升级版本及依赖",
		Long: `读取本机已安装软件包，对照 repo.yaml 里的上游仓库，下载比本机更新的版本及其依赖。

需要在与目标环境相同发行版的 Linux 上运行（用 rpm / dpkg-query 读已装列表）。
制作结果写入该仓库的 repo_dir，并生成可离线使用的 yum/apt 索引。`,
		Args: noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, cfg, err := loadRepo()
			if err != nil {
				return err
			}
			repos := selectRepos(cfg, repoName, func(*repo.Repository) bool { return true })
			if repoName != "" && len(repos) == 0 {
				return fmt.Errorf("未找到 repository %q", repoName)
			}
			if len(repos) == 0 {
				return fmt.Errorf("repo.yaml 中没有仓库")
			}
			ctx := withProgress(command)
			var anyErr bool
			var anyWork bool
			for _, r := range repos {
				installed, err := engine.QueryInstalled(ctx, r.Backend)
				if err != nil {
					fmt.Fprintf(command.OutOrStderr(), "[ERROR] %s: %v\n", r.Name, err)
					anyErr = true
					continue
				}
				variants, err := repo.Expand(cfg, r)
				if err != nil {
					return err
				}
				for _, ev := range variants {
					fmt.Fprintf(command.OutOrStdout(), "制作升级源: %s\n  本机已装: %d\n  上游: %s\n  输出: %s\n",
						r.Name, len(installed), ev.URL, ev.ContentRoot(cfg))
					result, err := engine.MakeUpgrade(ctx, cfg, &ev, installed)
					if err != nil {
						fmt.Fprintf(command.OutOrStderr(), "[ERROR] %s: %v\n", r.Name, err)
						anyErr = true
						continue
					}
					anyWork = true
					fmt.Fprintf(command.OutOrStdout(), "完成: 已选 %d 包，下载 %d，repodata: %s\n",
						result.Selected, result.Downloaded, result.Repodata)
					for _, n := range result.Notices {
						fmt.Fprintf(command.OutOrStdout(), "  [INFO] %s\n", n)
					}
					for _, p := range result.Problems {
						fmt.Fprintf(command.OutOrStderr(), "  [WARN] %s\n", p)
						anyErr = true
					}
				}
			}
			if anyErr {
				return fmt.Errorf("制作升级源过程中存在未解决的问题")
			}
			if !anyWork {
				return fmt.Errorf("没有可处理的仓库")
			}
			fmt.Fprintln(command.OutOrStdout(), "升级源制作完成。")
			return nil
		},
	}
	command.Flags().StringVar(&repoName, "repo", "", "要制作升级源的 repository 名称")
	return command
}
