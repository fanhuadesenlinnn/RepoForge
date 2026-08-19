package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
)

// QueryInstalled lists packages installed on this machine for the given backend.
func QueryInstalled(ctx context.Context, backend string) ([]InstalledPkg, error) {
	runner := executor.New(false)
	switch backend {
	case "rpm":
		return queryRPMInstalled(ctx, runner)
	case "deb":
		return queryDEBInstalled(ctx, runner)
	default:
		return nil, fmt.Errorf("不支持的 backend %q", backend)
	}
}

func queryRPMInstalled(ctx context.Context, runner executor.Runner) ([]InstalledPkg, error) {
	if _, err := runner.LookPath("rpm"); err != nil {
		return nil, fmt.Errorf("未找到 rpm 命令，make-upgrade 需要在 RPM 系统上运行: %w", err)
	}
	result, err := runner.Run(ctx, executor.Command{
		Name:    "rpm",
		Args:    []string{"-qa", "--qf", "%{NAME}\t%{EPOCH}\t%{VERSION}\t%{RELEASE}\t%{ARCH}\n"},
		Timeout: 2 * time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("读取本机 RPM 已安装列表失败: %w", err)
	}
	var out []InstalledPkg
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 5 {
			continue
		}
		out = append(out, InstalledPkg{
			Name:    f[0],
			Epoch:   f[1],
			Version: f[2],
			Release: f[3],
			Arch:    f[4],
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("rpm -qa 没有返回已安装包")
	}
	return out, nil
}

func queryDEBInstalled(ctx context.Context, runner executor.Runner) ([]InstalledPkg, error) {
	if _, err := runner.LookPath("dpkg-query"); err != nil {
		return nil, fmt.Errorf("未找到 dpkg-query，make-upgrade 需要在 Debian/Ubuntu 系统上运行: %w", err)
	}
	result, err := runner.Run(ctx, executor.Command{
		Name:    "dpkg-query",
		Args:    []string{"-W", "-f", `${Package}\t${Version}\t${Architecture}\n`},
		Timeout: 2 * time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("读取本机 DEB 已安装列表失败: %w", err)
	}
	var out []InstalledPkg
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 3 {
			continue
		}
		epoch, ver := splitDEBVersion(f[1])
		out = append(out, InstalledPkg{
			Name:    f[0],
			Epoch:   epoch,
			Version: ver,
			Arch:    f[2],
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("dpkg-query 没有返回已安装包")
	}
	return out, nil
}

func splitDEBVersion(v string) (epoch, ver string) {
	if i := strings.Index(v, ":"); i >= 0 {
		return v[:i], v[i+1:]
	}
	return "", v
}
