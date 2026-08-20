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
	selected, problems, _ := Solve(ix, []string{"vim"}, SolveOptions{Backend: "rpm", WeakDeps: false})
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
	selected, problems, _ := Solve(ix, []string{"app"}, SolveOptions{Backend: "deb"})
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
	selected, problems, _ := Solve(ix, []string{"nginx"}, SolveOptions{Backend: "rpm"})
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

func TestToleratedDep(t *testing.T) {
	cases := []struct {
		dep     upstream.DependencyEntry
		backend string
		want    bool
	}{
		{upstream.DependencyEntry{Name: "annobin if gcc"}, "rpm", true},
		{upstream.DependencyEntry{Name: "python3-cffi-backend-api-max"}, "deb", true},
		{upstream.DependencyEntry{Name: "libc.so.6"}, "rpm", false},
		{upstream.DependencyEntry{Name: "libc6"}, "deb", false},
	}
	for _, c := range cases {
		if got := toleratedDep(c.dep, c.backend); got != c.want {
			t.Errorf("toleratedDep(%q,%s) = %v, want %v", c.dep.Name, c.backend, got, c.want)
		}
	}
}

func TestSolveNoticesForConditional(t *testing.T) {
	// A package requiring "annobin if gcc" (conditional) an "annobin" that doesn't exist.
	ix := &upstream.Index{Backend: "rpm", Packages: []upstream.Pkg{
		mkPkg("gcc", "10", "1", "x86_64", "P/gcc.rpm",
			[]upstream.DependencyEntry{{Name: "annobin if gcc"}}, nil, nil),
		mkPkg("gcc", "10", "1", "x86_64", "P/gcc2.rpm", nil, nil, nil),
	}}
	_, problems, notices := Solve(ix, []string{"gcc"}, SolveOptions{Backend: "rpm"})
	if len(problems) != 0 {
		t.Fatalf("problems should be empty for conditional dep, got %v", problems)
	}
	if len(notices) != 1 {
		t.Fatalf("notices = %v, want 1", notices)
	}
}

// Local packages parsed from package_dirs must win over upstream versions and
// their dependencies must be resolved.
func TestSolveLocalPkgsWin(t *testing.T) {
	ix := &upstream.Index{Packages: []upstream.Pkg{
		{Name: "vim", Version: "8.2", Release: "1", Arch: "x86_64", Location: "Packages/v/vim.rpm"},
		{Name: "glibc", Version: "2.34", Release: "1", Arch: "x86_64", Location: "Packages/g/glibc.rpm"},
	}}
	// Local vim 9.0 (different version than upstream) + local third-party mylocal.
	local := []upstream.Pkg{
		{Name: "vim", Version: "9.0", Release: "1", Arch: "x86_64", Location: "Packages/vim-9.0.rpm", Local: true,
			Requires: []upstream.DependencyEntry{{Name: "glibc"}}},
		{Name: "mylocal", Version: "1.0", Release: "1", Arch: "x86_64", Location: "Packages/mylocal.rpm", Local: true,
			Requires: []upstream.DependencyEntry{{Name: "vim"}}},
	}
	selected, problems, notices := Solve(ix, []string{"mylocal"}, SolveOptions{
		Backend: "rpm", Archs: []string{"x86_64", "noarch"}, LocalPkgs: local,
	})
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	// Local vim wins (9.0), upstream 8.2 not downloaded.
	vim, ok := selected["vim"]
	if !ok {
		t.Fatal("vim not selected")
	}
	if vim.Version != "9.0" || !vim.Local {
		t.Fatalf("vim = %+v, want local 9.0", vim)
	}
	// mylocal selected + glibc dep resolved.
	if _, ok := selected["mylocal"]; !ok {
		t.Fatal("mylocal not selected")
	}
	glibc, ok := selected["glibc"]
	if !ok || glibc.Location != "Packages/g/glibc.rpm" {
		t.Fatalf("glibc dep not resolved: %+v", glibc)
	}
	_ = notices
}

// Cross-arch complement: a variant without a local copy requests the name from
// upstream and gets the matching architecture (not the local-only one).
func TestSolveCrossArchComplementFromUpstream(t *testing.T) {
	ix := &upstream.Index{Packages: []upstream.Pkg{
		{Name: "vim", Version: "8.2", Release: "1", Arch: "x86_64", Location: "P/vim.x86.rpm"},
		{Name: "vim", Version: "8.2", Release: "1", Arch: "aarch64", Location: "P/vim.arm.rpm"},
	}}
	selected, problems, _ := Solve(ix, []string{"vim"}, SolveOptions{
		Backend: "rpm", Archs: []string{"aarch64", "noarch"}, SoftRequests: []string{"vim"},
	})
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	vim, ok := selected["vim"]
	if !ok || vim.Arch != "aarch64" || vim.Location != "P/vim.arm.rpm" {
		t.Fatalf("vim = %+v, want upstream aarch64 copy", vim)
	}
}
