package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/fanhuadesenlinnn/RepoForge/internal/upstream"
)

// InstallResult summarizes an install build.
type InstallResult struct {
	Repository string
	URL        string
	Output     string
	Selected   int
	Downloaded int
	Problems   []string
	Repodata   string
}

// Install builds an offline repo containing only the requested packages and
// their resolved dependencies, then generates repodata for the subset.
func Install(ctx context.Context, cfg *repo.Config, ev *repo.Expanded) (*InstallResult, error) {
	if len(ev.Repository.Install.Packages) == 0 {
		return nil, fmt.Errorf("repository %q 未配置 install.packages", ev.Repository.Name)
	}
	root := ev.ContentRoot(cfg)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	// Load the aggregated index across all aggregate sources (e.g. BaseOS+AppStream).
	ix, err := loadIndex(ctx, ev)
	if err != nil {
		return nil, err
	}

	opt := SolveOptions{
		Backend:  ev.Repository.Backend,
		Archs:    archList(ev.Repository),
		WeakDeps: ev.Repository.Dependency.WeakDeps,
	}
	selected, problems := Solve(ix, ev.Repository.Install.Packages, opt)

	// Deduplicate selected packages by location (a package may be selected under
	// multiple provides names, e.g. glibc provides many libc.so.6 variants).
	unique := map[string]upstream.Pkg{}
	for _, p := range selected {
		unique[p.Location] = p
	}
	subset := make([]upstream.Pkg, 0, len(unique))
	for _, p := range unique {
		subset = append(subset, p)
	}

	// Build download plan for selected packages.
	var items []downloadItem
	for _, p := range subset {
		loc := strings.TrimPrefix(p.Location, "/")
		dst := filepath.Join(root, filepath.FromSlash(loc))
		if _, err := os.Stat(dst); err == nil {
			os.Remove(partPath(dst)) // final already present, drop stale partial
			continue
		}
		items = append(items, downloadItem{
			URL:      pkgURL(ix, p),
			Dst:      dst,
			Checksum: p.Checksum,
			Size:     p.Size,
		})
	}
	d := newDownloader(ev.Repository.Sync.Concurrency, true)
	downloaded, errs := d.runAll(ctx, items)
	problems = append(problems, errs...)

	// Generate repodata for the (deduplicated) subset.
	repodata, gerr := genRepodata(ctx, root, subset, ev.Repository.Backend)
	if gerr != nil {
		return nil, gerr
	}

	return &InstallResult{
		Repository: ev.Repository.Name,
		URL:        ev.URL,
		Output:     root,
		Selected:   len(subset),
		Downloaded: downloaded,
		Problems:   dedupe(problems),
		Repodata:   repodata,
	}, nil
}

func archList(r *repo.Repository) []string {
	// Use upstream.Arch if present, else derive from target.Arch + noarch/all,
	// else default to x86_64 + noarch/all (avoids pulling unexpected multilib).
	if len(r.Upstream.Arch) > 0 {
		return append([]string{}, r.Upstream.Arch...)
	}
	if r.Target.Arch != "" {
		if r.Backend == "rpm" {
			return []string{r.Target.Arch, "noarch"}
		}
		return []string{r.Target.Arch, "all"}
	}
	if r.Backend == "rpm" {
		return []string{"x86_64", "noarch"}
	}
	return []string{"amd64", "all"}
}
