package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/RepoForge/internal/progress"
)

func TestRenderDownloadBar(t *testing.T) {
	var buf bytes.Buffer
	// Partial batch: bar filled proportionally, no trailing newline.
	renderDownloadBar(&buf, progress.Download{Done: 5, Total: 20, Status: "完成", Name: "vim-common-9.0-45.p05.ky10.x86_64.rpm"})
	got := buf.String()
	if !strings.HasPrefix(got, "\r[下载] █████░░") {
		t.Errorf("bar head = %q", got)
	}
	if !strings.Contains(got, "25%") {
		t.Errorf("missing percentage: %q", got)
	}
	if !strings.Contains(got, "5/20") {
		t.Errorf("missing count: %q", got)
	}
	if strings.HasSuffix(got, "\n") {
		t.Errorf("partial batch must not emit newline: %q", got)
	}

	// Long file names are truncated with an ellipsis.
	buf.Reset()
	longName := strings.Repeat("x", 100) + ".rpm"
	renderDownloadBar(&buf, progress.Download{Done: 1, Total: 1, Status: "完成", Name: longName})
	if strings.Contains(buf.String(), strings.Repeat("x", 100)) {
		t.Errorf("long name not truncated: %q", buf.String())
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Errorf("final event must emit newline: %q", buf.String())
	}
}

func TestRenderDownloadBarComplete(t *testing.T) {
	var buf bytes.Buffer
	renderDownloadBar(&buf, progress.Download{Done: 20, Total: 20, Status: "完成", Name: "a.rpm"})
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Errorf("complete batch must end with newline: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "100%") {
		t.Errorf("complete batch must show 100%%: %q", buf.String())
	}
}

func TestIsTerminal(t *testing.T) {
	var buf bytes.Buffer
	if isTerminal(&buf) {
		t.Error("bytes.Buffer must not be a terminal")
	}
}
