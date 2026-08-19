package upstream

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientHasTimeouts(t *testing.T) {
	tr, ok := NewClient().Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	if tr.DialContext == nil {
		t.Fatal("DialContext not set")
	}
	if tr.TLSHandshakeTimeout == 0 {
		t.Fatal("TLSHandshakeTimeout not set")
	}
	if tr.ResponseHeaderTimeout == 0 {
		t.Fatal("ResponseHeaderTimeout not set")
	}
	if tr.ForceAttemptHTTP2 {
		t.Fatal("HTTP/2 should be disabled (fragile CDNs stall h2)")
	}
}

func TestFragileURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://update.cs2c.com.cn/NS/V10/V10SP3-2403/os/adv/lic/base/x86_64/", true},
		{"http://update.cs2c.com.cn/foo", true},
		{"https://mirrors.kylinos.cn/kylin/", true},
		{"https://mirrors.aliyun.com/centos/", false},
		{"https://mirrors.rockylinux.org/pub/rocky/9/BaseOS/x86_64/os/", false},
		{"https://deb.debian.org/debian", false},
	}
	for _, c := range cases {
		if got := FragileURL(c.url); got != c.want {
			t.Errorf("FragileURL(%q) = %v, want %v", c.url, got, c.want)
		}
	}
	if !AnyFragile("https://example.com", "https://update.cs2c.com.cn/x") {
		t.Fatal("AnyFragile should detect cs2c among mixed URLs")
	}
	if AnyFragile("https://mirrors.aliyun.com/centos/") {
		t.Fatal("AnyFragile should be false for aliyun")
	}
}

func TestPrepareRequestClosesFragile(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://update.cs2c.com.cn/repodata/repomd.xml", nil)
	PrepareRequest(req)
	if req.Header.Get("User-Agent") != UserAgent {
		t.Fatalf("UA = %q", req.Header.Get("User-Agent"))
	}
	if !req.Close {
		t.Fatal("expected Connection: close on fragile host")
	}

	req2, _ := http.NewRequest(http.MethodGet, "https://mirrors.aliyun.com/centos/", nil)
	PrepareRequest(req2)
	if req2.Close {
		t.Fatal("did not expect Connection: close on friendly host")
	}
}

func TestFetchRetriesAfterReset(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) < 3 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "no hijack", 500)
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				return
			}
			conn.Close()
			return
		}
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	data, err := Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if string(data) != "ok" {
		t.Fatalf("body = %q", data)
	}
	if n.Load() != 3 {
		t.Fatalf("attempts = %d, want 3", n.Load())
	}
}

func TestFetchAbortsHungBody(t *testing.T) {
	old := ReadIdleLimit
	ReadIdleLimit = 200 * time.Millisecond
	t.Cleanup(func() { ReadIdleLimit = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100000")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("partial"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	}))
	defer srv.Close()

	start := time.Now()
	_, err := Fetch(context.Background(), srv.URL)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected hung-body error")
	}
	// 3 attempts * ~200ms idle + small backoff, well under the 3s sleep.
	if elapsed > 2500*time.Millisecond {
		t.Fatalf("hung body took %v, want a fast abort+retry", elapsed)
	}
}

func TestFetchRetriesHTTP500(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) < 2 {
			http.Error(w, "nope", http.StatusBadGateway)
			return
		}
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	data, err := Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ok" {
		t.Fatalf("body = %q", data)
	}
}

func TestIsTransient(t *testing.T) {
	if !IsTransient(io.ErrUnexpectedEOF) {
		t.Fatal("unexpected EOF should be transient")
	}
	if !IsTransient(httpStatusError{url: "u", status: 502}) {
		t.Fatal("502 should be transient")
	}
	if IsTransient(httpStatusError{url: "u", status: 404}) {
		t.Fatal("404 should not be transient")
	}
}
