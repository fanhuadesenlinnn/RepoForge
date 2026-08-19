package cmd

import (
	"fmt"

	"github.com/fanhuadesenlinnn/RepoForge/internal/engine"
	"github.com/fanhuadesenlinnn/RepoForge/internal/home"
	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/spf13/cobra"
)

func newInstallCommand() *cobra.Command {
	var repoName string
	command := &cobra.Command{
		Use:   "install [packages...]",
		Short: "按需制作离线源（指定软件 + 自动依赖求解）",
		Long: `按 repo.yaml 中配置的 repositories 制作离线源，只下载指定软件及其依赖。

依赖自动求解（RPM 与 DEB 都支持），生成可离线使用的 yum/apt 源。
可用 --repo 指定仓库；命令行额外包会追加到 install.packages。`,
		Args: cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			homeDir, err := home.Detect(false)
			if err != nil {
				return err
			}
			cfg, err := repo.Load(homeDir)
			if err != nil {
				return err
			}
			repos := selectRepos(cfg, repoName, func(r *repo.Repository) bool {
				return true
			})
			if repoName != "" && len(repos) == 0 {
				return fmt.Errorf("未找到 repository %q 或未配置 install", repoName)
			}
			if repoName == "" {
				// pick repos that have install configured
				repos = nil
				for i := range cfg.Repositories {
					if len(cfg.Repositories[i].Install.Packages) > 0 {
						repos = append(repos, &cfg.Repositories[i])
					}
				}
			}
			if len(repos) == 0 {
				return fmt.Errorf("没有配置 install.packages 的仓库")
			}
			var anyErr bool
			for _, r := range repos {
				pkgs := append(append([]string{}, r.Install.Packages...), args...)
				if len(pkgs) == 0 {
					return fmt.Errorf("仓库 %q 未指定要安装的软件包", r.Name)
				}
				variants, err := repo.Expand(cfg, r)
				if err != nil {
					return err
				}
				for _, ev := range variants {
					fmt.Fprintf(command.OutOrStdout(), "制作离线源: %s\n  请求: %v\n  URL: %s\n  输出: %s\n", r.Name, pkgs, ev.URL, ev.ContentRoot(cfg))
					// Temporarily set the requested packages on the variant's repo.
					inst := ev
					inst.Repository.Install.Packages = pkgs
					result, err := engine.Install(command.Context(), cfg, &inst)
					if err != nil {
						fmt.Fprintf(command.OutOrStderr(), "[ERROR] %s: %v\n", r.Name, err)
						anyErr = true
						continue
					}
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
				return fmt.Errorf("制作过程中存在未解决的问题")
			}
			fmt.Fprintln(command.OutOrStdout(), "离线源制作完成。")
			return nil
		},
	}
	command.Flags().StringVar(&repoName, "repo", "", "要制作的 repository 名称")
	return command
}
