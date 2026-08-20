package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, home string) {
	t.Helper()
	content := `schema_version: 2
vars:
  releasever: [9]
paths:
  home_dir: auto
  repo_dir: ${home}/repos
repositories:
  - name: rocky-9
    backend: rpm
    upstream:
      url: http://mirrors.rockylinux.org/pub/rocky/$releasever/BaseOS/$basearch/os/
      vars:
        - name: basearch
          values: [x86_64, aarch64]
    sync:
      enabled: true
`
	if err := os.MkdirAll(filepath.Join(home, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config", "repo.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadAndExpand(t *testing.T) {
	home := t.TempDir()
	writeConfig(t, home)
	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	r := &cfg.Repositories[0]
	if cfg.Paths.RepoDir != filepath.Join(home, "repos") {
		t.Fatalf("RepoDir = %q", cfg.Paths.RepoDir)
	}
	variants, err := Expand(cfg, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(variants) != 2 {
		t.Fatalf("expected 2 variants, got %d", len(variants))
	}
	want := map[string]bool{
		"http://mirrors.rockylinux.org/pub/rocky/9/BaseOS/aarch64/os/": true,
		"http://mirrors.rockylinux.org/pub/rocky/9/BaseOS/x86_64/os/":  true,
	}
	for i, v := range variants {
		if !want[v.URL] {
			t.Errorf("variant %d URL = %q, unexpected", i, v.URL)
		}
		root := v.ContentRoot(cfg)
		if filepath.Base(root) == "repos" {
			t.Errorf("variant %d content root did not add arch subdir: %q", i, root)
		}
	}
}

func TestSingleValueNoSubdir(t *testing.T) {
	home := t.TempDir()
	content := `schema_version: 2
paths:
  repo_dir: ${home}/repos
repositories:
  - name: centos
    backend: rpm
    upstream:
      url: http://x/centos/$basearch/os/
      vars:
        - name: basearch
          value: x86_64
    sync:
      enabled: true
`
	if err := os.MkdirAll(filepath.Join(home, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config", "repo.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	r := &cfg.Repositories[0]
	variants, err := Expand(cfg, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(variants) != 1 {
		t.Fatalf("expected 1 variant, got %d", len(variants))
	}
	if variants[0].URL != "http://x/centos/x86_64/os/" {
		t.Fatalf("URL = %q", variants[0].URL)
	}
	// Single value -> content root is exactly repo_dir/<name> (no arch subdir).
	want := filepath.Join(home, "repos", "centos")
	if got := variants[0].ContentRoot(cfg); got != want {
		t.Fatalf("ContentRoot = %q, want %q", got, want)
	}
}

func TestRepoDirOverride(t *testing.T) {
	home := t.TempDir()
	content := `schema_version: 2
paths:
  repo_dir: ${home}/repos
repositories:
  - name: special
    repo_dir: /data/special
    backend: rpm
    upstream:
      url: http://x/special/$basearch/os/
      vars:
        - name: basearch
          values: [x86_64, aarch64]
    sync:
      enabled: true
`
	if err := os.MkdirAll(filepath.Join(home, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config", "repo.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	r := &cfg.Repositories[0]
	variants, _ := Expand(cfg, r)
	wantBase := filepath.FromSlash("/data/special")
	if got := variants[0].ContentRoot(cfg); !strings.HasPrefix(filepath.Clean(got), wantBase) {
		t.Fatalf("ContentRoot = %q, want under %s", got, wantBase)
	}
}

func TestExpandMultiSource(t *testing.T) {
	home := t.TempDir()
	content := `schema_version: 2
paths:
  repo_dir: ${home}/repos
repositories:
  - name: centos8
    backend: rpm
    upstream:
      sources:
        - url: http://mirrors/baseos/$basearch/os/
          vars:
            - name: basearch
              value: x86_64
        - url: http://mirrors/appstream/$basearch/os/
          vars:
            - name: basearch
              value: x86_64
    input:
      packages: [vim]
`
	if err := os.MkdirAll(filepath.Join(home, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config", "repo.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	r := &cfg.Repositories[0]
	variants, err := Expand(cfg, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(variants) != 1 {
		t.Fatalf("expected 1 variant, got %d", len(variants))
	}
	ev := variants[0]
	if len(ev.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(ev.Sources))
	}
	if ev.Sources[0].URL != "http://mirrors/baseos/x86_64/os/" {
		t.Fatalf("source0 URL = %q", ev.Sources[0].URL)
	}
	if ev.Sources[1].URL != "http://mirrors/appstream/x86_64/os/" {
		t.Fatalf("source1 URL = %q", ev.Sources[1].URL)
	}
}

func TestSegmentModeParsing(t *testing.T) {
	cases := []struct {
		yaml string
		want SegmentMode
	}{
		{"segment: false", SegmentDisabled},
		{"segment: 4", 4},
		{"segment: true", SegmentSmart},
		{"", SegmentSmart}, // absent → defaulted to smart in Load
		{"segment: 12", 12},
	}
	for _, c := range cases {
		home := t.TempDir()
		content := "schema_version: 2\npaths:\n  repo_dir: " + home + "/repos\nrepositories:\n  - name: x\n    backend: rpm\n    upstream:\n      url: http://e.invalid/x\n    sync:\n      enabled: true\n      " + c.yaml + "\n"
		if err := os.MkdirAll(filepath.Join(home, "config"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, "config", "repo.yaml"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(home)
		if err != nil {
			t.Fatalf("yaml=%q err=%v", c.yaml, err)
		}
		got := cfg.Repositories[0].Sync.Segment
		if got != c.want {
			t.Errorf("yaml=%q segment=%v, want %v", c.yaml, got, c.want)
		}
	}
}

func loadRepoConfig(t *testing.T, content string) *Config {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config", "repo.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// value: [x86_64, aarch64] — a list given to the single-value field — must
// expand into multiple variants instead of failing to unmarshal.
func TestVarValueAcceptsList(t *testing.T) {
	content := `schema_version: 2
paths:
  repo_dir: ${home}/repos
repositories:
  - name: multiarch
    backend: rpm
    upstream:
      url: http://x/$basearch/os/
      vars:
        - name: basearch
          value: [x86_64, aarch64]
    sync:
      enabled: true
`
	cfg := loadRepoConfig(t, content)
	variants, err := Expand(cfg, &cfg.Repositories[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(variants) != 2 {
		t.Fatalf("expected 2 variants, got %d", len(variants))
	}
	want := map[string]bool{
		"http://x/x86_64/os/":  true,
		"http://x/aarch64/os/": true,
	}
	for _, v := range variants {
		if !want[v.URL] {
			t.Errorf("unexpected variant URL %q", v.URL)
		}
	}
}

// Global vars accept both a scalar and a list per key.
func TestVarMapScalarAndList(t *testing.T) {
	for _, tc := range []struct {
		name    string
		global  string
		wantURL string
	}{
		{"scalar", "  basearch: x86_64\n", "http://x/x86_64/os/"},
		{"list", "  basearch: [x86_64, aarch64]\n", "http://x/aarch64/os/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content := "schema_version: 2\npaths:\n  repo_dir: ${home}/repos\nvars:\n" + tc.global + "repositories:\n  - name: multiarch\n    backend: rpm\n    upstream:\n      url: http://x/$basearch/os/\n    sync:\n      enabled: true\n"
			cfg := loadRepoConfig(t, content)
			variants, err := Expand(cfg, &cfg.Repositories[0])
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, v := range variants {
				if v.URL == tc.wantURL {
					found = true
				}
			}
			if !found {
				t.Fatalf("variants %+v missing URL %q", variants, tc.wantURL)
			}
		})
	}
}

// Local vars override global vars, and a scalar override stays a single variant.
func TestVarLocalOverrideScalar(t *testing.T) {
	content := `schema_version: 2
paths:
  repo_dir: ${home}/repos
vars:
  basearch: [x86_64, aarch64]
repositories:
  - name: multiarch
    backend: rpm
    upstream:
      url: http://x/$basearch/os/
      vars:
        - name: basearch
          value: x86_64
    sync:
      enabled: true
`
	cfg := loadRepoConfig(t, content)
	variants, err := Expand(cfg, &cfg.Repositories[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(variants) != 1 {
		t.Fatalf("expected 1 variant after scalar override, got %d", len(variants))
	}
	if variants[0].URL != "http://x/x86_64/os/" {
		t.Fatalf("URL = %q", variants[0].URL)
	}
}

// Multi-arch basearch declared in sources vars must still split the content
// root into per-architecture subdirectories (same rule as variant expansion),
// so variants do not overwrite each other's output.
func TestContentRootMultiArchFromSourcesVars(t *testing.T) {
	content := `schema_version: 2
paths:
  repo_dir: ${home}/repos
repositories:
  - name: multiarch
    backend: rpm
    upstream:
      sources:
        - url: http://x/base/$basearch/
          vars:
            - name: basearch
              values: [x86_64, aarch64]
    sync:
      enabled: true
`
	cfg := loadRepoConfig(t, content)
	r := &cfg.Repositories[0]
	variants, err := Expand(cfg, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(variants) != 2 {
		t.Fatalf("expected 2 variants, got %d", len(variants))
	}
	roots := map[string]bool{}
	for _, v := range variants {
		roots[v.ContentRoot(cfg)] = true
	}
	want := map[string]bool{
		filepath.Join(cfg.Paths.RepoDir, "multiarch", "x86_64"):  true,
		filepath.Join(cfg.Paths.RepoDir, "multiarch", "aarch64"): true,
	}
	for r := range want {
		if !roots[r] {
			t.Errorf("missing content root %q, got %v", r, roots)
		}
	}
}
