package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/fanhuadesenlinnn/RepoForge/internal/upstream"
)

// SyncResult summarizes one sync run for one expanded variant.
type SyncResult struct {
	Repository string
	URL        string
	Output     string
	Total      int
	Downloaded int
	Skipped    int
	Deleted    int
	Errors     []string
	Repodata   string
}

// Sync mirrors an expanded repository variant fully.
func Sync(ctx context.Context, cfg *repo.Config, ev *repo.Expanded) (*SyncResult, error) {
	root := ev.ContentRoot(cfg)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	// Load the aggregated index across all aggregate sources.
	ix, err := loadIndex(ctx, cfg, ev, false)
	if err != nil {
		return nil, err
	}

	prev := loadState(root)
	items, skipped := planDownloads(ix, root, prev, ev.Repository.Upstream.Verify)
	d := newDownloader(ev.Repository.Sync.Concurrency, ev.Repository.Sync.Segment, ev.Repository.Sync.SegmentThreshold, ev.Repository.Sync.Resume)
	d.applyFragileTune(expandedURLs(ev)...)
	downloaded, errs := d.runAll(ctx, items)

	present := presentPkgs(root, ix.Packages)
	repodata, gerr := genRepodata(ctx, root, present, ev.Repository.Backend)
	if gerr != nil {
		return nil, fmt.Errorf("生成索引失败: %w", gerr)
	}
	if serr := signRepodata(ctx, cfg, root, ev.Repository.Backend); serr != nil {
		return nil, serr
	}

	// persistence: update state with all current packages.
	st := state{Revision: fmt.Sprintf("%d", time.Now().Unix()), SyncedAt: time.Now(), Packages: map[string]string{}}
	for _, p := range ix.Packages {
		st.Packages[p.Location] = p.Checksum
	}

	deleted := 0
	switch ev.Repository.Sync.DeletePolicy {
	case "prune-expired":
		deleted = pruneExpired(root, prev.Packages, st.Packages, ev.Repository.Sync.ExpireDays)
	case "prune":
		deleted = pruneDeleted(root, prev.Packages, st.Packages)
	}
	if err := saveState(root, st); err != nil {
		return nil, err
	}

	return &SyncResult{
		Repository: ev.Repository.Name,
		URL:        ev.URL,
		Output:     root,
		Total:      len(ix.Packages),
		Downloaded: downloaded,
		Skipped:    skipped,
		Deleted:    deleted,
		Errors:     errs,
		Repodata:   repodata,
	}, nil
}

func debSuiteSpec(suites []repo.Suite) []upstream.DEBSuite {
	var out []upstream.DEBSuite
	for _, s := range suites {
		archs := s.Arch
		if len(archs) == 0 {
			archs = []string{"amd64"}
		}
		out = append(out, upstream.DEBSuite{Name: s.Suite, Components: s.Components, Archs: archs})
	}
	return out
}

func pruneDeleted(root string, prev, next map[string]string) int {
	deleted := 0
	for loc := range prev {
		if _, ok := next[loc]; ok {
			continue
		}
		// skip packages present on disk that are still there but not in next
		p := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(loc, "/")))
		if err := os.Remove(p); err == nil {
			deleted++
		}
	}
	return deleted
}

// pruneExpired removes packages upstream no longer provides, but only once
// their local copy is older than the expire window — a grace period so clients
// that still have the package in their local yum/apt cache keep working while
// stale files are cleaned up lazily. expireDays <= 0 falls back to 30.
func pruneExpired(root string, prev, next map[string]string, expireDays int) int {
	if expireDays <= 0 {
		expireDays = 30
	}
	cutoff := time.Now().AddDate(0, 0, -expireDays)
	deleted := 0
	for loc := range prev {
		if _, ok := next[loc]; ok {
			continue
		}
		p := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(loc, "/")))
		st, err := os.Stat(p)
		if err != nil {
			continue // already gone
		}
		if st.ModTime().Before(cutoff) {
			if err := os.Remove(p); err == nil {
				deleted++
			}
		}
	}
	return deleted
}
