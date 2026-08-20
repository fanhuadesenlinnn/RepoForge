package upstream

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestParseLocalRPM(t *testing.T) {
	p, err := ParseLocalPackage(
		filepath.Join("testdata", "mylocal-1.0-1.x86_64.rpm"),
		"Packages/mylocal.rpm", "rpm")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "mylocal" || p.Version != "1.0" || p.Release != "1" || p.Arch != "x86_64" {
		t.Fatalf("unexpected identity: %+v", p)
	}
	if !p.Local {
		t.Error("local flag not set")
	}
	// Requires: vim (rpmlib/rtld virtual deps filtered out).
	found := false
	for _, r := range p.Requires {
		if r.Name == "vim" {
			found = true
		}
		if len(r.Name) >= 7 && r.Name[:7] == "rpmlib(" {
			t.Errorf("rpmlib dep leaked: %q", r.Name)
		}
	}
	if !found {
		t.Errorf("Requires missing vim: %+v", p.Requires)
	}
	if p.Size <= 0 {
		t.Errorf("size not set: %d", p.Size)
	}
}

// buildTestDEB creates a minimal .deb (ar archive with control.tar.gz) in
// memory and returns its bytes.
func buildTestDEB(t *testing.T) []byte {
	t.Helper()
	var ctrl bytes.Buffer
	ctrl.WriteString("Package: hello-local\n")
	ctrl.WriteString("Version: 2.10-1\n")
	ctrl.WriteString("Architecture: amd64\n")
	ctrl.WriteString("Depends: libc6 (>= 2.14), libssl3\n")
	ctrl.WriteString("Provides: hello\n")
	ctrl.WriteString("Description: local test package\n")

	var tarbuf bytes.Buffer
	tw := tar.NewWriter(&tarbuf)
	if err := tw.WriteHeader(&tar.Header{Name: "./control", Mode: 0o644, Size: int64(ctrl.Len())}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(ctrl.Bytes()); err != nil {
		t.Fatal(err)
	}
	tw.Close()

	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write(tarbuf.Bytes())
	zw.Close()

	// ar archive: magic + members (debian-binary, control.tar.gz, data.tar.gz)
	var ar bytes.Buffer
	ar.WriteString("!<arch>\n")
	writeArMember := func(name string, data []byte) {
		hdr := make([]byte, 60)
		copy(hdr[0:16], name)
		for i := len(name); i < 16; i++ {
			hdr[i] = ' '
		}
		copy(hdr[48:58], pad10(len(data)))
		hdr[58], hdr[59] = '`', '\n'
		ar.Write(hdr)
		ar.Write(data)
		if len(data)%2 == 1 {
			ar.WriteByte('\n')
		}
	}
	writeArMember("debian-binary", []byte("2.0\n"))
	writeArMember("control.tar.gz", gz.Bytes())
	writeArMember("data.tar.gz", []byte{})
	return ar.Bytes()
}

func pad10(n int) []byte {
	s := []byte{byte('0' + n/1e9%10), byte('0' + n/1e8%10), byte('0' + n/1e7%10), byte('0' + n/1e6%10),
		byte('0' + n/1e5%10), byte('0' + n/1e4%10), byte('0' + n/1e3%10), byte('0' + n/1e2%10),
		byte('0' + n/10%10), byte('0' + n%10)}
	return s
}

func TestParseLocalDEB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello-local_2.10-1_amd64.deb")
	if err := os.WriteFile(path, buildTestDEB(t), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := ParseLocalPackage(path, "Packages/hello-local.deb", "deb")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "hello-local" || p.Version != "2.10-1" || p.Arch != "amd64" {
		t.Fatalf("unexpected identity: %+v", p)
	}
	if !p.Local {
		t.Error("local flag not set")
	}
	got := map[string]bool{}
	for _, r := range p.Requires {
		got[r.Name] = true
	}
	if !got["libc6"] || !got["libssl3"] {
		t.Errorf("Requires = %+v, want libc6 + libssl3", p.Requires)
	}
}
