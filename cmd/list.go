package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/spf13/cobra"
)

type listedPackage struct {
	Name    string
	Version string
	Release string
	Arch    string
	Path    string
	Size    int64
}

func newListCommand() *cobra.Command {
	var repoName string
	command := &cobra.Command{
		Use:   "list",
		Short: "列出本地离线源中的软件包",
		Long: `按 repo.yaml 列出已制作到本地的软件包。

默认列出所有仓库；可用 --repo 只看其中一个。不依赖本机 rpm/dpkg。`,
		Args: noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			homeDir, cfg, err := loadRepo()
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
			output := command.OutOrStdout()
			var any bool
			for _, r := range repos {
				variants, err := repo.Expand(cfg, r)
				if err != nil {
					return err
				}
				for _, ev := range variants {
					root := ev.ContentRoot(cfg)
					packages, err := listRepositoryPackages(root, r.Backend)
					if err != nil {
						if os.IsNotExist(err) {
							fmt.Fprintf(output, "仓库 %s：目录不存在 %s\n\n", r.Name, root)
							continue
						}
						return err
					}
					any = true
					fmt.Fprintf(output, `仓库: %s
backend: %s
目录: %s
Home: %s
软件包数量: %d
软件包总大小: %s

`, r.Name, r.Backend, root, homeDir, len(packages), humanSize(totalPackageSize(packages)))
					if len(packages) == 0 {
						fmt.Fprintln(output, "未找到软件包。")
						fmt.Fprintln(output)
						continue
					}
					writer := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
					fmt.Fprintln(writer, "NAME\tVERSION\tRELEASE\tARCH\tSIZE\tFILE")
					for _, pkg := range packages {
						fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n",
							pkg.Name, pkg.Version, pkg.Release, pkg.Arch, humanSize(pkg.Size), pkg.Path)
					}
					if err := writer.Flush(); err != nil {
						return err
					}
					fmt.Fprintln(output)
				}
			}
			if !any {
				fmt.Fprintln(output, "本地还没有软件包，请先执行 repoforge sync 或 repoforge make")
			}
			return nil
		},
	}
	command.Flags().StringVar(&repoName, "repo", "", "要列出的 repository 名称")
	return command
}

func listRepositoryPackages(root, backend string) ([]listedPackage, error) {
	ext := ".rpm"
	if backend == "deb" {
		ext = ".deb"
	}
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "repodata" {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || strings.HasSuffix(d.Name(), ".part") {
			return nil
		}
		if strings.EqualFold(filepath.Ext(d.Name()), ext) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	packages := make([]listedPackage, 0, len(files))
	for _, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		rel := path
		if r, rerr := filepath.Rel(root, path); rerr == nil {
			rel = r
		}
		name, ver, rels, arch := parsePackageFileName(filepath.Base(path), backend)
		packages = append(packages, listedPackage{
			Name:    name,
			Version: ver,
			Release: rels,
			Arch:    arch,
			Path:    rel,
			Size:    info.Size(),
		})
	}
	sort.Slice(packages, func(i, j int) bool {
		if packages[i].Name != packages[j].Name {
			return packages[i].Name < packages[j].Name
		}
		if packages[i].Arch != packages[j].Arch {
			return packages[i].Arch < packages[j].Arch
		}
		return packages[i].Path < packages[j].Path
	})
	return packages, nil
}

func parsePackageFileName(base, backend string) (name, ver, rel, arch string) {
	name, ver, rel, arch = base, "-", "-", "-"
	if backend == "deb" {
		base = strings.TrimSuffix(base, ".deb")
		parts := strings.Split(base, "_")
		if len(parts) >= 3 {
			return parts[0], parts[1], "-", parts[len(parts)-1]
		}
		if len(parts) == 2 {
			return parts[0], parts[1], "-", "-"
		}
		return base, "-", "-", "-"
	}
	base = strings.TrimSuffix(base, ".rpm")
	dot := strings.LastIndex(base, ".")
	if dot > 0 {
		arch = base[dot+1:]
		base = base[:dot]
	}
	relAt := strings.LastIndex(base, "-")
	if relAt <= 0 {
		return base, "-", "-", arch
	}
	rel = base[relAt+1:]
	base = base[:relAt]
	verAt := strings.LastIndex(base, "-")
	if verAt <= 0 {
		return base, rel, "-", arch
	}
	return base[:verAt], base[verAt+1:], rel, arch
}

func totalPackageSize(packages []listedPackage) int64 {
	var total int64
	for _, pkg := range packages {
		total += pkg.Size
	}
	return total
}

func humanSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%dB", size)
	}
	value := float64(size)
	units := []string{"KB", "MB", "GB", "TB"}
	for _, suffix := range units {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f%s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1fPB", value/unit)
}
