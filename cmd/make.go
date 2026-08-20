package cmd

import (
	"fmt"

	"github.com/fanhuadesenlinnn/RepoForge/internal/engine"
	"github.com/fanhuadesenlinnn/RepoForge/internal/home"
	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/spf13/cobra"
)

func newMakeCommand() *cobra.Command {
	var repoName string
	command := &cobra.Command{
		Use:   "make [packages...]",
		Short: "按需制作离线源（点名 / 本地补齐依赖 / 升级）",
		Long: `按 repo.yaml 中配置的 repositories 制作离线源。

支持多种起点（可并存），输出统一到 repo_dir/<name>：
  - make.packages          点名要做的软件（+ 依赖）
  - input.package_dirs     本地已有包 → 补齐缺失依赖
  - input.upgrade_packages 升级 → 取这些软件的上游新版本（+ 依赖）

依赖自动求解（RPM 与 DEB 都支持），生成可离线使用的 yum/apt 源。
可用 --repo 指定仓库；命令行额外包会追加到 make.packages。`,
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
				return fmt.Errorf("未找到 repository %q", repoName)
			}
			if repoName == "" {
				repos = nil
				for i := range cfg.Repositories {
					r := &cfg.Repositories[i]
					if len(r.Input.Packages) > 0 || len(r.Input.PackageDirs) > 0 || len(r.Input.UpgradePackages) > 0 {
						repos = append(repos, r)
					}
				}
			}
			if len(repos) == 0 {
				return fmt.Errorf("没有配置 make.packages / input 的仓库")
			}
			var anyErr bool
			for _, r := range repos {
				pkgs := append(append([]string{}, r.Input.Packages...), args...)
				variants, err := repo.Expand(cfg, r)
				if err != nil {
					return err
				}
				for _, ev := range variants {
					fmt.Fprintf(command.OutOrStdout(), "制作离线源: %s\n  点名: %v\n  升级: %v\n  输出: %s\n",
						r.Name, pkgs, r.Input.UpgradePackages, ev.ContentRoot(cfg))
					mk := ev
					mk.Repository.Input.Packages = pkgs
					result, err := engine.Make(withProgress(command), cfg, &mk)
					if err != nil {
						fmt.Fprintf(command.OutOrStderr(), "[ERROR] %s: %v\n", r.Name, err)
						anyErr = true
						continue
					}
					if result.SkippedLocal > 0 {
						fmt.Fprintf(command.OutOrStdout(), "完成: 已选 %d 包，下载 %d，本地复制 %d，跳过(架构不匹配) %d，repodata: %s\n",
							result.Selected, result.Downloaded, result.Copied, result.SkippedLocal, result.Repodata)
					} else {
						fmt.Fprintf(command.OutOrStdout(), "完成: 已选 %d 包，下载 %d，本地复制 %d，repodata: %s\n",
							result.Selected, result.Downloaded, result.Copied, result.Repodata)
					}
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
