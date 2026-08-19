package engine

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
)

type pkgDef struct {
	path, name, ver, rel, arch string
	content                    string
	requires, provides         string
}

var rpmPkgs = []pkgDef{
	{"Packages/v/vim.rpm", "vim", "8.2", "1", "x86_64", "vim-content", "libc.so.6()(64bit)", "vim"},
	{"Packages/g/glibc.rpm", "glibc", "2.34", "1", "x86_64", "glibc-content", "glibc-common", "libc.so.6()(64bit)"},
	{"Packages/g/glibc-common.rpm", "glibc-common", "2.34", "1", "x86_64", "glibc-common-content", "", ""},
}

func sha(p string) string { h := sha256.Sum256([]byte(p)); return hex.EncodeToString(h[:]) }

func rpmPrimaryXML() []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><metadata packages="3">` + "\n")
	for _, p := range rpmPkgs {
		b.WriteString("<package type=\"rpm\"><name>" + p.name + "</name><arch>" + p.arch + "</arch>")
		b.WriteString("<version epoch=\"0\" ver=\"" + p.ver + "\" rel=\"" + p.rel + "\"/>")
		b.WriteString("<checksum type=\"sha256\">" + sha(p.content) + "</checksum>")
		b.WriteString("<location href=\"" + p.path + "\"/><size package=\"" + itoa(len(p.content)) + "\"/>")
		b.WriteString("<summary>pkg " + p.name + "</summary><format>")
		if p.requires != "" {
			b.WriteString("<requires><entry name=\"" + p.requires + "\"/></requires>")
		}
		if p.provides != "" {
			b.WriteString("<provides><entry name=\"" + p.provides + "\"/></provides>")
		}
		b.WriteString("</format></package>\n")
	}
	b.WriteString("</metadata>")
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write([]byte(b.String()))
	gw.Close()
	return buf.Bytes()
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }

func rpmRepomd() []byte {
	return []byte(`<?xml version="1.0"?><repomd><revision>1</revision><data type="primary"><location href="repodata/x-primary.xml.gz"/></data></repomd>`)
}

func rpmHandler(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "repomd.xml") {
		w.Write(rpmRepomd())
		return
	}
	if strings.HasSuffix(r.URL.Path, "primary.xml.gz") {
		w.Write(rpmPrimaryXML())
		return
	}
	for _, p := range rpmPkgs {
		if strings.HasSuffix(r.URL.Path, p.path) {
			w.Write([]byte(p.content))
			return
		}
	}
	http.NotFound(w, r)
}

func loadConfig(t *testing.T, home, content string) *repo.Config {
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

func TestSyncEndToEnd(t *testing.T) {
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
	result, err := Sync(t.Context(), cfg, &variants[0])
	if err != nil {
		t.Fatal(err)
	}
	if result.Downloaded != 3 {
		t.Fatalf("downloaded = %d, want 3", result.Downloaded)
	}
	for _, p := range rpmPkgs {
		fp := filepath.Join(variants[0].ContentRoot(cfg), strings.TrimPrefix(p.path, "/"))
		if _, err := os.Stat(fp); err != nil {
			t.Errorf("missing %s: %v", fp, err)
		}
	}
	// incremental: second run should skip all
	result2, _ := Sync(t.Context(), cfg, &variants[0])
	if result2.Skipped != 3 {
		t.Fatalf("second run skipped = %d, want 3", result2.Skipped)
	}
}

func TestInstallEndToEnd(t *testing.T) {
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
    install:
      packages: [vim]
`, home, srv.URL)
	cfg := loadConfig(t, home, content)
	r := &cfg.Repositories[0]
	variants, _ := repo.Expand(cfg, r)
	res, err := Install(t.Context(), cfg, &variants[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) != 0 {
		t.Fatalf("problems: %v", res.Problems)
	}
	// vim + glibc + glibc-common (transitive) => 3 selected
	if res.Selected != 3 {
		t.Fatalf("selected = %d, want 3", res.Selected)
	}
	repomd := filepath.Join(variants[0].ContentRoot(cfg), "repodata", "repomd.xml")
	if _, err := os.Stat(repomd); err != nil {
		t.Fatalf("install did not generate repomd.xml: %v", err)
	}
}

var debPkgs = []struct{ path, name, ver, arch, content, depends, provides string }{
	{"pool/main/a/app.deb", "app", "1.0", "amd64", "app-content", "libfoo (>= 2.0)", ""},
	{"pool/main/l/libfoo.deb", "libfoo", "2.1", "amd64", "libfoo-content", "", "libfoo"},
}

func debPackagesText() string {
	var b strings.Builder
	for _, p := range debPkgs {
		b.WriteString("Package: " + p.name + "\n")
		b.WriteString("Version: " + p.ver + "\n")
		b.WriteString("Architecture: " + p.arch + "\n")
		b.WriteString("Filename: " + p.path + "\n")
		b.WriteString("SHA256: " + sha(p.content) + "\n")
		b.WriteString("Size: " + itoa(len(p.content)) + "\n")
		if p.depends != "" {
			b.WriteString("Depends: " + p.depends + "\n")
		}
		if p.provides != "" {
			b.WriteString("Provides: " + p.provides + "\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func debHandler(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.URL.Path, "Packages") {
		w.Write([]byte(debPackagesText()))
		return
	}
	for _, p := range debPkgs {
		if strings.HasSuffix(r.URL.Path, p.path) {
			w.Write([]byte(p.content))
			return
		}
	}
	http.NotFound(w, r)
}

func TestInstallDEBEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(debHandler))
	defer srv.Close()
	home := t.TempDir()
	content := fmt.Sprintf(`schema_version: 2
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
    install:
      packages: [app]
`, home, srv.URL)
	cfg := loadConfig(t, home, content)
	r := &cfg.Repositories[0]
	variants, _ := repo.Expand(cfg, r)
	res, err := Install(t.Context(), cfg, &variants[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) != 0 {
		t.Fatalf("problems: %v", res.Problems)
	}
	// app + libfoo (transitive dep) => 2
	if res.Selected != 2 {
		t.Fatalf("selected = %d, want 2", res.Selected)
	}
	packages := filepath.Join(variants[0].ContentRoot(cfg), "Packages")
	if _, err := os.Stat(packages); err != nil {
		t.Fatalf("install did not generate Packages: %v", err)
	}
}
