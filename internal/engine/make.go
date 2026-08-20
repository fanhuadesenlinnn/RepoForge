package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fanhuadesenlinnn/RepoForge/internal/progress"
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
	// SkippedLocal counts local package_dirs files not copied because their
	// architecture does not match this variant (e.g. x86_64 rpm in a directory
	// scanned while building the aarch64 variant).
	SkippedLocal int
	Problems     []string // hard problems — should surface as errors
	Notices      []string // tolerated soft/virtual dep notes — informational only
	Repodata     string
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
	skippedLocal := 0
	var preProvided []string
	var localCopies map[string]string // package name -> flat file name copied into root
	var baseRequests []string         // union of input.packages + upgrade names
	baseRequests = append(baseRequests, r.Input.Packages...)

	if len(r.Input.PackageDirs) > 0 {
		var names []string
		var n, skipped int
		var err error
		names, localCopies, n, skipped, _, err = collectAndCopyInput(ctx, r, root, archList(r, ev.Vars), cfg.Paths.HomeDir)
		if err != nil {
			return nil, err
		}
		copied += n
		skippedLocal += skipped
		preProvided = append(preProvided, names...)
	}

	opt := SolveOptions{
		Backend:     r.Backend,
		Archs:       archList(r, ev.Vars),
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
	// Local package_dirs copies landed flat in root. When the upstream index
	// carries the same NEVRA (same file name), move the local copy to the
	// upstream Location so the on-disk layout matches the generated repodata;
	// when the versions differ, drop the local copy and fetch the upstream one
	// (with a notice), so repodata and disk stay consistent.
	var items []downloadItem
	for _, p := range subset {
		loc := strings.TrimPrefix(p.Location, "/")
		dst := filepath.Join(root, filepath.FromSlash(loc))
		if _, err := os.Stat(dst); err == nil {
			os.Remove(partPath(dst))
			continue
		}
		if flat, ok := localCopies[p.Name]; ok {
			flatPath := filepath.Join(root, flat)
			if filepath.Base(flat) == filepath.Base(loc) {
				// Same NEVRA: adopt the local copy at the upstream location.
				if err := os.MkdirAll(filepath.Dir(dst), 0o755); err == nil {
					if err := os.Rename(flatPath, dst); err == nil {
						continue
					}
				}
			}
			// Version differs from upstream (or move failed): discard the
			// local copy and fetch the upstream version for index consistency.
			os.Remove(flatPath)
			progress.Infof(ctx, "[输入] 本地包 %s 与上游版本不同（上游 %s），已采用上游版本", flat, filepath.Base(loc))
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
		Repository:   r.Name,
		URL:          ev.URL,
		Output:       root,
		Selected:     len(subset),
		Downloaded:   downloaded,
		Copied:       copied,
		SkippedLocal: skippedLocal,
		Problems:     dedupe(problems),
		Notices:      dedupe(notices),
		Repodata:     repodata,
	}, nil
}
