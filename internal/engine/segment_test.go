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

	d := newDownloader(4, true)
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
