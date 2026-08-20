package upstream

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// UserAgent reported to mirror sites. A curl-like UA is required because
// several mirrors (e.g. USTC) fingerprint and block "Go-http-client/1.1".
const UserAgent = "curl/8.2.1"

// FragileMaxJobs is the concurrency cap applied to CDNs that reset or
// throttle parallel Go-client connections (Tencent EdgeOne in front of
// official Kylin mirrors).
const FragileMaxJobs = 2

const (
	dialTimeout     = 10 * time.Second
	tlsTimeout      = 10 * time.Second
	headerTimeout   = 30 * time.Second
	idlePoolTimeout = 10 * time.Second
	maxFetchTries   = 3
)

// ReadIdleLimit is how long a connection may sit without receiving bytes
// before the request is cancelled and retried. Tests may shorten this.
var ReadIdleLimit = 20 * time.Second

// DefaultClient shared across upstream fetches and engine downloads.
var DefaultClient = newHTTPClient()

func newHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 30 * time.Second,
	}
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				c, err := dialer.DialContext(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				return &deadlineConn{Conn: c}, nil
			},
			ForceAttemptHTTP2:     false,
			TLSNextProto:          map[string]func(string, *tls.Conn) http.RoundTripper{},
			MaxIdleConns:          64,
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       idlePoolTimeout,
			TLSHandshakeTimeout:   tlsTimeout,
			ResponseHeaderTimeout: headerTimeout,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

// connIdleLimit is the TCP-level read/write deadline applied on every I/O. It
// is a constant (not the mutable ReadIdleLimit) so background transport
// goroutines never read a global that tests may shorten — that would be a
// data race. The per-request watchdog (IdleBody) aborts much sooner anyway.
const connIdleLimit = 30 * time.Second

// deadlineConn refreshes a read/write deadline on every I/O so a stalled
// CDN (headers sent, body frozen) cannot hang the client forever.
type deadlineConn struct{ net.Conn }

func (c *deadlineConn) Read(b []byte) (int, error) {
	_ = c.SetReadDeadline(time.Now().Add(connIdleLimit))
	return c.Conn.Read(b)
}

func (c *deadlineConn) Write(b []byte) (int, error) {
	_ = c.SetWriteDeadline(time.Now().Add(connIdleLimit))
	return c.Conn.Write(b)
}

// NewClient returns the shared HTTP client (connection reuse + timeouts).
func NewClient() *http.Client { return DefaultClient }

// CloseIdle drops cached keep-alive connections so the next request dials
// fresh. Needed against CDNs that reset reused Go-client sockets.
func CloseIdle() {
	if tr, ok := DefaultClient.Transport.(*http.Transport); ok {
		tr.CloseIdleConnections()
	}
}

// FragileURL reports whether the host is known to reset or throttle the
// Go HTTP stack (official Kylin / cs2c mirrors behind Tencent EdgeOne).
func FragileURL(raw string) bool {
	host := strings.ToLower(raw)
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		host = strings.ToLower(u.Hostname())
	}
	for _, s := range []string{"cs2c.com.cn", "kylinos.cn"} {
		if strings.Contains(host, s) {
			return true
		}
	}
	return false
}

// AnyFragile reports whether any URL is a fragile-CDN host.
func AnyFragile(urls ...string) bool {
	for _, u := range urls {
		if FragileURL(u) {
			return true
		}
	}
	return false
}

// PrepareRequest sets the shared User-Agent and, for fragile CDNs, disables
// keep-alive so a reset idle socket cannot poison the next request.
func PrepareRequest(req *http.Request) {
	req.Header.Set("User-Agent", UserAgent)
	if req.URL != nil && FragileURL(req.URL.String()) {
		req.Close = true
		req.Header.Set("Connection", "close")
	}
}

// IdleBody wraps a response body so that silence longer than ReadIdleLimit
// cancels the request context (which unblocks Read). cancel must belong to
// the request's context. The limit is snapshotted at creation so background
// reads never touch the mutable global (see connIdleLimit for the same idea).
func IdleBody(body io.ReadCloser, cancel context.CancelFunc) io.ReadCloser {
	if body == nil {
		return body
	}
	limit := ReadIdleLimit
	t := time.AfterFunc(limit, cancel)
	return &idleBody{ReadCloser: body, timer: t, cancel: cancel, limit: limit}
}

type idleBody struct {
	io.ReadCloser
	timer  *time.Timer
	cancel context.CancelFunc
	limit  time.Duration
}

func (b *idleBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.timer.Reset(b.limit)
	}
	return n, err
}

func (b *idleBody) Close() error {
	b.timer.Stop()
	return b.ReadCloser.Close()
}

// Fetch downloads a URL fully into memory, retrying transient failures
// (connection reset, TLS timeout, hung body). Some CDNs, e.g. Kylin on
// Tencent EdgeOne, reset Go-client connections that the transport tries
// to reuse; a fresh dial usually succeeds.
func Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < maxFetchTries; attempt++ {
		if err := waitAttempt(ctx, attempt); err != nil {
			return nil, err
		}
		data, err := fetchOnce(ctx, rawURL)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !IsTransient(err) {
			return nil, fmt.Errorf("请求 %s 失败: %w", rawURL, err)
		}
		CloseIdle()
	}
	return nil, fmt.Errorf("请求 %s 多次重试失败: %w", rawURL, lastErr)
}

func fetchOnce(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, cancel, err := do(ctx, DefaultClient, req)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, httpStatusError{url: rawURL, status: resp.StatusCode}
	}
	return io.ReadAll(resp.Body)
}

func do(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(ctx)
	PrepareRequest(req)
	req = req.WithContext(ctx)
	resp, err := client.Do(req)
	if err != nil {
		cancel()
		return nil, func() {}, err
	}
	resp.Body = IdleBody(resp.Body, cancel)
	return resp, cancel, nil
}

func waitAttempt(ctx context.Context, attempt int) error {
	if attempt == 0 {
		return nil
	}
	d := time.Duration(attempt) * 200 * time.Millisecond
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

type httpStatusError struct {
	url    string
	status int
}

func (e httpStatusError) Error() string {
	return fmt.Sprintf("GET %s 返回 HTTP %d", e.url, e.status)
}

// IsTransient reports whether an error is a retryable transport failure
// (reset, TLS/handshake timeout, hung body, 5xx).
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.ErrNoProgress) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	// Watchdog cancel / idle deadline look like context errors. The caller
	// must have already checked that the parent context is still alive.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var se httpStatusError
	if errors.As(err, &se) {
		return se.status >= 500
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "connection reset") ||
		strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "TLS handshake timeout") ||
		strings.Contains(s, "i/o timeout")
}
