package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSegmentedDownload(t *testing.T) {
	// A file larger than segThreshold (4 MiB) to exercise segmented path.
	size := 12 << 20 // 12 MiB
	content := make([]byte, size)
	for i := range content {
		content[i] = byte(i % 251)
	}
	sum := sha256.Sum256(content)
	checksum := hex.EncodeToString(sum[:])

	var rangeCount int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rng := r.Header.Get("Range")
		if rng == "" {
			w.Write(content)
			return
		}
		rangeCount++
		var a, b int
		n, _ := fmt.Sscanf(strings.TrimPrefix(rng, "bytes="), "%d-%d", &a, &b)
		if n == 1 {
			b = size - 1
		}
		if b > size-1 {
			b = size - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", a, b, size))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(content[a : b+1])
	}))
	defer srv.Close()

	dir := t.TempDir()
	dst := filepath.Join(dir, "big.rpm")

	d := newDownloader(4, 8, 4, true)
	if err := d.fetch(t.Context(), srv.URL, dst, checksum, int64(size)); err != nil {
		t.Fatalf("segmented fetch: %v", err)
	}
	if rangeCount < 2 {
		t.Fatalf("expected multiple range segments, got %d", rangeCount)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != size {
		t.Fatalf("length = %d, want %d", len(got), size)
	}
	if h := sha256.Sum256(got); hex.EncodeToString(h[:]) != checksum {
		t.Fatalf("checksum mismatch after segmented download")
	}
}

func TestSmallFileNotSegmented(t *testing.T) {
	// 2 MiB < 20 MiB threshold -> should use a single (non-Range) request.
	size := 2 << 20
	content := make([]byte, size)
	for i := range content {
		content[i] = byte(i % 249)
	}
	sum := sha256.Sum256(content)
	checksum := hex.EncodeToString(sum[:])

	var fullReqs, rangeReqs int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			rangeReqs++
		}
		fullReqs++
		w.Write(content)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dst := filepath.Join(dir, "small.rpm")
	// threshold 20 MiB > size 2 MiB -> no segmentation.
	d := newDownloader(4, 8, 20, true)
	if err := d.fetch(t.Context(), srv.URL, dst, checksum, int64(size)); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if rangeReqs != 0 {
		t.Fatalf("expected no Range requests for small file, got %d", rangeReqs)
	}
	if fullReqs != 1 {
		t.Fatalf("expected 1 full request, got %d", fullReqs)
	}
}

// TestSmartSegmentCount verifies segments grow with file size and are capped.
func TestSmartSegmentCount(t *testing.T) {
	mib := int64(1 << 20)
	cases := []struct {
		size, segMiB int64
		max          int
		want         int
	}{
		{20 * mib, 20, 8, 2},  // exactly one segment -> min 2
		{40 * mib, 20, 8, 2},  // 2 segments
		{100 * mib, 20, 8, 5}, // 5 segments
		{200 * mib, 20, 8, 8}, // capped at 8 (would be 10)
		{1 << 30, 20, 8, 8},   // 1 GiB -> capped at 8
	}
	for _, c := range cases {
		n := segmentCount(c.size, c.segMiB<<20, c.max)
		if n != c.want {
			t.Errorf("size=%d segMiB=%d -> segments=%d, want %d", c.size, c.segMiB, n, c.want)
		}
	}
}

func segmentCount(size, segSize int64, max int) int {
	n := int((size + segSize - 1) / segSize)
	if n < 2 {
		n = 2
	}
	if n > max {
		n = max
	}
	return n
}
