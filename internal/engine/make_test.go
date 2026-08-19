package engine

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
)

// TestMakeInputCopy verifies input.package_dirs files are copied into the output
// root and their names are treated as starting points.
func TestMakeInputCopy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(rpmHandler))
	defer srv.Close()
	home := t.TempDir()

	// create an input dir with a fake "tree" rpm (Copied, not downloaded)
	inDir := filepath.Join(home, "input")
	os.MkdirAll(inDir, 0o755)
	os.WriteFile(filepath.Join(inDir, "tree-1.7.0-1.el8.x86_64.rpm"), []byte("fake-tree"), 0o644)

	content := "schema_version: 2\n" +
		"paths:\n  repo_dir: " + home + "/repos\n" +
		"repositories:\n" +
		"  - name: x\n    backend: rpm\n" +
		"    upstream:\n      url: " + srv.URL + "\n" +
		"    input:\n      package_dirs: [" + inDir + "]\n"
	cfg := loadConfigForMake(t, home, content)
	r := &cfg.Repositories[0]
	variants, _ := repo.Expand(cfg, r)
	res, err := Make(t.Context(), cfg, &variants[0])
	if err != nil {
		t.Fatalf("make: %v", err)
	}
	if res.Copied != 1 {
		t.Fatalf("copied = %d, want 1", res.Copied)
	}
	root := variants[0].ContentRoot(cfg)
	if _, err := os.Stat(filepath.Join(root, "tree-1.7.0-1.el8.x86_64.rpm")); err != nil {
		t.Fatalf("input package not copied: %v", err)
	}
}

func loadConfigForMake(t *testing.T, home, content string) *repo.Config {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config", "repo.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := repo.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// TestMakeCombinedInputs verifies the three input starting points can be used
// together: packages + package_dirs + upgrade_packages union into one build.
func TestMakeCombinedInputs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(rpmHandler)) // serves vim/glibc/glibc-common
	defer srv.Close()
	home := t.TempDir()

	inDir := filepath.Join(home, "pkgs")
	os.MkdirAll(inDir, 0o755)
	// pre-existing local vim-common rpm (copied; vim's dep)
	os.WriteFile(filepath.Join(inDir, "vim-common-8.2-1.el8.x86_64.rpm"), []byte("lok"), 0o644)

	content := "schema_version: 2\n" +
		"paths:\n  repo_dir: " + home + "/repos\n" +
		"repositories:\n" +
		"  - name: x\n    backend: rpm\n" +
		"    upstream:\n      url: " + srv.URL + "\n" +
		"    input:\n" +
		"      packages: [vim]\n" +
		"      package_dirs: [" + inDir + "]\n" +
		"      upgrade_packages: [vim]\n"
	cfg := loadConfigForMake(t, home, content)
	r := &cfg.Repositories[0]
	variants, _ := repo.Expand(cfg, r)
	res, err := Make(t.Context(), cfg, &variants[0])
	if err != nil {
		t.Fatalf("make: %v", err)
	}
	if res.Copied != 1 {
		t.Fatalf("copied = %d, want 1", res.Copied)
	}
	root := variants[0].ContentRoot(cfg)
	// input file copied
	if _, err := os.Stat(filepath.Join(root, "vim-common-8.2-1.el8.x86_64.rpm")); err != nil {
		t.Fatalf("input package not copied: %v", err)
	}
	// Repodata generated with no hard problems.
	repomd := filepath.Join(root, "repodata", "repomd.xml")
	if _, err := os.Stat(repomd); err != nil {
		t.Fatalf("repomd not generated: %v", err)
	}
	if len(res.Problems) != 0 {
		t.Fatalf("problems: %v", res.Problems)
	}
}
