package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
	"github.com/fanhuadesenlinnn/RepoForge/internal/initialize"
	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
)

func TestHandlerIsReadOnlyAndHidesDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Packages.gz"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler, err := Handler(root, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method string
		path   string
		status int
	}{
		{method: http.MethodGet, path: "/Packages.gz", status: http.StatusOK},
		{method: http.MethodHead, path: "/Packages.gz", status: http.StatusOK},
		{method: http.MethodGet, path: "/", status: http.StatusForbidden},
		{method: http.MethodPost, path: "/Packages.gz", status: http.StatusMethodNotAllowed},
		{method: http.MethodGet, path: "/../secret", status: http.StatusForbidden},
	} {
		request := httptest.NewRequest(test.method, test.path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Errorf("%s %s status = %d, want %d", test.method, test.path, response.Code, test.status)
		}
	}
}

func TestResolveConfiguredPublicURL(t *testing.T) {
	got, candidates, err := ResolvePublicURL(repo.Server{PublicURL: "http://192.0.2.10:8080/"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://192.0.2.10:8080" || candidates != nil {
		t.Fatalf("ResolvePublicURL() = %q, %v", got, candidates)
	}
	if _, _, err := ResolvePublicURL(repo.Server{PublicURL: "not-a-url"}); err == nil {
		t.Fatal("ResolvePublicURL accepted invalid URL")
	}
}

type fakeRunner struct {
	commands []executor.Command
}

func (f *fakeRunner) LookPath(name string) (string, error) {
	return "/usr/bin/" + name, nil
}

func (f *fakeRunner) Run(_ context.Context, command executor.Command) (executor.Result, error) {
	f.commands = append(f.commands, command)
	return executor.Result{Stdout: "active\n"}, nil
}

func TestManagerEnableAndDisable(t *testing.T) {
	home := t.TempDir()
	if err := initialize.Run(home, false); err != nil {
		t.Fatal(err)
	}
	cfg, err := repo.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Server.Systemd.ServiceFile = filepath.Join(t.TempDir(), "repoforge-server.service")
	runner := &fakeRunner{}
	manager := NewManager(runner)
	if err := manager.Enable(context.Background(), cfg, filepath.Join(home, "bin", "repoforge")); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 3 {
		t.Fatalf("enable command count = %d, want 3", len(runner.commands))
	}
	if manager.Status(context.Background(), cfg) != "active" {
		t.Fatal("status is not active")
	}
	if err := manager.Disable(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfg.Server.Systemd.ServiceFile); !os.IsNotExist(err) {
		t.Fatalf("service file still exists: %v", err)
	}
}

// TestManagerEnableWithoutInit ensures server enable works with only the
// embedded systemd template — no `repoforge init`, no config/templates dir.
func TestManagerEnableWithoutInit(t *testing.T) {
	home := t.TempDir()
	// Only repo.yaml exists (simulating init v2 output); no config/templates.
	if err := os.MkdirAll(filepath.Join(home, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config", "repo.yaml"), []byte(`schema_version: 2
paths:
  repo_dir: `+filepath.Join(home, "repos")+`
repositories:
  - name: rocky
    backend: rpm
    upstream:
      url: http://example.invalid
    sync:
      enabled: true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := repo.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "config", "templates")); !os.IsNotExist(err) {
		t.Fatalf("config/templates should not exist in init v2 layout")
	}
	cfg.Server.Systemd.ServiceFile = filepath.Join(t.TempDir(), "repoforge-server.service")
	manager := NewManager(&fakeRunner{})
	if err := manager.Enable(context.Background(), cfg, filepath.Join(home, "bin", "repoforge")); err != nil {
		t.Fatalf("Enable without init failed: %v", err)
	}
	content, err := os.ReadFile(cfg.Server.Systemd.ServiceFile)
	if err != nil {
		t.Fatal(err)
	}
	s := string(content)
	for _, want := range []string{"Description=RepoForge", "REPOFORGE_HOME=", "repoforge server start", "ReadOnlyPaths="} {
		if !strings.Contains(s, want) {
			t.Errorf("service file missing %q:\n%s", want, s)
		}
	}
}
