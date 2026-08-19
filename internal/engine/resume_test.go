package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
)

// TestResume verifies that an interrupted download resumes via Range and passes
// the final checksum. The server serves a 1000-byte file, honors Range, and
// counts how many full vs range requests arrive.
func TestResume(t *testing.T) {
	content := []byte(strings.Repeat("abcdefghij", 100)) // 1000 bytes
	sum := sha256.Sum256(content)
	checksum := hex.EncodeToString(sum[:])

	var fullReqs, rangeReqs int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rng := r.Header.Get("Range")
		if rng == "" {
			atomic.AddInt64(&fullReqs, 1)
			w.Write(content)
			return
		}
		atomic.AddInt64(&rangeReqs, 1)
		// parse "bytes=N-"
		var from int64
		fmt.Sscanf(strings.TrimPrefix(rng, "bytes="), "%d-", &from)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, len(content)-1, len(content)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(content[from:])
	}))
	defer srv.Close()

	dir := t.TempDir()
	dst := filepath.Join(dir, "pkg.rpm")
	part := partPath(dst)

	d := newDownloader(2, repo.SegmentMode(8), 20, true)

	// First call (no partial) should do a full request.
	if err := d.fetch(context.Background(), srv.URL, dst, checksum, int64(len(content))); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	full := atomic.LoadInt64(&fullReqs)
	if full != 1 {
		t.Fatalf("expected 1 full request, got %d", full)
	}
	_ = os.Remove(dst) // remove final, keep none

	// Simulate a partial download: write half the file to .part, then fetch again.
	os.MkdirAll(filepath.Dir(dst), 0o755)
	os.WriteFile(part, content[:500], 0o644)
	if err := d.fetch(context.Background(), srv.URL, dst, checksum, int64(len(content))); err != nil {
		t.Fatalf("resume fetch: %v", err)
	}
	rr := atomic.LoadInt64(&rangeReqs)
	if rr != 1 {
		t.Fatalf("expected 1 range request on resume, got %d", rr)
	}
	// final file must be complete and match checksum
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read final: %v", err)
	}
	if len(got) != len(content) {
		t.Fatalf("final length = %d, want %d", len(got), len(content))
	}
	if h := sha256.Sum256(got); hex.EncodeToString(h[:]) != checksum {
		t.Fatalf("checksum mismatch after resume")
	}
}
