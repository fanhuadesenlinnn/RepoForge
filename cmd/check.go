package cmd

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/RepoForge/internal/detect"
	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/spf13/cobra"
)

func newCheckCommand() *cobra.Command {
	var repoName string
	command := &cobra.Command{
		Use:   "check",
		Short: "检查环境和 repo.yaml 仓库状态",
		Args:  noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			homeDir, cfg, err := loadRepo()
			if err != nil {
				return err
			}
			output := command.OutOrStdout()
			fmt.Fprintf(output, "[OK] RepoForge Home: %s\n", homeDir)

			for _, path := range []string{cfg.Paths.RepoDir, cfg.Paths.CacheDir, cfg.Paths.ClientDir} {
				if path == "" {
					continue
				}
				if _, err := os.Stat(path); err != nil {
					fmt.Fprintf(output, "[WARN] 路径不存在: %s\n", path)
					continue
				}
				fmt.Fprintf(output, "[OK] 路径存在: %s\n", path)
			}
			fmt.Fprintf(output, "[OK] repo.yaml 有效: schema_version=%d, repositories=%d\n",
				cfg.SchemaVersion, len(cfg.Repositories))

			if system, err := detect.Current(command.Context(), executor.New(false)); err == nil {
				fmt.Fprintf(output, "[OK] 当前系统: %s %s %s (backend=%s)\n",
					system.PrettyName, system.VersionID, system.RawArch, system.Backend)
			} else {
				fmt.Fprintf(output, "[INFO] 非 Linux 或无法探测本机发行版: %v\n", err)
			}

			repos := selectRepos(cfg, repoName, func(*repo.Repository) bool { return true })
			if repoName != "" && len(repos) == 0 {
				return fmt.Errorf("未找到 repository %q", repoName)
			}
			var warned bool
			for _, r := range repos {
				variants, err := repo.Expand(cfg, r)
				if err != nil {
					return err
				}
				for _, ev := range variants {
					root := ev.ContentRoot(cfg)
					fmt.Fprintf(output, "\n仓库 %s (%s)\n  上游: %s\n  目录: %s\n", r.Name, r.Backend, ev.URL, root)
					index := filepath.Join(root, "Packages")
					if r.Backend == "rpm" {
						index = filepath.Join(root, "repodata", "repomd.xml")
					}
					if _, err := os.Stat(index); err == nil {
						fmt.Fprintf(output, "  [OK] 本地索引: %s\n", index)
					} else {
						fmt.Fprintf(output, "  [WARN] 本地索引不存在，请先 sync / make\n")
						warned = true
					}
					if ev.URL != "" && !strings.Contains(ev.URL, "example.invalid") {
						if err := probeUpstream(ev.URL, r.Backend); err != nil {
							fmt.Fprintf(output, "  [WARN] 上游不可达: %v\n", err)
							warned = true
						} else {
							fmt.Fprintf(output, "  [OK] 上游可访问\n")
						}
					}
				}
			}
			if warned {
				fmt.Fprintln(output, "\n检查完成，存在警告。")
				return nil
			}
			fmt.Fprintln(output, "\n检查通过。")
			return nil
		},
	}
	command.Flags().StringVar(&repoName, "repo", "", "只检查指定 repository")
	return command
}

func probeUpstream(base, backend string) error {
	url := strings.TrimRight(base, "/")
	if backend == "rpm" {
		url += "/repodata/repomd.xml"
	} else {
		url += "/dists"
	}
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusMethodNotAllowed {
		return fmt.Errorf("HTTP %d  %s", resp.StatusCode, url)
	}
	return nil
}
