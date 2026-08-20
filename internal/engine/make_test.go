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
// Packages/ dir with their metadata parsed, and their deps get downloaded.
func TestMakeInputCopy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(rpmHandler))
	defer srv.Close()
	home := t.TempDir()

	// real local rpm (Requires: vim); copied, not downloaded
	inDir := filepath.Join(home, "input")
	os.MkdirAll(inDir, 0o755)
	copyTestdata(t, filepath.Join(inDir, "mylocal-1.0-1.x86_64.rpm"), "mylocal-1.0-1.x86_64.rpm")

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
	if _, err := os.Stat(filepath.Join(root, "Packages", "mylocal-1.0-1.x86_64.rpm")); err != nil {
		t.Fatalf("input package not copied: %v", err)
	}
	// mylocal's dependency (vim) resolved and downloaded.
	if res.Downloaded != 3 { // vim + glibc + glibc-common
		t.Fatalf("downloaded = %d, want 3 (mylocal's deps)", res.Downloaded)
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
	// real local rpm (Requires: vim); copied, its dep vim is also requested
	copyTestdata(t, filepath.Join(inDir, "mylocal-1.0-1.x86_64.rpm"), "mylocal-1.0-1.x86_64.rpm")

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
	// input file copied into Packages/
	if _, err := os.Stat(filepath.Join(root, "Packages", "mylocal-1.0-1.x86_64.rpm")); err != nil {
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
func TestCollectLocalPkgsArchFilter(t *testing.T) {
	home := t.TempDir()
	inDir := filepath.Join(home, "mixed")
	os.MkdirAll(inDir, 0o755)
	// Real rpm fixtures: x86_64 (Requires: vim) and noarch (Requires: tree).
	copyTestdata(t, filepath.Join(inDir, "mylocal-1.0-1.x86_64.rpm"), "mylocal-1.0-1.x86_64.rpm")
	copyTestdata(t, filepath.Join(inDir, "mylocal-1.0-1.noarch.rpm"), "mylocal-1.0-1.noarch.rpm")

	r := &repo.Repository{
		Backend: "rpm",
		Input:   repo.Input{PackageDirs: []string{inDir}},
	}
	ctx := context.Background()

	// x86_64 variant: both packages match, metadata is parsed.
	root := filepath.Join(home, "out")
	pkgs, copied, skipped, byArch, err := collectLocalPkgs(ctx, r, root, []string{"x86_64", "noarch"}, home)
	if err != nil {
		t.Fatal(err)
	}
	if copied != 2 || skipped != 0 {
		t.Fatalf("copied=%d skipped=%d (%v), want 2/0", copied, skipped, byArch)
	}
	if len(pkgs) != 2 {
		t.Fatalf("localPkgs = %d, want 2", len(pkgs))
	}
	gotReq := map[string]bool{}
	for _, p := range pkgs {
		if !p.Local {
			t.Errorf("package %s must be marked Local", p.Name)
		}
		if !strings.HasPrefix(p.Location, "Packages/") {
			t.Errorf("location %q must be under Packages/", p.Location)
		}
		for _, r := range p.Requires {
			gotReq[r.Name] = true
		}
	}
	if !gotReq["vim"] || !gotReq["tree"] {
		t.Errorf("deps not parsed, got %v", gotReq)
	}
	for _, f := range []string{"mylocal-1.0-1.x86_64.rpm", "mylocal-1.0-1.noarch.rpm"} {
		if _, err := os.Stat(filepath.Join(root, "Packages", f)); err != nil {
			t.Errorf("expected copied file Packages/%s: %v", f, err)
		}
	}

	// aarch64 variant: noarch matches, x86_64 is skipped before parsing.
	root2 := filepath.Join(home, "out2")
	pkgs2, copied2, skipped2, byArch2, err := collectLocalPkgs(ctx, r, root2, []string{"aarch64", "noarch"}, home)
	if err != nil {
		t.Fatal(err)
	}
	if copied2 != 1 || skipped2 != 1 || byArch2["x86_64"] != 1 {
		t.Fatalf("aarch64 copied=%d skipped=%d (%v), want 1/1 x86_64", copied2, skipped2, byArch2)
	}
	if len(pkgs2) != 1 || pkgs2[0].Arch != "noarch" {
		t.Fatalf("aarch64 localPkgs = %+v, want single noarch", pkgs2)
	}
}

func copyTestdata(t *testing.T, dst, src string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", src))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
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

// A third-party local package (not present in upstream metadata) must be
// published into the repo (file + repodata entry) AND its dependencies must
// be resolved and downloaded from upstream.
func TestMakeLocalThirdPartyPublishedWithDeps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(rpmHandler))
	defer srv.Close()
	home := t.TempDir()

	inDir := filepath.Join(home, "local")
	os.MkdirAll(inDir, 0o755)
	copyTestdata(t, filepath.Join(inDir, "mylocal-1.0-1.x86_64.rpm"), "mylocal-1.0-1.x86_64.rpm") // Requires: vim

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

	// Local package file published under Packages/.
	if _, err := os.Stat(filepath.Join(root, "Packages", "mylocal-1.0-1.x86_64.rpm")); err != nil {
		t.Fatalf("local package not published: %v", err)
	}
	// Local package appears in the generated repodata.
	primary := readGeneratedPrimary(t, root)
	if !strings.Contains(primary, "mylocal-1.0-1.x86_64.rpm") {
		t.Error("repodata does not list the local third-party package")
	}
	// Its dependency (vim) plus vim's deps (glibc, glibc-common) were fetched.
	if res.Downloaded != 3 {
		t.Fatalf("downloaded = %d, want 3 (vim + glibc + glibc-common)", res.Downloaded)
	}
	if res.Copied != 1 {
		t.Fatalf("copied = %d, want 1", res.Copied)
	}
	// No problem should remain (vim resolves fine).
	if len(res.Problems) != 0 {
		t.Fatalf("unexpected problems: %v", res.Problems)
	}
}
