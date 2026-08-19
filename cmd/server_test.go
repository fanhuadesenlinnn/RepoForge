package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
)

func TestGenerateClientRepos(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `schema_version: 2
paths:
  repo_dir: ${home}/repos
repositories:
  - name: kylin
    backend: rpm
    upstream:
      url: http://example.invalid/kylin
    sync:
      enabled: true
  - name: debian
    backend: deb
    upstream:
      url: http://example.invalid/debian
      suites:
        - suite: bookworm
          components: [main]
    sync:
      enabled: true
`
	if err := os.WriteFile(filepath.Join(home, "config", "repo.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := repo.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Server.Listen = "0.0.0.0:8080"
	if err := generateClient(cfg); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(home, "client", "repoforge-debian.list"),
		filepath.Join(home, "client", "repoforge-kylin.repo"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("missing client config %s: %v", path, err)
		}
		if !strings.Contains(string(data), "http://127.0.0.1:8080") {
			t.Fatalf("client config lacks base URL: %s", data)
		}
	}
}
