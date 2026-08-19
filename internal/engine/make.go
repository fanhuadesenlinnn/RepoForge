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

// MakeResult summarizes a make build.
type MakeResult struct {
	Repository string
	URL        string
	Output     string
	Selected   int
	Downloaded int
	Copied     int
	Problems   []string // hard problems — should surface as errors
	Notices    []string // tolerated soft/virtual dep notes — informational only
	Repodata   string
}

// Make builds an offline repo from the repository's make/input starting points:
// make.packages (explicit), input.package_dirs (pre-existing local packages),
// and input.upgrade_packages (fetch latest version). All resolve through the
// same dependency solver and output into repo_dir/<name>.
func Make(ctx context.Context, cfg *repo.Config, ev *repo.Expanded) (*MakeResult, error) {
	r := ev.Repository
	if len(r.Input.Packages) == 0 && len(r.Input.PackageDirs) == 0 && len(r.Input.UpgradePackages) == 0 {
		return nil, fmt.Errorf("repository %q 未配置 input.packages / input.package_dirs / input.upgrade_packages", r.Name)
	}
	root := ev.ContentRoot(cfg)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	// Load the aggregated index across all aggregate sources.
	ix, err := loadIndex(ctx, ev, true)
	if err != nil {
		return nil, err
	}

	// 1) Copy local input packages into the output dir and collect their names
	// as pre-provided (already available) starting points, so only their missing
	// dependencies get resolved/fetched.
	copied := 0
	var preProvided []string
	var baseRequests []string // union of input.packages + upgrade names
	baseRequests = append(baseRequests, r.Input.Packages...)

	if len(r.Input.PackageDirs) > 0 {
		names, n, err := collectAndCopyInput(r, root)
		if err != nil {
			return nil, err
		}
		copied += n
		preProvided = append(preProvided, names...)
	}

	opt := SolveOptions{
		Backend:     r.Backend,
		Archs:       archList(r),
		WeakDeps:    r.Dependency.WeakDeps,
		PreProvided: preProvided,
	}

	// 2) upgrade_packages: add each name to the solve request so the latest
	// version AND its dependencies are resolved together (Solver picks newest).
	var notices []string
	for _, name := range r.Input.UpgradePackages {
		if _, err := latestVersion(ix, name, opt); err != nil {
			notices = append(notices, err.Error())
			continue
		}
		baseRequests = append(baseRequests, name)
	}

	// 3) Solve over the union of all requests.
	selected, problems, solveNotices := Solve(ix, baseRequests, opt)
	notices = append(notices, solveNotices...)

	// Deduplicate by location.
	mergeLocations := map[string]upstream.Pkg{}
	for _, p := range selected {
		mergeLocations[p.Location] = p
	}
	subset := make([]upstream.Pkg, 0, len(mergeLocations))
	for _, p := range mergeLocations {
		subset = append(subset, p)
	}

	// 4) Download selected (non-local) packages into root.
	var items []downloadItem
	for _, p := range subset {
		loc := strings.TrimPrefix(p.Location, "/")
		dst := filepath.Join(root, filepath.FromSlash(loc))
		if _, err := os.Stat(dst); err == nil {
			os.Remove(partPath(dst))
			continue
		}
		items = append(items, downloadItem{
			URL:      pkgURL(ix, p),
			Dst:      dst,
			Checksum: p.Checksum,
			Size:     p.Size,
		})
	}
	d := newDownloader(r.Sync.Concurrency, r.Sync.Segment, r.Sync.SegmentThreshold, true)
	d.applyFragileTune(expandedURLs(ev)...)
	downloaded, errs := d.runAll(ctx, items)
	problems = append(problems, errs...)

	// 5) Generate repodata over packages that actually landed on disk.
	repodata, gerr := genRepodata(ctx, root, presentPkgs(root, subset), r.Backend)
	if gerr != nil {
		return nil, gerr
	}

	return &MakeResult{
		Repository: r.Name,
		URL:        ev.URL,
		Output:     root,
		Selected:   len(subset),
		Downloaded: downloaded,
		Copied:     copied,
		Problems:   dedupe(problems),
		Notices:    dedupe(notices),
		Repodata:   repodata,
	}, nil
}
