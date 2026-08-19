package upstream

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func rpmRepomd() string {
	return `<?xml version="1.0"?>
<repomd>
  <revision>123</revision>
  <data type="primary">
    <location href="repodata/abc-primary.xml.gz"/>
  </data>
</repomd>`
}

func rpmPrimary() []byte {
	xml := `<?xml version="1.0"?>
<metadata xmlns="http://linux.duke.edu/metadata/common">
  <package type="rpm">
    <name>vim-enhanced</name>
    <arch>x86_64</arch>
    <version epoch="0" ver="8.2" rel="1.el9"/>
    <checksum type="sha256" pkgid="YES">AAAA</checksum>
    <location href="Packages/v/vim-enhanced-8.2-1.el9.x86_64.rpm"/>
    <size package="123456"/>
    <summary>VIM editor</summary>
    <format>
      <requires><entry name="libc.so.6()(64bit)"/></requires>
      <provides><entry name="vim"/></provides>
      <recommends><entry name="vim-common"/></recommends>
    </format>
  </package>
</metadata>`
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write([]byte(xml))
	w.Close()
	return buf.Bytes()
}

func TestRPMIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "repomd.xml"):
			w.Write([]byte(rpmRepomd()))
		case strings.HasSuffix(r.URL.Path, "primary.xml.gz"):
			w.Write(rpmPrimary())
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ix, err := RPMIndex(t.Context(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if ix.Backend != "rpm" {
		t.Fatalf("backend = %q", ix.Backend)
	}
	if len(ix.Packages) != 1 {
		t.Fatalf("packages = %d", len(ix.Packages))
	}
	p := ix.Packages[0]
	if p.Name != "vim-enhanced" || p.Arch != "x86_64" || p.Version != "8.2" || p.Release != "1.el9" {
		t.Fatalf("unexpected pkg: %+v", p)
	}
	if p.Location != "Packages/v/vim-enhanced-8.2-1.el9.x86_64.rpm" {
		t.Fatalf("location = %q", p.Location)
	}
	if len(p.Requires) != 1 || p.Requires[0].Name != "libc.so.6()(64bit)" {
		t.Fatalf("requires = %+v", p.Requires)
	}
	if len(p.Provides) != 1 || p.Provides[0] != "vim" {
		t.Fatalf("provides = %+v", p.Provides)
	}
}

func debPackages() string {
	return `Package: vim
Version: 2:8.2.4918-1
Architecture: amd64
Filename: pool/main/v/vim/vim_8.2.4918-1_amd64.deb
SHA256: BBBB
Size: 2000000
Depends: libc6 (>= 2.15), vim-common (= 2:8.2.4918-1)
Provides: editor
Description: Vi IMproved

Package: vim-common
Version: 2:8.2.4918-1
Architecture: all
Filename: pool/main/v/vim/vim-common_8.2.4918-1_all.deb
SHA256: CCCC
Size: 3000000
Depends: vim-runtime (= 2:8.2.4918-1)
Description: vim common
`
}

func TestDEBIndex(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "Packages") {
			w.Write([]byte(debPackages()))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	spec := DEBSpec{
		BaseURL: srv.URL,
		Suites: []DEBSuite{
			{Name: "bookworm", Components: []string{"main"}, Archs: []string{"amd64"}},
		},
	}
	ix, err := DEBIndex(t.Context(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Packages) != 2 {
		t.Fatalf("packages = %d", len(ix.Packages))
	}
	var vim *Pkg
	for i := range ix.Packages {
		if ix.Packages[i].Name == "vim" {
			vim = &ix.Packages[i]
		}
	}
	if vim == nil {
		t.Fatal("vim not found")
	}
	if vim.Epoch != "2" || vim.Version != "8.2.4918-1" {
		t.Fatalf("vim version = %+v", vim)
	}
	if len(vim.Requires) != 2 {
		t.Fatalf("vim requires = %+v", vim.Requires)
	}
	if vim.Requires[0].Name != "libc6" || vim.Requires[0].Op != ">=" {
		t.Fatalf("requires[0] = %+v", vim.Requires[0])
	}
	if len(vim.Provides) != 1 || vim.Provides[0] != "editor" {
		t.Fatalf("provides = %+v", vim.Provides)
	}
	// filename resolution
	if got := ix.ResolveLocation(vim.Location); !strings.HasSuffix(got, "/pool/main/v/vim/vim_8.2.4918-1_amd64.deb") {
		t.Fatalf("resolve = %q", got)
	}
}
