package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
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
	var profileName string
	command := &cobra.Command{
		Use:   "list",
		Short: "列出本地软件源中的软件包",
		Long: `列出当前 profile 软件源目录中的软件包。

RPM backend 会优先使用 rpm 查询包头，输出包名、版本、发布号、架构和大小。
DEB backend 当前按 deb 文件名和大小输出。`,
		Args: noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			homeDir, _, profile, runner, err := loadProfileInputs(command.Context(), profileName)
			if err != nil {
				return err
			}

			packages, err := listRepositoryPackages(command.Context(), runner, profile)
			if err != nil {
				return err
			}

			output := command.OutOrStdout()
			fmt.Fprintf(output, `本地软件源软件包列表。

profile: %s
backend: %s
软件源目录: %s
RepoForge Home: %s
软件包数量: %d
软件包总大小: %s
`, profile.Profile, profile.Backend, profile.Repository.PackageDir, homeDir, len(packages), humanSize(totalPackageSize(packages)))
			if len(packages) == 0 {
				fmt.Fprintln(output, "\n未找到软件包。")
				return nil
			}

			fmt.Fprintln(output)
			writer := tabwriter.NewWriter(output, 0, 0, 2, ' ', 0)
			fmt.Fprintln(writer, "NAME\tVERSION\tRELEASE\tARCH\tSIZE\tFILE")
			for _, pkg := range packages {
				fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n",
					pkg.Name,
					pkg.Version,
					pkg.Release,
					pkg.Arch,
					humanSize(pkg.Size),
					filepath.Base(pkg.Path),
				)
			}
			return writer.Flush()
		},
	}
	command.Flags().StringVar(&profileName, "profile", "", "要列出软件包的 profile 名称（留空自动匹配当前系统）")
	return command
}

func listRepositoryPackages(ctx context.Context, runner executor.Runner, profile *config.ProfileConfig) ([]listedPackage, error) {
	extension := ".rpm"
	if profile.Backend == "deb" {
		extension = ".deb"
	}

	files, err := packageFiles(profile.Repository.PackageDir, extension)
	if err != nil {
		return nil, err
	}

	packages := make([]listedPackage, 0, len(files))
	for _, path := range files {
		pkg, err := packageInfo(ctx, runner, profile.Backend, path)
		if err != nil {
			return nil, err
		}
		packages = append(packages, pkg)
	}

	sort.Slice(packages, func(i, j int) bool {
		if packages[i].Name != packages[j].Name {
			return packages[i].Name < packages[j].Name
		}
		if packages[i].Arch != packages[j].Arch {
			return packages[i].Arch < packages[j].Arch
		}
		if packages[i].Version != packages[j].Version {
			return packages[i].Version < packages[j].Version
		}
		return packages[i].Release < packages[j].Release
	})
	return packages, nil
}

func packageFiles(dir, extension string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读取软件源目录失败 %s: %w", dir, err)
	}
	files := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.EqualFold(filepath.Ext(name), extension) {
			files = append(files, filepath.Join(dir, name))
		}
	}
	sort.Strings(files)
	return files, nil
}

func packageInfo(ctx context.Context, runner executor.Runner, backendName, path string) (listedPackage, error) {
	info, err := os.Stat(path)
	if err != nil {
		return listedPackage{}, fmt.Errorf("读取软件包文件失败 %s: %w", path, err)
	}

	pkg := listedPackage{
		Name:    strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Version: "-",
		Release: "-",
		Arch:    "-",
		Path:    path,
		Size:    info.Size(),
	}

	if backendName != "rpm" {
		return pkg, nil
	}

	metadata, err := rpmPackageMetadata(ctx, runner, path)
	if err != nil {
		// 文件名兜底，避免单个损坏 RPM 直接导致 list 不可用。
		pkg.Release = "读取失败"
		return pkg, nil
	}
	pkg.Name = metadata.Name
	pkg.Version = metadata.Version
	pkg.Release = metadata.Release
	pkg.Arch = metadata.Arch
	return pkg, nil
}

type rpmMetadata struct {
	Name    string
	Version string
	Release string
	Arch    string
}

func rpmPackageMetadata(ctx context.Context, runner executor.Runner, path string) (rpmMetadata, error) {
	if _, err := runner.LookPath("rpm"); err != nil {
		return rpmMetadata{}, err
	}
	result, err := runner.Run(ctx, executor.Command{
		Name:    "rpm",
		Args:    []string{"-qp", "--qf", "%{NAME}\t%{VERSION}\t%{RELEASE}\t%{ARCH}\n", path},
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return rpmMetadata{}, err
	}
	fields := strings.Split(strings.TrimSpace(result.Stdout), "\t")
	if len(fields) != 4 {
		return rpmMetadata{}, fmt.Errorf("RPM 元数据格式异常: %s", strings.TrimSpace(result.Stdout))
	}
	return rpmMetadata{
		Name:    fields[0],
		Version: fields[1],
		Release: fields[2],
		Arch:    fields[3],
	}, nil
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
