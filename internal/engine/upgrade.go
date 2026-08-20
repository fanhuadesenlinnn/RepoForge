package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/fanhuadesenlinnn/RepoForge/internal/progress"
	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/fanhuadesenlinnn/RepoForge/internal/upstream"
)

// InstalledPkg is one package currently installed on the build host.
type InstalledPkg struct {
	Name    string
	Epoch   string
	Version string
	Release string
	Arch    string
}

// MakeUpgrade compares installed packages against the upstream index and
// downloads newer versions plus their dependencies into the offline repo.
func MakeUpgrade(ctx context.Context, cfg *repo.Config, ev *repo.Expanded, installed []InstalledPkg) (*MakeResult, error) {
	if len(installed) == 0 {
		return nil, fmt.Errorf("未探测到本机已安装软件包，make-upgrade 需要在目标发行版的 Linux 上运行")
	}
	progress.Infof(ctx, "[升级] 读取上游索引以对照 %d 个已安装包", len(installed))
	ix, err := loadIndex(ctx, cfg, ev, true)
	if err != nil {
		return nil, err
	}
	opt := SolveOptions{
		Backend:   ev.Repository.Backend,
		Archs:     archList(ev.Repository, ev.Vars),
		Conflicts: ev.Repository.Dependency.Conflicts,
	}
	names, notices := upgradesFromInstalled(ix, installed, opt)
	progress.Infof(ctx, "[升级] 发现 %d 个可升级包", len(names))
	if len(names) == 0 {
		root := ev.ContentRoot(cfg)
		return &MakeResult{
			Repository: ev.Repository.Name,
			URL:        ev.URL,
			Output:     root,
			Notices:    notices,
		}, nil
	}

	clone := *ev.Repository
	clone.Input.Packages = nil
	clone.Input.PackageDirs = nil
	clone.Input.UpgradePackages = names
	next := *ev
	next.Repository = &clone
	result, err := Make(ctx, cfg, &next)
	if err != nil {
		return nil, err
	}
	result.Notices = dedupe(append(notices, result.Notices...))
	return result, nil
}

func upgradesFromInstalled(ix *upstream.Index, installed []InstalledPkg, opt SolveOptions) (names []string, notices []string) {
	idx := buildSolveIndex(ix)
	seen := map[string]bool{}
	for _, inst := range installed {
		if inst.Name == "" || seen[inst.Name] {
			continue
		}
		latest, err := latestVersion(idx, inst.Name, opt)
		if err != nil {
			continue
		}
		if !archCompatible(latest.Arch, inst.Arch, opt.Backend) {
			continue
		}
		if pkgNewer(latest, installedAsPkg(inst), opt.Backend) > 0 {
			seen[inst.Name] = true
			names = append(names, inst.Name)
		}
	}
	return names, notices
}

func installedAsPkg(p InstalledPkg) upstream.Pkg {
	return upstream.Pkg{
		Name:    p.Name,
		Epoch:   normalizeEpoch(p.Epoch),
		Version: p.Version,
		Release: p.Release,
		Arch:    p.Arch,
	}
}

func normalizeEpoch(e string) string {
	e = strings.TrimSpace(e)
	if e == "" || e == "(none)" || e == "none" {
		return ""
	}
	return e
}

func archCompatible(got, want, backend string) bool {
	if want == "" || got == "" {
		return true
	}
	if got == want {
		return true
	}
	if backend == "rpm" && (got == "noarch" || want == "noarch") {
		return true
	}
	if backend == "deb" && (got == "all" || want == "all") {
		return true
	}
	return false
}
