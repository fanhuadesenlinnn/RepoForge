package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveAllWithin(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "cache", "installroot")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RemoveAllWithin(root, child); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(child); !os.IsNotExist(err) {
		t.Fatalf("target still exists: %v", err)
	}
	if err := RemoveAllWithin(root, root); err == nil {
		t.Fatal("RemoveAllWithin(root, root) error = nil")
	}
	if err := RemoveAllWithin(root, filepath.Dir(root)); err == nil {
		t.Fatal("RemoveAllWithin accepted parent path")
	}
}
