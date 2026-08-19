package upstream

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DefaultClient shared across upstream fetches. A curl-like User-Agent is set
// because several mirror sites (e.g. USTC) fingerprint and block the default
// "Go-http-client/1.1" UA.
func newHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			// reuse connections across downloads
			MaxIdleConns:        64,
			MaxIdleConnsPerHost: 16,
			IdleConnTimeout:     90e9, // 90s
			// don't let a dead mirror block connection setup forever
			ResponseHeaderTimeout: 30e9, // 30s to first response byte
		},
	}
}

// UserAgent reported to mirror sites.
const UserAgent = "curl/8.2.1"

// DefaultClient shared across upstream fetches.
var DefaultClient = newHTTPClient()

// NewClient returns a shared HTTP client with connection reuse and the common
// mirror User-Agent (set per-request). Engine downloaders use this too.
func NewClient() *http.Client { return DefaultClient }

// Fetch downloads a URL fully into memory.
func Fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 %s 失败: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s 返回 HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// FetchToFile downloads a URL to a file, returning the file path.
func FetchToFile(ctx context.Context, url, dst string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 %s 失败: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s 返回 HTTP %d", url, resp.StatusCode)
	}
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// HasSuffixAny reports whether name ends with any of the suffixes.
func HasSuffixAny(name string, suffixes ...string) bool {
	for _, s := range suffixes {
		if strings.HasSuffix(name, s) {
			return true
		}
	}
	return false
}
