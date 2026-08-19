package repo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fanhuadesenlinnn/RepoForge/templates"
)

// TestEmbeddedExampleValid ensures the shipped repo.yaml example loads cleanly.
func TestEmbeddedExampleValid(t *testing.T) {
	data, err := templates.Read("repo.yaml")
	if err != nil {
		t.Fatalf("read embedded repo.yaml: %v", err)
	}
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config", "repo.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(home); err != nil {
		t.Fatalf("embedded repo.yaml is invalid: %v", err)
	}
}
