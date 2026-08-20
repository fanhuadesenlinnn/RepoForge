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
	ix, err := loadIndex(ctx, cfg, ev, true)
	if err != nil {
		return nil, err
	}

	// 1) Copy local input packages into the output dir, parse their real
	// metadata (deps included), and let the solver select them as-is and
	// resolve their dependencies. Local packages are published into the repo;
	// they are not just "starting points".
	copied := 0
	skippedLocal := 0
	var localPkgs []upstream.Pkg
	var softRequests []string
	var baseRequests []string // union of input.packages + upgrade names
	baseRequests = append(baseRequests, r.Input.Packages...)

	if len(r.Input.PackageDirs) > 0 {
		pkgs, allNames, n, skipped, _, err := collectLocalPkgs(ctx, r, root, archList(r, ev.Vars, expandedURLs(ev)...), cfg.Paths.HomeDir)
		if err != nil {
			return nil, err
		}
		copied += n
		skippedLocal += skipped
		localPkgs = append(localPkgs, pkgs...)
		// Cross-arch complement: package_dirs may only carry one architecture;
		// request the same names on every variant so variants without a local
		// copy fetch the matching package from upstream. Soft: a third-party
		// package with no upstream counterpart becomes a notice, not an error.
		baseRequests = append(baseRequests, allNames...)
		softRequests = append(softRequests, allNames...)
	}

	opt := SolveOptions{
		Backend:      r.Backend,
		Archs:        archList(r, ev.Vars, expandedURLs(ev)...),
		WeakDeps:     r.Dependency.WeakDeps,
		Conflicts:    r.Dependency.Conflicts,
		LocalPkgs:    localPkgs,
		SoftRequests: softRequests,
	}

	// 2) upgrade_packages: add each name to the solve request so the latest
	// version AND its dependencies are resolved together (Solver picks newest).
	var notices []string
	idx := buildSolveIndex(ix)
	for _, name := range r.Input.UpgradePackages {
		if _, err := latestVersion(idx, name, opt); err != nil {
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

	// 4) Download selected (non-local) packages into root. Local packages were
	// already copied into root/Packages by collectLocalPkgs.
	var items []downloadItem
	for _, p := range subset {
		if p.Local {
			continue // file already present under root/Packages
		}
		loc := strings.TrimPrefix(p.Location, "/")
		dst := filepath.Join(root, filepath.FromSlash(loc))
		if _, err := os.Stat(dst); err == nil {
			os.Remove(partPath(dst))
			continue
		}
		items = append(items, downloadItem{
			URL:       pkgURL(ix, p),
			Dst:       dst,
			Checksum:  p.Checksum,
			VerifyAlg: verifyAlg(r.Upstream.Verify, p.ChecksumType),
			Size:      p.Size,
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
	if serr := signRepodata(ctx, cfg, root, r.Backend); serr != nil {
		return nil, serr
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
