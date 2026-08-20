package engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/fanhuadesenlinnn/RepoForge/internal/sign"
	"github.com/fanhuadesenlinnn/RepoForge/internal/upstream"
)

func sha1hex(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func gzipBytes(data []byte) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write(data)
	gw.Close()
	return buf.Bytes()
}

func TestSyncSignsRPMRepomd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(rpmHandler))
	defer srv.Close()
	home := t.TempDir()
	priv, _, _, err := sign.Generate("", "")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "config", "signing")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "private.key"), priv, 0o600); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf(`schema_version: 2
signing:
  enabled: true
paths:
  repo_dir: %s/repos
repositories:
  - name: rocky
    backend: rpm
    upstream:
      url: %s
    sync:
      enabled: true
`, home, srv.URL)
	cfg := loadConfig(t, home, content)
	r := &cfg.Repositories[0]
	variants, _ := repo.Expand(cfg, r)
	if _, err := Sync(context.Background(), cfg, &variants[0]); err != nil {
		t.Fatal(err)
	}
	root := variants[0].ContentRoot(cfg)
	asc := filepath.Join(root, "repodata", "repomd.xml.asc")
	data, err := os.ReadFile(asc)
	if err != nil {
		t.Fatalf("sync did not sign repomd: %v", err)
	}
	if !strings.Contains(string(data), "BEGIN PGP SIGNATURE") {
		t.Fatalf("repomd.xml.asc not an armored signature")
	}
}

func TestMakeSignsDEBRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(debHandler))
	defer srv.Close()
	home := t.TempDir()
	priv, _, _, err := sign.Generate("", "")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, "config", "signing")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "private.key"), priv, 0o600); err != nil {
		t.Fatal(err)
	}
	content := fmt.Sprintf(`schema_version: 2
signing:
  enabled: true
paths:
  repo_dir: %s/repos
repositories:
  - name: debian
    backend: deb
    upstream:
      url: %s
      suites:
        - suite: bookworm
          components: [main]
      arch: [amd64]
    input:
      packages: [app]
`, home, srv.URL)
	cfg := loadConfig(t, home, content)
	r := &cfg.Repositories[0]
	variants, _ := repo.Expand(cfg, r)
	if _, err := Make(context.Background(), cfg, &variants[0]); err != nil {
		t.Fatal(err)
	}
	root := variants[0].ContentRoot(cfg)
	for _, f := range []string{"Release", "InRelease", "Release.gpg"} {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Fatalf("missing %s: %v", f, err)
		}
	}
	release, err := os.ReadFile(filepath.Join(root, "Release"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(release), "SHA256:") {
		t.Fatalf("Release missing SHA256 stanza:\n%s", release)
	}
	if !strings.Contains(string(release), "Packages") {
		t.Fatalf("Release missing Packages entry:\n%s", release)
	}
	// InRelease must be a clearsigned document wrapping the Release body.
	in, err := os.ReadFile(filepath.Join(root, "InRelease"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(in)
	if !strings.Contains(s, "BEGIN PGP SIGNED MESSAGE") || !strings.Contains(s, "BEGIN PGP SIGNATURE") {
		t.Fatalf("InRelease is not clearsigned:\n%s", s)
	}
	// Release.gpg is an armored detached signature.
	gpg, err := os.ReadFile(filepath.Join(root, "Release.gpg"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gpg), "BEGIN PGP SIGNATURE") {
		t.Fatalf("Release.gpg is not an armored signature")
	}
}

func TestSyncSkipsSigningWhenDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(rpmHandler))
	defer srv.Close()
	home := t.TempDir()
	content := fmt.Sprintf(`schema_version: 2
paths:
  repo_dir: %s/repos
repositories:
  - name: rocky
    backend: rpm
    upstream:
      url: %s
    sync:
      enabled: true
`, home, srv.URL)
	cfg := loadConfig(t, home, content)
	r := &cfg.Repositories[0]
	variants, _ := repo.Expand(cfg, r)
	if _, err := Sync(context.Background(), cfg, &variants[0]); err != nil {
		t.Fatal(err)
	}
	root := variants[0].ContentRoot(cfg)
	if _, err := os.Stat(filepath.Join(root, "repodata", "repomd.xml.asc")); !os.IsNotExist(err) {
		t.Fatalf("signing disabled but repomd.xml.asc exists")
	}
}

func TestPruneExpiredKeepsRecent(t *testing.T) {
	root := t.TempDir()
	recent := filepath.Join(root, "Packages/r/recent.rpm")
	stale := filepath.Join(root, "Packages/s/stale.rpm")
	if err := os.MkdirAll(filepath.Dir(recent), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().AddDate(0, 0, -60)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	prev := map[string]string{"Packages/r/recent.rpm": "a", "Packages/s/stale.rpm": "b"}
	next := map[string]string{} // upstream no longer has either
	n := pruneExpired(root, prev, next, 30)
	if n != 1 {
		t.Fatalf("deleted = %d, want 1 (only the stale one)", n)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale file should have been removed")
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatalf("recent file should have been kept")
	}
}

func TestPruneExpiredDefaultWindow(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "Packages/s/s.rpm")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().AddDate(0, 0, -45)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	prev := map[string]string{"Packages/s/s.rpm": "a"}
	if n := pruneExpired(root, prev, map[string]string{}, 0); n != 1 {
		t.Fatalf("expireDays=0 (default 30) should delete a 45-day-old file, deleted=%d", n)
	}
}

func TestVerifyAlgSelection(t *testing.T) {
	cases := []struct{ mode, pkgType, want string }{
		{"auto", "", "sha256"},
		{"auto", "sha1", "sha1"},
		{"auto", "md5", "md5"},
		{"sha256", "sha1", "sha256"}, // explicit wins over metadata
		{"sha1", "md5", "sha1"},
		{"SHA1", "", "sha1"},
		{"", "md5", "md5"},
	}
	for _, c := range cases {
		if got := verifyAlg(c.mode, c.pkgType); got != c.want {
			t.Errorf("verifyAlg(%q, %q) = %q, want %q", c.mode, c.pkgType, got, c.want)
		}
	}
}

func TestConflictsResolveFindsSatisfyingVersion(t *testing.T) {
	pkgs := []upstream.Pkg{
		{Name: "appA", Version: "1.0", Release: "1", Arch: "x86_64", Location: "Packages/a/appA.rpm",
			Requires: []upstream.DependencyEntry{{Name: "libfoo", Op: ">=", Version: "2.0"}}},
		{Name: "appB", Version: "1.0", Release: "1", Arch: "x86_64", Location: "Packages/a/appB.rpm",
			Requires: []upstream.DependencyEntry{{Name: "libfoo", Op: "<", Version: "3.0"}}},
		{Name: "libfoo", Version: "2.5", Release: "1", Arch: "x86_64", Location: "Packages/l/libfoo-2.5.rpm", Provides: []string{"libfoo"}},
		{Name: "libfoo", Version: "3.5", Release: "1", Arch: "x86_64", Location: "Packages/l/libfoo-3.5.rpm", Provides: []string{"libfoo"}},
	}
	ix := &upstream.Index{Backend: "rpm", Packages: pkgs}

	// report mode: the conflict surfaces as a problem.
	_, problems, _ := Solve(ix, []string{"appA", "appB"}, SolveOptions{Backend: "rpm", Conflicts: "report"})
	var conflictFound bool
	for _, p := range problems {
		if strings.Contains(p, "依赖冲突") {
			conflictFound = true
		}
	}
	if !conflictFound {
		t.Fatalf("report mode should surface a conflict, problems=%v", problems)
	}

	// resolve mode: 2.5 satisfies both >=2.0 and <3.0 -> no conflict.
	sel, problems, _ := Solve(ix, []string{"appA", "appB"}, SolveOptions{Backend: "rpm", Conflicts: "resolve"})
	for _, p := range problems {
		if strings.Contains(p, "依赖冲突") {
			t.Fatalf("resolve mode should find a satisfying version, problems=%v", problems)
		}
	}
	var libfoo *upstream.Pkg
	for i := range sel {
		p := sel[i]
		if p.Name == "libfoo" {
			libfoo = &p
		}
	}
	if libfoo == nil || libfoo.Version != "2.5" {
		t.Fatalf("expected libfoo 2.5 selected, got %+v", libfoo)
	}
}

func TestConflictsResolveGivesUpWhenImpossible(t *testing.T) {
	pkgs := []upstream.Pkg{
		{Name: "appA", Version: "1.0", Release: "1", Arch: "x86_64", Location: "Packages/a/appA.rpm",
			Requires: []upstream.DependencyEntry{{Name: "libfoo", Op: ">=", Version: "3.0"}}},
		{Name: "appB", Version: "1.0", Release: "1", Arch: "x86_64", Location: "Packages/a/appB.rpm",
			Requires: []upstream.DependencyEntry{{Name: "libfoo", Op: "<", Version: "3.0"}}},
		{Name: "libfoo", Version: "2.5", Release: "1", Arch: "x86_64", Location: "Packages/l/libfoo-2.5.rpm", Provides: []string{"libfoo"}},
		{Name: "libfoo", Version: "3.5", Release: "1", Arch: "x86_64", Location: "Packages/l/libfoo-3.5.rpm", Provides: []string{"libfoo"}},
	}
	ix := &upstream.Index{Backend: "rpm", Packages: pkgs}
	sel, problems, _ := Solve(ix, []string{"appA", "appB"}, SolveOptions{Backend: "rpm", Conflicts: "resolve"})
	if len(sel) == 0 {
		t.Fatalf("expected selected packages even when conflict impossible")
	}
	var conflictFound bool
	for _, p := range problems {
		if strings.Contains(p, "依赖冲突") {
			conflictFound = true
		}
	}
	if !conflictFound {
		t.Fatalf("impossible conflict should still be reported, problems=%v", problems)
	}
}

func TestSyncVerifyAutoSHA1(t *testing.T) {
	// Upstream primary declares sha256 checksums; force verify: auto and
	// confirm download+verify succeeds with the metadata algorithm.
	srv := httptest.NewServer(http.HandlerFunc(rpmHandler))
	defer srv.Close()
	home := t.TempDir()
	content := fmt.Sprintf(`schema_version: 2
paths:
  repo_dir: %s/repos
repositories:
  - name: rocky
    backend: rpm
    upstream:
      url: %s
      verify: auto
    sync:
      enabled: true
`, home, srv.URL)
	cfg := loadConfig(t, home, content)
	r := &cfg.Repositories[0]
	variants, _ := repo.Expand(cfg, r)
	res, err := Sync(context.Background(), cfg, &variants[0])
	if err != nil {
		t.Fatal(err)
	}
	if res.Downloaded != 3 || len(res.Errors) != 0 {
		t.Fatalf("auto verify sync failed: downloaded=%d errors=%v", res.Downloaded, res.Errors)
	}
}

func TestSyncVerifyForcedSHA256FailsOnSHA1Upstream(t *testing.T) {
	// A repo whose packages are checksummed with sha1, but verify is forced
	// to sha256: verification must fail and surface as an error.
	sha1Primary := func() []byte {
		var b strings.Builder
		b.WriteString(`<?xml version="1.0"?><metadata packages="3">` + "\n")
		for _, p := range rpmPkgs {
			b.WriteString("<package type=\"rpm\"><name>" + p.name + "</name><arch>" + p.arch + "</arch>")
			b.WriteString("<version epoch=\"0\" ver=\"" + p.ver + "\" rel=\"" + p.rel + "\"/>")
			b.WriteString("<checksum type=\"sha1\">" + sha1hex(p.content) + "</checksum>")
			b.WriteString("<location href=\"" + p.path + "\"/><size package=\"" + itoa(len(p.content)) + "\"/>")
			b.WriteString("<summary>pkg " + p.name + "</summary><format></format></package>\n")
		}
		b.WriteString("</metadata>")
		return gzipBytes([]byte(b.String()))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "repomd.xml") {
			w.Write(rpmRepomd())
			return
		}
		if strings.HasSuffix(r.URL.Path, "primary.xml.gz") {
			w.Write(sha1Primary())
			return
		}
		for _, p := range rpmPkgs {
			if strings.HasSuffix(r.URL.Path, p.path) {
				w.Write([]byte(p.content))
				return
			}
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	home := t.TempDir()
	content := fmt.Sprintf(`schema_version: 2
paths:
  repo_dir: %s/repos
repositories:
  - name: rocky
    backend: rpm
    upstream:
      url: %s
      verify: sha256
    sync:
      enabled: true
`, home, srv.URL)
	cfg := loadConfig(t, home, content)
	r := &cfg.Repositories[0]
	variants, _ := repo.Expand(cfg, r)
	res, err := Sync(context.Background(), cfg, &variants[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) == 0 {
		t.Fatalf("forced sha256 verify against sha1 upstream should produce errors")
	}
	if !strings.Contains(strings.Join(res.Errors, " "), "校验失败") {
		t.Fatalf("errors should mention checksum failure: %v", res.Errors)
	}
}
