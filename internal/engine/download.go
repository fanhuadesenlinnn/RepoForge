// Package engine implements the build engines: sync (full mirror) and install
// (on-demand with dependency solving). All engines are pure Go and do not rely
// on a host yum/apt.
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/fanhuadesenlinnn/RepoForge/internal/upstream"
)

// downloader downloads packages with bounded concurrency and checksum verify.
type downloader struct {
	client  *http.Client
	jobs    int              // multi-file parallel count
	segment repo.SegmentMode // segment mode: Disabled / Smart / fixed cap
	segSize int64            // base bytes per segment (default 20 MiB)
	resume  bool
}

// newDownloader takes the multi-file concurrency, the per-segment size in MiB,
// and a segment mode (repo.SegmentMode): Smart=auto, Disabled=off, or a fixed
// cap. A fixed cap of n means a large file is split into at most n segments.
func newDownloader(jobs int, segment repo.SegmentMode, segSizeMiB int64, resume bool) *downloader {
	if jobs < 1 {
		jobs = 4
	}
	if segment == 0 {
		segment = repo.SegmentSmart
	}
	if segSizeMiB <= 0 {
		segSizeMiB = 20
	}
	return &downloader{client: upstream.NewClient(), jobs: jobs, segment: segment, segSize: segSizeMiB << 20, resume: resume}
}

// applyFragileTune lowers parallelism for CDNs that reset or throttle the
// Go HTTP stack (official Kylin / cs2c on Tencent EdgeOne).
func (d *downloader) applyFragileTune(urls ...string) {
	if !upstream.AnyFragile(urls...) {
		return
	}
	if d.jobs > upstream.FragileMaxJobs {
		d.jobs = upstream.FragileMaxJobs
	}
	d.segment = repo.SegmentDisabled
}

func expandedURLs(ev *repo.Expanded) []string {
	out := make([]string, 0, len(ev.Sources)+1)
	if ev.URL != "" {
		out = append(out, ev.URL)
	}
	for _, s := range ev.Sources {
		out = append(out, s.URL)
	}
	return out
}

// partPath returns a stable temp path so an interrupted download can resume.
func partPath(dst string) string {
	return filepath.Join(filepath.Dir(dst), "."+filepath.Base(dst)+".part")
}

// fetch one package: supports resuming from a partial download and verifies
// size + checksum when provided.
func (d *downloader) fetch(ctx context.Context, url, dst, checksum string, size int64) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	part := partPath(dst)
	// Existing partial file -> resume from there when enabled.
	var have int64
	if st, err := os.Stat(part); err == nil && d.resume && !st.IsDir() {
		have = st.Size()
	}
	// Segment big files when segmentation is enabled (smart or fixed cap).
	// When explicitly disabled (SegmentDisabled), always use a single connection.
	if have == 0 && size >= d.segSize && d.segment != repo.SegmentDisabled && d.segment != 0 {
		if err := d.segmentedFetch(ctx, url, part, size, checksum); err == nil {
			return os.Rename(part, dst)
		}
		os.Remove(part) // fall through to single connection on any failure
	}

	// Single-connection download with retry: the source CDN intermittently hangs
	// or resets a connection; retry on a fresh connection a couple of times,
	// keeping any partial bytes so a mid-file stall can resume via Range.
	var lastErr error
	for attempt := 0; attempt <= 2; attempt++ {
		lastErr = d.singleFetch(ctx, url, part, have, checksum, size)
		if lastErr == nil {
			return os.Rename(part, dst)
		}
		if ctx.Err() != nil {
			return fmt.Errorf("%s: %w", url, lastErr)
		}
		if d.resume && isResumeWorthy(lastErr) {
			if st, err := os.Stat(part); err == nil && st.Size() > have {
				have = st.Size()
				upstream.CloseIdle()
				continue
			}
		}
		os.Remove(part)
		have = 0
		upstream.CloseIdle()
	}
	return fmt.Errorf("%s: %w", url, lastErr)
}

func isResumeWorthy(err error) bool {
	if err == nil {
		return false
	}
	if upstream.IsTransient(err) {
		return true
	}
	var se shortSizeError
	return errors.As(err, &se) && se.got < se.want
}

type shortSizeError struct{ got, want int64 }

func (e shortSizeError) Error() string {
	return fmt.Sprintf("大小校验失败: 期望 %d 实际 %d", e.want, e.got)
}

// singleFetch performs one request+write of a single-connection download.
func (d *downloader) singleFetch(ctx context.Context, url, part string, have int64, checksum string, size int64) error {
	if err := os.MkdirAll(filepath.Dir(part), 0o755); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	upstream.PrepareRequest(req)
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	if have > 0 && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		return fmt.Errorf("服务器失去部分文件")
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("GET 返回 HTTP %d", resp.StatusCode)
	}
	resp.Body = upstream.IdleBody(resp.Body, cancel)

	f, err := os.OpenFile(part, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	if have > 0 {
		if _, err := f.Seek(have, io.SeekStart); err != nil {
			f.Close()
			return err
		}
	}
	hasher := sha256.New()
	var n int64
	if have > 0 {
		f.Seek(0, io.SeekStart)
		if _, err := io.Copy(hasher, f); err != nil {
			f.Close()
			return err
		}
		f.Seek(have, io.SeekStart)
		w, werr := io.Copy(io.MultiWriter(f, hasher), resp.Body)
		n = have + w
		if werr != nil {
			f.Close()
			return werr
		}
	} else {
		w, werr := io.Copy(io.MultiWriter(f, hasher), resp.Body)
		n = w
		if werr != nil {
			f.Close()
			return werr
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if size > 0 && n != size {
		return shortSizeError{got: n, want: size}
	}
	if checksum != "" && !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), checksum) {
		return fmt.Errorf("SHA256 校验失败")
	}
	return nil
}

// segmentedFetch downloads a large file by splitting it into N concurrent HTTP
// Range requests, each writing to its own offset in the output file. It returns
// an error (so the caller can fall back) if Range is unsupported or any segment
// fails.
func (d *downloader) segmentedFetch(ctx context.Context, url, dst string, size int64, checksum string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	// Probe for Range support with a HEAD/GET Range request on byte 0.
	probe, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	upstream.PrepareRequest(probe)
	probe.Header.Set("Range", "bytes=0-0")
	pr, err := d.client.Do(probe)
	if err != nil {
		return err
	}
	if pr.StatusCode != http.StatusPartialContent {
		pr.Body.Close()
		return fmt.Errorf("server does not support Range (HTTP %d)", pr.StatusCode)
	}
	pr.Body.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if err := out.Truncate(size); err != nil {
		return err
	}

	// Segment count: maximum cap is the fixed value when set, else the smart
	// default (8). Actual count = ceil(size/segSize), capped at that maximum.
	maxSegs := 8
	if d.segment > 0 {
		maxSegs = int(d.segment)
	} else if d.segment != repo.SegmentSmart {
		maxSegs = 1 // disabled or unknown — a single segment
	}
	n := int((size + d.segSize - 1) / d.segSize)
	if n < 2 {
		n = 2
	}
	if n > maxSegs {
		n = maxSegs
	}
	if n < 1 {
		n = 1
	}
	segSize := size / int64(n)
	if segSize < 1 {
		segSize = 1
	}

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		start := int64(i) * segSize
		end := start + segSize - 1
		if i == n-1 {
			end = size - 1
		}
		wg.Add(1)
		go func(idx int, start, end int64) {
			defer wg.Done()
			segCtx, segCancel := context.WithCancel(ctx)
			defer segCancel()
			req, rerr := http.NewRequestWithContext(segCtx, http.MethodGet, url, nil)
			if rerr != nil {
				errs[idx] = rerr
				return
			}
			upstream.PrepareRequest(req)
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
			resp, rerr := d.client.Do(req)
			if rerr != nil {
				errs[idx] = rerr
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusPartialContent {
				errs[idx] = fmt.Errorf("segment %d HTTP %d", idx, resp.StatusCode)
				return
			}
			resp.Body = upstream.IdleBody(resp.Body, segCancel)
			// Each goroutine writes at its fixed offset (no shared cursor).
			f, oerr := os.OpenFile(dst, os.O_WRONLY, 0o644)
			if oerr != nil {
				errs[idx] = oerr
				return
			}
			if _, serr := f.Seek(start, io.SeekStart); serr != nil {
				f.Close()
				errs[idx] = serr
				return
			}
			if _, cerr := io.Copy(f, resp.Body); cerr != nil {
				f.Close()
				errs[idx] = cerr
				return
			}
			if cerr := f.Close(); cerr != nil {
				errs[idx] = cerr
			}
		}(i, start, end)
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			os.Remove(dst)
			return e
		}
	}

	// Verify size and checksum on the assembled file.
	st, err := out.Stat()
	if err != nil {
		os.Remove(dst)
		return err
	}
	if st.Size() != size {
		os.Remove(dst)
		return fmt.Errorf("分段下载大小不符: %d != %d", st.Size(), size)
	}
	if checksum != "" {
		out.Seek(0, io.SeekStart)
		h := sha256.New()
		if _, err := io.Copy(h, out); err != nil {
			os.Remove(dst)
			return err
		}
		if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), checksum) {
			os.Remove(dst)
			return fmt.Errorf("分段下载 SHA256 校验失败")
		}
	}
	return nil
}

// runAll downloads all entries in parallel with bounded concurrency, collecting
// failures. Returns total downloaded and a slice of error strings.
func (d *downloader) runAll(ctx context.Context, items []downloadItem) (downloaded int, errs []string) {
	if len(items) == 0 {
		return 0, nil
	}
	sem := make(chan struct{}, d.jobs)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, it := range items {
		wg.Add(1)
		go func(it downloadItem) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			if err := d.fetch(ctx, it.URL, it.Dst, it.Checksum, it.Size); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Sprintf("%s: %v", it.URL, err))
				mu.Unlock()
				return
			}
			mu.Lock()
			downloaded++
			mu.Unlock()
		}(it)
	}
	wg.Wait()
	return downloaded, errs
}

type downloadItem struct {
	URL      string
	Dst      string
	Checksum string
	Size     int64
}

// downloadNeeded splits packages into (to download, skipped) based on local state.
func planDownloads(ix *upstream.Index, root string, prev state) (items []downloadItem, skipped int) {
	for _, p := range ix.Packages {
		loc := strings.TrimPrefix(p.Location, "/")
		dst := filepath.Join(root, filepath.FromSlash(loc))
		// skip if already present with matching checksum/size
		if st, err := os.Stat(dst); err == nil {
			if p.Checksum != "" && prev.matches(loc, p.Checksum) && st.Size() == p.Size {
				// cleanup any stale partial left from an earlier interrupted run
				os.Remove(partPath(dst))
				skipped++
				continue
			}
		}
		items = append(items, downloadItem{
			URL:      pkgURL(ix, p),
			Dst:      dst,
			Checksum: p.Checksum,
			Size:     p.Size,
		})
	}
	return items, skipped
}

// pkgURL resolves a package's download URL, honoring a per-package source base
// (aggregate repos) and falling back to the index base.
func pkgURL(ix *upstream.Index, p upstream.Pkg) string {
	if p.BaseURL != "" {
		if u := p.Resolve(); u != "" {
			return u
		}
	}
	return ix.ResolveLocation(p.Location)
}
