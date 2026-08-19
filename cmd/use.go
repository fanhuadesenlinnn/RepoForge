package cmd

import (
	"fmt"
	"os"

	"github.com/fanhuadesenlinnn/RepoForge/internal/home"
	"github.com/fanhuadesenlinnn/RepoForge/internal/privilege"
	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/spf13/cobra"
)

// localRepoFile returns the system repo file path for a repository by backend.
func localRepoFile(r *repo.Repository) string {
	if r.Backend == "rpm" {
		return "/etc/yum.repos.d/repoforge-" + r.Name + ".repo"
	}
	return "/etc/apt/sources.list.d/repoforge-" + r.Name + ".list"
}

// renderLocal returns the local file:// repo config content.
func renderLocal(r *repo.Repository, root string) []byte {
	if r.Backend == "rpm" {
		base := "file://" + root
		return []byte(fmt.Sprintf("[%s]\nname=%s\nbaseurl=%s\nenabled=1\ngpgcheck=%d\n",
			r.Name, "RepoForge "+r.Name, base, boolInt(r.Client.GPGCheck)))
	}
	suite := "stable"
	comp := "main"
	if len(r.Upstream.Suites) > 0 {
		if r.Upstream.Suites[0].Suite != "" {
			suite = r.Upstream.Suites[0].Suite
		}
		if len(r.Upstream.Suites[0].Components) > 0 {
			comp = r.Upstream.Suites[0].Components[0]
		}
	}
	return []byte(fmt.Sprintf("deb file:%s %s %s\n", root, suite, comp))
}

func newUseCommand() *cobra.Command {
	var repoName string
	var disable bool
	var remove bool
	command := &cobra.Command{
		Use:   "use",
		Short: "启用或禁用本机 file:// 软件源（基于 repo.yaml）",
		Args:  noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if remove && !disable {
				return fmt.Errorf("--remove 只能与 --disable 一起使用")
			}
			homeDir, err := home.Detect(false)
			if err != nil {
				return err
			}
			cfg, err := repo.Load(homeDir)
			if err != nil {
				return err
			}
			if err := privilege.RequireRoot("repoforge use 需要写入系统软件源目录", "sudo repoforge use --repo "+repoName); err != nil {
				return err
			}
			repos := selectRepos(cfg, repoName, func(*repo.Repository) bool { return true })
			if len(repos) == 0 {
				return fmt.Errorf("没有可处理的 repository")
			}
			for _, r := range repos {
				path := localRepoFile(r)
				if disable {
					if remove {
						if os.Remove(path) != nil && !os.IsNotExist(err) {
							return err
						}
						fmt.Fprintf(command.OutOrStdout(), "已删除: %s\n", path)
					} else {
						// rename to .disabled
						if _, err := os.Stat(path); err == nil {
							os.Rename(path, path+".disabled")
						}
						fmt.Fprintf(command.OutOrStdout(), "已禁用: %s\n", path)
					}
					continue
				}
				root := cfg.ContentRoot(r)
				content := renderLocal(r, root)
				if err := os.WriteFile(path, content, 0o644); err != nil {
					return err
				}
				fmt.Fprintf(command.OutOrStdout(), "已启用本机源: %s -> %s\n", r.Name, path)
			}
			return nil
		},
	}
	command.Flags().StringVar(&repoName, "repo", "", "要处理的 repository 名称（留空处理全部）")
	command.Flags().BoolVar(&disable, "disable", false, "禁用本机软件源")
	command.Flags().BoolVar(&remove, "remove", false, "禁用时删除配置文件")
	return command
}
