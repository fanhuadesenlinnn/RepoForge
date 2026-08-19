package upstream

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/fanhuadesenlinnn/RepoForge/internal/progress"
)

// DEBSpec describes which suites/components/architectures to read.
type DEBSpec struct {
	BaseURL string // e.g. https://deb.debian.org/debian
	Suites  []DEBSuite
}
type DEBSuite struct {
	Name       string
	Components []string
	Archs      []string
}

// DEBIndex fetches Packages files for the given spec and merges them.
func DEBIndex(ctx context.Context, spec DEBSpec) (*Index, error) {
	base := strings.TrimRight(spec.BaseURL, "/")
	merged := map[string]Pkg{} // key = Filename;pkgname;arch+ver
	order := []string{}
	for _, suite := range spec.Suites {
		comps := suite.Components
		if len(comps) == 0 {
			comps = []string{"main"}
		}
		archs := suite.Archs
		if len(archs) == 0 {
			archs = []string{"amd64"}
		}
		for _, comp := range comps {
			for _, arch := range archs {
				progress.Infof(ctx, "[元数据] 读取 Packages  %s/%s/%s", suite.Name, comp, arch)
				pkgs, notFound, err := readDEBPackages(ctx, base, suite.Name, comp, arch)
				if err != nil {
					return nil, err
				}
				if notFound {
					// A suite/component may legitimately be empty or absent
					// (e.g. bookworm-updates/main on some mirrors). Skip.
					continue
				}
				for _, p := range pkgs {
					k := p.Location + "|" + p.Name + "|" + p.Version + "|" + p.Arch
					if _, ok := merged[k]; !ok {
						merged[k] = p
						order = append(order, k)
					}
				}
			}
		}
	}
	out := make([]Pkg, 0, len(order))
	for _, k := range order {
		out = append(out, merged[k])
	}
	return &Index{BaseURL: base, Backend: "deb", Packages: out}, nil
}

func readDEBPackages(ctx context.Context, base, suite, comp, arch string) ([]Pkg, bool, error) {
	rel := path.Join("dists", suite, comp, "binary-"+arch, "Packages")
	// Try uncompressed, then .gz, then .zst.
	var data []byte
	var err error
	tried := []string{}
	for _, suffix := range []string{"", ".gz", ".zst"} {
		url := base + "/" + rel + suffix
		if suffix == ".zst" {
			continue // zstd unsupported for now
		}
		data, err = Fetch(ctx, url)
		if err == nil {
			if suffix == ".gz" {
				if d, gerr := decompressAny(data, rel+".gz"); gerr == nil {
					data = d
				}
			}
			got, perr := parseDEBPackages(base, string(data))
			return got, false, perr
		}
		tried = append(tried, suffix)
	}
	// All variants missing -> treat as absent component (not an error).
	if missingPackages(err, tried) {
		return nil, true, nil
	}
	return nil, false, fmt.Errorf("读取 DEB Packages（%s/%s/%s）失败: %w", suite, comp, arch, err)
}

// missingPackages reports whether the error indicates every variant 404'd.
func missingPackages(err error, tried []string) bool {
	s := err.Error()
	if !strings.Contains(s, "404") {
		return false
	}
	return true
}

var debEntrySep = regexp.MustCompile(`\n\s*\n`)

func parseDEBPackages(baseURL, text string) ([]Pkg, error) {
	var out []Pkg
	for _, block := range debEntrySep.Split(text, -1) {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		fields := map[string]string{}
		lines := strings.Split(block, "\n")
		var curKey string
		for _, line := range lines {
			line = strings.TrimRight(line, "\r")
			if line == "" {
				continue
			}
			if line[0] == ' ' || line[0] == '\t' {
				fields[curKey] += " " + strings.TrimSpace(line)
				continue
			}
			idx := strings.Index(line, ":")
			if idx < 0 {
				continue
			}
			curKey = line[:idx]
			fields[curKey] = strings.TrimSpace(line[idx+1:])
		}
		pkg := Pkg{
			Name:      fields["Package"],
			Version:   fields["Version"],
			Arch:      fields["Architecture"],
			Location:  fields["Filename"],
			Checksum:  fields["SHA256"],
			Summary:   fields["Description"],
			MultiArch: fields["Multi-Arch"],
		}
		if n, err := strconv.ParseInt(fields["Size"], 10, 64); err == nil {
			pkg.Size = n
		}
		// epoch may be embedded as "epoch:version"
		if idx := strings.Index(pkg.Version, ":"); idx >= 0 {
			pkg.Epoch = pkg.Version[:idx]
			pkg.Version = pkg.Version[idx+1:]
		}
		pkg.Requires = parseDEBDeps(fields["Depends"])
		pkg.Recommends = parseDEBDeps(fields["Recommends"])
		pkg.Provides = parseDEBProvides(fields["Provides"])
		if pkg.Name != "" {
			out = append(out, pkg)
		}
	}
	return out, nil
}

// parseDEBDeps parses "a (>= 1), b | c (>= 2)". Alternatives (|) become multiple
// entries with the same position hint; we flatten them as separate Requires.
func parseDEBDeps(raw string) []DependencyEntry {
	if raw == "" || strings.EqualFold(raw, "(none)") {
		return nil
	}
	// Split top-level on commas (alternatives kept on one entry via "|").
	var entries []DependencyEntry
	for _, comma := range strings.Split(raw, ",") {
		for _, alt := range strings.Split(comma, "|") {
			alt = strings.TrimSpace(alt)
			if alt == "" {
				continue
			}
			re := regexp.MustCompile(`^([a-zA-Z0-9.+\-:]+)(?:\s*\(([<>=! ]+)\s*([^)]+)\))?$`)
			m := re.FindStringSubmatch(alt)
			if m == nil {
				continue
			}
			name := m[1]
			// Strip Multi-Arch qualifiers (":any", ":native", ":<arch>") so that
			// e.g. "python3:any" resolves to the package "python3".
			if idx := strings.Index(name, ":"); idx >= 0 {
				if q := name[idx+1:]; q == "any" || q == "native" {
					name = name[:idx]
				}
			}
			e := DependencyEntry{Name: name}
			if len(m) > 2 && m[2] != "" {
				e.Op = normalizeDebOp(m[2])
				e.Version = m[3]
			}
			entries = append(entries, e)
		}
	}
	return entries
}

func normalizeDebOp(op string) string {
	op = strings.ReplaceAll(op, " ", "")
	switch {
	case strings.HasPrefix(op, ">="):
		return ">="
	case strings.HasPrefix(op, "<="):
		return "<="
	case strings.HasPrefix(op, ">>"):
		return ">"
	case strings.HasPrefix(op, "<<"):
		return "<"
	case strings.HasPrefix(op, "="):
		return "="
	case strings.HasPrefix(op, "!"):
		return "!="
	}
	return op
}

func parseDEBProvides(raw string) []string {
	if raw == "" || strings.EqualFold(raw, "(none)") {
		return nil
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if i := strings.Index(p, "("); i >= 0 {
			p = strings.TrimSpace(p[:i])
		}
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
