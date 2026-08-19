package upstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
// Fetch downloads a URL fully into memory, retrying on transient connection
// resets (some CDNs, e.g. Kylin, actively reset Go client connections that the
// transport tries to reuse; a fresh request usually succeeds).
func Fetch(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", UserAgent)
		resp, err := DefaultClient.Do(req)
		if err != nil {
			lastErr = err
			if isTransient(err) {
				continue
			}
			return nil, fmt.Errorf("请求 %s 失败: %w", url, err)
		}
		if resp.StatusCode == http.StatusOK {
			data, rerr := io.ReadAll(resp.Body)
			resp.Body.Close()
			return data, rerr
		}
		lastErr = fmt.Errorf("GET %s 返回 HTTP %d", url, resp.StatusCode)
		resp.Body.Close()
		if resp.StatusCode >= 500 && attempt < 2 {
			continue // transient server error, retry
		}
		return nil, lastErr
	}
	return nil, fmt.Errorf("请求 %s 多次重试失败: %w", url, lastErr)
}

// isTransient reports whether an error is a retryable transport failure
// (connection reset / unexpected EOF / temporary network error).
func isTransient(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "connection reset") || strings.Contains(s, "broken pipe")
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
