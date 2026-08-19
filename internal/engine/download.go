// Package engine implements the build engines: sync (full mirror) and install
// (on-demand with dependency solving). All engines are pure Go and do not rely
// on a host yum/apt.
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fanhuadesenlinnn/RepoForge/internal/upstream"
)

// downloader downloads packages with bounded concurrency and checksum verify.
type downloader struct {
	client      *http.Client
	jobs        int   // multi-file parallel count
	maxSegments int   // cap on segments per large file
	segSize     int64 // base bytes per segment (default 20 MiB)
	resume      bool
}

// newDownloader takes the multi-file concurrency, the per-segment size in MiB,
// and the max segments cap. Segments per file are computed automatically:
// ceil(fileSize/segSize) capped at maxSegments.
func newDownloader(jobs, maxSegments int, segSizeMiB int64, resume bool) *downloader {
	if jobs < 1 {
		jobs = 4
	}
	if maxSegments < 1 {
		maxSegments = 8
	}
	if segSizeMiB <= 0 {
		segSizeMiB = 20
	}
	return &downloader{client: upstream.NewClient(), jobs: jobs, maxSegments: maxSegments, segSize: segSizeMiB << 20, resume: resume}
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
	// For fresh big files (>= one segment), download with segmented parallel.
	if have == 0 && size >= d.segSize && d.maxSegments > 1 {
		if err := d.segmentedFetch(ctx, url, part, size, checksum); err == nil {
			return os.Rename(part, dst)
		}
		os.Remove(part) // fall through to single connection on any failure
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", upstream.UserAgent)
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求 %s 失败: %w", url, err)
	}
	defer resp.Body.Close()
	if have > 0 && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		// Server lost the partial file; restart from scratch.
		os.Remove(part)
		return d.fetch(ctx, url, dst, checksum, size)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("GET %s 返回 HTTP %d", url, resp.StatusCode)
	}
	// Wrap body with a read-idle timeout so a hung connection aborts rather
	// than blocking the whole download forever.
	resp.Body = struct {
		io.Reader
		io.Closer
	}{idleReader{r: resp.Body}, resp.Body}

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
	// If we resumed with a Range request, the bytes written do not start at 0,
	// so compute the full checksum over existing + appended by seeking back.
	var n int64
	if have > 0 {
		// hash the existing prefix
		f.Seek(0, io.SeekStart)
		if _, err := io.Copy(hasher, f); err != nil {
			f.Close()
			return err
		}
		// then append new bytes at the end
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
		os.Remove(part)
		return fmt.Errorf("大小校验失败 %s: 期望 %d 实际 %d", url, size, n)
	}
	if checksum != "" && !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), checksum) {
		os.Remove(part)
		return fmt.Errorf("SHA256 校验失败 %s", url)
	}
	return os.Rename(part, dst)
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
	probe.Header.Set("User-Agent", upstream.UserAgent)
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

	// Segment count grows with file size but is capped: ceil(size/segSize),
	// at most maxSegments. Guarantees progress and avoids thread explosion.
	n := int((size + d.segSize - 1) / d.segSize)
	if n < 2 {
		n = 2
	}
	if n > d.maxSegments {
		n = d.maxSegments
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
			req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if rerr != nil {
				errs[idx] = rerr
				return
			}
			req.Header.Set("User-Agent", upstream.UserAgent)
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
			resp.Body = struct {
				io.Reader
				io.Closer
			}{idleReader{r: resp.Body}, resp.Body}
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
