package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/initialize"
)

func TestGenerateClientRepos(t *testing.T) {
	home := t.TempDir()
	if err := initialize.Run(home, false); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := generateClientRepos(home, cfg, "http://192.0.2.10:8080"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(home, "client", "repoforge-debian-12-amd64.list"),
		filepath.Join(home, "client", "repoforge-kylin-v10-sp3-x86_64.repo"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "http://192.0.2.10:8080") {
			t.Fatalf("client config lacks public URL: %s", data)
		}
	}
}
