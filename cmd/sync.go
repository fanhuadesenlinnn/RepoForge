package cmd

import (
	"fmt"

	"github.com/fanhuadesenlinnn/RepoForge/internal/engine"
	"github.com/fanhuadesenlinnn/RepoForge/internal/home"
	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/spf13/cobra"
)

func newSyncCommand() *cobra.Command {
	var repoName string
	command := &cobra.Command{
		Use:   "sync",
		Short: "全量镜像上游仓库（纯 Go，不依赖本机源）",
		Long: `按 repo.yaml 中配置的 repositories 从上游仓库全量镜像软件。

默认同步所有启用了 sync 的仓库；可用 --repo 指定单个。
引擎纯 Go 实现，读取上游元数据并下载校验，不依赖本机 dnf/apt/createrepo_c。`,
		Args: noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			homeDir, err := home.Detect(false)
			if err != nil {
				return err
			}
			cfg, err := repo.Load(homeDir)
			if err != nil {
				return err
			}
			repos := selectRepos(cfg, repoName, func(r *repo.Repository) bool {
				return r.Sync.Enabled
			})
			if len(repos) == 0 {
				return fmt.Errorf("没有可同步的仓库（请先在 repo.yaml 中启用 sync）")
			}
			var anyErr bool
			for _, r := range repos {
				variants, err := repo.Expand(cfg, r)
				if err != nil {
					return err
				}
				for _, ev := range variants {
					fmt.Fprintf(command.OutOrStdout(), "开始同步: %s\n  URL: %s\n  输出: %s\n", r.Name, ev.URL, ev.ContentRoot(cfg))
					result, err := engine.Sync(withProgress(command), cfg, &ev)
					if err != nil {
						fmt.Fprintf(command.OutOrStderr(), "[ERROR] %s: %v\n", r.Name, err)
						anyErr = true
						continue
					}
					fmt.Fprintf(command.OutOrStdout(), "完成: +%d 跳过 %d 删除 %d 失败 %d（共 %d）\n  索引: %s\n",
						result.Downloaded, result.Skipped, result.Deleted, len(result.Errors), result.Total, result.Repodata)
					for _, e := range result.Errors {
						fmt.Fprintf(command.OutOrStderr(), "  [WARN] %s\n", e)
						anyErr = true
					}
				}
			}
			if anyErr {
				return fmt.Errorf("同步过程中存在部分失败")
			}
			fmt.Fprintln(command.OutOrStdout(), "全部同步完成。")
			return nil
		},
	}
	command.Flags().StringVar(&repoName, "repo", "", "要同步的 repository 名称（留空同步全部启用的）")
	return command
}

// selectRepos filters repositories by predicate, honoring a --repo name filter.
func selectRepos(cfg *repo.Config, name string, pred func(*repo.Repository) bool) []*repo.Repository {
	if name != "" {
		if r, err := cfg.Resolve(name); err == nil && pred(r) {
			return []*repo.Repository{r}
		}
		return nil
	}
	var out []*repo.Repository
	for i := range cfg.Repositories {
		if pred(&cfg.Repositories[i]) {
			out = append(out, &cfg.Repositories[i])
		}
	}
	return out
}
