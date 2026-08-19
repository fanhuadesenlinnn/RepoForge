package engine

import (
	"testing"

	"github.com/fanhuadesenlinnn/RepoForge/internal/upstream"
)

func mkPkg(name, ver, rel, arch, loc string, req, prov []upstream.DependencyEntry, provides []string) upstream.Pkg {
	return upstream.Pkg{Name: name, Version: ver, Release: rel, Arch: arch, Location: loc, Requires: req, Provides: provides}
}

func dep(name, op, ver string) upstream.DependencyEntry {
	return upstream.DependencyEntry{Name: name, Op: op, Version: ver}
}

func TestSolveRPMTransitive(t *testing.T) {
	ix := &upstream.Index{Backend: "rpm", Packages: []upstream.Pkg{
		mkPkg("vim", "8.2", "1", "x86_64", "P/vim.rpm",
			[]upstream.DependencyEntry{dep("libc.so.6()(64bit)", "", "")}, nil, []string{"vim"}),
		mkPkg("glibc", "2.34", "1", "x86_64", "P/glibc.rpm",
			[]upstream.DependencyEntry{dep("glibc-common", "", "")}, nil, []string{"libc.so.6()(64bit)"}),
		mkPkg("glibc-common", "2.34", "1", "x86_64", "P/glibc-common.rpm", nil, nil, nil),
		mkPkg("other-libc", "2.28", "1", "x86_64", "P/other.rpm", nil, nil, []string{"libc.so.6()(64bit)"}),
	}}
	selected, problems := Solve(ix, []string{"vim"}, SolveOptions{Backend: "rpm", WeakDeps: false})
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	// Expect vim, glibc, glibc-common (transitive). Should NOT pick other-libc (older).
	locToName := map[string]string{}
	for _, p := range selected {
		locToName[p.Location] = p.Name
	}
	for _, want := range []string{"vim", "glibc", "glibc-common"} {
		if _, ok := locToName[wantLocation(t, want)]; !ok {
			t.Errorf("missing %s in selected: %v", want, locToName)
		}
	}
	if _, ok := locToName["P/other.rpm"]; ok {
		t.Errorf("should not select older provider other-libc")
	}
}

func wantLocation(t *testing.T, name string) string {
	switch name {
	case "vim":
		return "P/vim.rpm"
	case "glibc":
		return "P/glibc.rpm"
	case "glibc-common":
		return "P/glibc-common.rpm"
	}
	t.Fatalf("unknown name %q", name)
	return ""
}

func TestSolveDEBVersionConstraint(t *testing.T) {
	ix := &upstream.Index{Backend: "deb", Packages: []upstream.Pkg{
		mkPkg("app", "1.0", "", "amd64", "pool/a/app.deb", []upstream.DependencyEntry{dep("libfoo", ">=", "2.0")}, nil, nil),
		mkPkg("libfoo", "1.5", "", "amd64", "pool/l/libfoo15.deb", nil, nil, nil),
		mkPkg("libfoo", "2.1", "", "amd64", "pool/l/libfoo21.deb", nil, nil, nil),
	}}
	selected, problems := Solve(ix, []string{"app"}, SolveOptions{Backend: "deb"})
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	var libfoo *upstream.Pkg
	for _, p := range selected {
		if p.Name == "libfoo" {
			libfoo = &p
		}
	}
	if libfoo == nil {
		t.Fatal("libfoo not selected")
	}
	if libfoo.Location != "pool/l/libfoo21.deb" {
		t.Fatalf("selected wrong libfoo: %s", libfoo.Location)
	}
}

func TestSolvePicksNewestVersion(t *testing.T) {
	ix := &upstream.Index{Backend: "rpm", Packages: []upstream.Pkg{
		mkPkg("nginx", "1.20", "1", "x86_64", "P/nginx-1.20.rpm", nil, nil, nil),
		mkPkg("nginx", "1.24", "1", "x86_64", "P/nginx-1.24.rpm", nil, nil, nil),
	}}
	selected, problems := Solve(ix, []string{"nginx"}, SolveOptions{Backend: "rpm"})
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	var nginx upstream.Pkg
	for _, p := range selected {
		if p.Name == "nginx" {
			nginx = p
		}
	}
	if nginx.Location != "P/nginx-1.24.rpm" {
		t.Fatalf("did not pick newest: %s", nginx.Location)
	}
}

func TestCompareRPM(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0", "1.1", -1},
		{"1.10", "1.9", 1}, // 10 > 9 numerically
		{"1.0", "1.0", 0},
		{"2:1.0", "1.9", 1}, // epoch higher wins
		{"1.0-1", "1.0-2", -1},
		{"1.0", "1.0-1", -1},
	}
	for _, c := range cases {
		got := compareRPM(c.a, c.b)
		if got != c.want {
			t.Errorf("compareRPM(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestCompareDEB(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0", "1.1", -1},
		{"1.0~rc1", "1.0", -1}, // ~ sorts before
		{"1.0", "1.0-1", -1},   // empty revision sorts before a revision
		{"2:1.0", "1.9", 1},
	}
	for _, c := range cases {
		got := compareDEB(c.a, c.b)
		if got != c.want {
			t.Errorf("compareDEB(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
