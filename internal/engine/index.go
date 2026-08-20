package engine

import (
	"context"
	"fmt"

	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/fanhuadesenlinnn/RepoForge/internal/upstream"
)

// loadIndex loads and merges the upstream index for an expanded variant. When
// the variant has multiple aggregate sources (e.g. BaseOS+AppStream), all are
// loaded and merged so dependency solving can draw from any of them.
// needFilelists includes filelists.xml (for dependency resolution); pure
// mirroring passes false to avoid fetching the large filelists unnecessarily.
func loadIndex(ctx context.Context, cfg *repo.Config, ev *repo.Expanded, needFilelists bool) (*upstream.Index, error) {
	backend := ev.Repository.Backend
	seen := map[string]bool{} // dedupe by location
	var merged []upstream.Pkg
	base := ev.Sources[0].URL
	cacheDir := cfg.Paths.CacheDir

	for _, src := range ev.Sources {
		var ix *upstream.Index
		var err error
		if backend == "rpm" {
			if needFilelists {
				ix, err = upstream.RPMIndexForSolve(ctx, src.URL, cacheDir)
			} else {
				ix, err = upstream.RPMIndex(ctx, src.URL, cacheDir)
			}
		} else {
			spec := upstream.DEBSpec{BaseURL: src.URL, Suites: debSuiteSpec(src.Suites)}
			ix, err = upstream.DEBIndex(ctx, spec)
		}
		if err != nil {
			return nil, fmt.Errorf("读取上游 %s 失败: %w", src.URL, err)
		}
		for _, p := range ix.Packages {
			// locations are relative; avoid shipping the same file twice across
			// sources by deduping on location.
			if seen[p.Location] {
				continue
			}
			seen[p.Location] = true
			if p.BaseURL == "" {
				p.BaseURL = ix.BaseURL
			}
			merged = append(merged, p)
		}
	}
	return &upstream.Index{BaseURL: base, Backend: backend, Packages: merged}, nil
}
