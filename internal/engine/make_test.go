package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	res, err := Make(context.Background(), cfg, &variants[0])
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
	res, err := Make(context.Background(), cfg, &variants[0])
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

// archList must honor the expanded variant's $basearch so multi-arch configs
// (same repo expanded into x86_64 and aarch64) solve against the right arch.
func TestArchListFromVariantBasearch(t *testing.T) {
	r := &repo.Repository{Backend: "rpm"}
	cases := []struct {
		vars map[string]string
		want []string
	}{
		{nil, []string{"x86_64", "noarch"}},
		{map[string]string{"basearch": "x86_64"}, []string{"x86_64", "noarch"}},
		{map[string]string{"basearch": "aarch64"}, []string{"aarch64", "noarch"}},
		{map[string]string{"basearch": "aarch64"}, []string{"aarch64", "noarch"}},
	}
	for _, c := range cases {
		got := archList(r, c.vars)
		if len(got) != len(c.want) {
			t.Fatalf("archList(%v) = %v, want %v", c.vars, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("archList(%v) = %v, want %v", c.vars, got, c.want)
			}
		}
	}
	// Explicit upstream.arch and target.arch still take priority.
	rt := &repo.Repository{Backend: "rpm", Target: repo.Target{Arch: "s390x"}}
	if got := archList(rt, map[string]string{"basearch": "aarch64"}); got[0] != "s390x" {
		t.Fatalf("target.arch should win, got %v", got)
	}
	ru := &repo.Repository{Backend: "rpm", Upstream: repo.Upstream{Arch: []string{"ppc64le"}}}
	if got := archList(ru, map[string]string{"basearch": "aarch64"}); got[0] != "ppc64le" {
		t.Fatalf("upstream.arch should win, got %v", got)
	}
}

// collectAndCopyInput must only copy packages whose architecture matches the
// variant's arch set (noarch always matches), and report skipped ones, so a
// mixed-arch package_dirs directory works per variant.
func TestCollectAndCopyInputArchFilter(t *testing.T) {
	home := t.TempDir()
	inDir := filepath.Join(home, "mixed")
	os.MkdirAll(inDir, 0o755)
	for _, f := range []string{
		"vim-enhanced-9.0-1.ky10.x86_64.rpm",
		"glibc-2.28-1.ky10.x86_64.rpm",
		"tzdata-2022a-1.ky10.noarch.rpm",
		"bash-5.0-1.ky10.aarch64.rpm",
	} {
		os.WriteFile(filepath.Join(inDir, f), []byte("x"), 0o644)
	}
	r := &repo.Repository{
		Backend: "rpm",
		Input:   repo.Input{PackageDirs: []string{inDir}},
	}
	root := filepath.Join(home, "out")
	ctx := context.Background()

	// x86_64 variant: copies x86_64 + noarch, skips aarch64.
	names, _, copied, skipped, byArch, err := collectAndCopyInput(ctx, r, root, []string{"x86_64", "noarch"}, home)
	if err != nil {
		t.Fatal(err)
	}
	if copied != 3 {
		t.Fatalf("copied = %d, want 3", copied)
	}
	if skipped != 1 || byArch["aarch64"] != 1 {
		t.Fatalf("skipped = %d (%v), want 1 aarch64", skipped, byArch)
	}
	got := map[string]bool{}
	for _, n := range names {
		got[n] = true
	}
	for _, want := range []string{"vim-enhanced", "glibc", "tzdata"} {
		if !got[want] {
			t.Errorf("missing pre-provided name %q in %v", want, names)
		}
	}
	for _, f := range []string{"vim-enhanced-9.0-1.ky10.x86_64.rpm", "glibc-2.28-1.ky10.x86_64.rpm", "tzdata-2022a-1.ky10.noarch.rpm"} {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Errorf("expected copied file %s: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "bash-5.0-1.ky10.aarch64.rpm")); err == nil {
		t.Error("aarch64 rpm must not be copied into x86_64 variant")
	}

	// aarch64 variant: copies aarch64 + noarch, skips x86_64.
	root2 := filepath.Join(home, "out2")
	_, _, copied2, skipped2, byArch2, err := collectAndCopyInput(ctx, r, root2, []string{"aarch64", "noarch"}, home)
	if err != nil {
		t.Fatal(err)
	}
	if copied2 != 2 {
		t.Fatalf("aarch64 copied = %d, want 2", copied2)
	}
	if skipped2 != 2 || byArch2["x86_64"] != 2 {
		t.Fatalf("aarch64 skipped = %d (%v), want 2 x86_64", skipped2, byArch2)
	}
}

// pkgArchFromFile parses RPM and DEB architectures from file names.
func TestPkgArchFromFile(t *testing.T) {
	cases := []struct{ file, backend, want string }{
		{"vim-common-9.0-45.p05.ky10.aarch64.rpm", "rpm", "aarch64"},
		{"tzdata-2022a-15.p01.ky10.noarch.rpm", "rpm", "noarch"},
		{"ImageMagick-devel-6.9.13.28-2.ky10.x86_64.rpm", "rpm", "x86_64"},
		{"vim_9.0_amd64.deb", "deb", "amd64"},
		{"libc6_2.36-9_all.deb", "deb", "all"},
	}
	for _, c := range cases {
		if got := pkgArchFromFile(c.file, c.backend); got != c.want {
			t.Errorf("pkgArchFromFile(%q, %s) = %q, want %q", c.file, c.backend, got, c.want)
		}
	}
}

// A local package_dirs copy with the same NEVRA as an upstream package must be
// adopted at the upstream Location (no duplicate download, no flat leftover),
// and its dependencies still get fetched.
func TestMakeLocalCopySameVersionAdopted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(rpmHandler))
	defer srv.Close()
	home := t.TempDir()

	inDir := filepath.Join(home, "local")
	os.MkdirAll(inDir, 0o755)
	os.WriteFile(filepath.Join(inDir, "vim.rpm"), []byte("local-vim-content"), 0o644)

	content := "schema_version: 2\npaths:\n  repo_dir: " + home + "/repos\n" +
		"repositories:\n  - name: x\n    backend: rpm\n    upstream:\n      url: " + srv.URL + "\n" +
		"    input:\n      package_dirs: [" + inDir + "]\n"
	cfg := loadConfigForMake(t, home, content)
	r := &cfg.Repositories[0]
	variants, _ := repo.Expand(cfg, r)
	res, err := Make(context.Background(), cfg, &variants[0])
	if err != nil {
		t.Fatal(err)
	}
	root := variants[0].ContentRoot(cfg)

	data, err := os.ReadFile(filepath.Join(root, "Packages/v/vim.rpm"))
	if err != nil {
		t.Fatalf("vim not adopted at upstream location: %v", err)
	}
	if string(data) != "local-vim-content" {
		t.Fatalf("vim content = %q, want the local copy", data)
	}
	if _, err := os.Stat(filepath.Join(root, "vim.rpm")); err == nil {
		t.Error("flat duplicate vim.rpm must not remain")
	}
	if res.Downloaded != 2 { // only vim's deps: glibc + glibc-common
		t.Fatalf("downloaded = %d, want 2 (deps only, vim from local)", res.Downloaded)
	}
	// repodata must reference the adopted local vim.
	if !strings.Contains(readGeneratedPrimary(t, root), "vim.rpm") {
		t.Error("repodata does not list vim.rpm")
	}
}

// A local copy whose version differs from upstream is discarded and the
// upstream version fetched instead, so disk layout and repodata stay consistent.
func TestMakeLocalCopyVersionMismatchUsesUpstream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(rpmHandler))
	defer srv.Close()
	home := t.TempDir()

	inDir := filepath.Join(home, "local")
	os.MkdirAll(inDir, 0o755)
	// Different file name (different NEVRA) than upstream vim.rpm.
	os.WriteFile(filepath.Join(inDir, "vim-9.0-1.ky10.x86_64.rpm"), []byte("local-vim-content"), 0o644)

	content := "schema_version: 2\npaths:\n  repo_dir: " + home + "/repos\n" +
		"repositories:\n  - name: x\n    backend: rpm\n    upstream:\n      url: " + srv.URL + "\n" +
		"    input:\n      package_dirs: [" + inDir + "]\n"
	cfg := loadConfigForMake(t, home, content)
	r := &cfg.Repositories[0]
	variants, _ := repo.Expand(cfg, r)
	res, err := Make(context.Background(), cfg, &variants[0])
	if err != nil {
		t.Fatal(err)
	}
	root := variants[0].ContentRoot(cfg)

	data, err := os.ReadFile(filepath.Join(root, "Packages/v/vim.rpm"))
	if err != nil {
		t.Fatalf("upstream vim missing: %v", err)
	}
	if string(data) != "vim-content" {
		t.Fatalf("vim content = %q, want the upstream copy", data)
	}
	if _, err := os.Stat(filepath.Join(root, "vim-9.0-1.ky10.x86_64.rpm")); err == nil {
		t.Error("discarded local copy must not remain")
	}
	if res.Downloaded != 3 { // vim + glibc + glibc-common
		t.Fatalf("downloaded = %d, want 3", res.Downloaded)
	}
}
