package engine

import (
	"fmt"
	"strings"

	"github.com/fanhuadesenlinnn/RepoForge/internal/upstream"
)

// SolveOptions controls dependency resolution behavior.
type SolveOptions struct {
	Backend  string   // rpm | deb
	Archs    []string // allowed architectures (empty = all)
	WeakDeps bool     // include Recommends
}

// Solve computes the set of package locations needed to satisfy the requested
// names, following hard dependencies (and weak deps when requested).
// It returns the selected packages and any unresolvable dependencies.
func Solve(ix *upstream.Index, request []string, opt SolveOptions) (map[string]upstream.Pkg, []string) {
	selected := map[string]upstream.Pkg{} // by provides-name -> package
	addedLoc := map[string]bool{}
	var queue []upstream.DependencyEntry
	var problems []string

	archOK := func(p upstream.Pkg) bool {
		if len(opt.Archs) == 0 {
			return true
		}
		for _, a := range opt.Archs {
			if a == p.Arch || a == "*" {
				return true
			}
		}
		if opt.Backend == "rpm" && p.Arch == "noarch" {
			return true
		}
		if opt.Backend == "deb" && p.Arch == "all" {
			return true
		}
		return false
	}

	for _, name := range request {
		queue = append(queue, upstream.DependencyEntry{Name: name})
	}

	for len(queue) > 0 {
		dep := queue[0]
		queue = queue[1:]

		// Map file-path requirements (/sbin/ldconfig, /usr/bin/bash) to the
		// package that owns them. primary.xml omits file provides; we carry a
		// small built-in table of common paths so these resolve cleanly.
		if opt.Backend == "rpm" && strings.HasPrefix(dep.Name, "/") {
			if owner, ok := fileProvider[dep.Name]; ok {
				dep.Name = owner
			}
		}

		// already satisfied?
		if existing, ok := selected[dep.Name]; ok {
			if satisfies(existing, dep, opt.Backend) {
				continue
			}
			// still try to find a better provider
		}

		candidates := providers(ix, dep.Name, archOK)
		best := chooseBest(candidates, dep, opt.Backend)
		if best == nil {
			problems = append(problems, fmt.Sprintf("无法满足依赖: %s", dep.String()))
			continue
		}
		if prev, ok := selected[dep.Name]; ok && prev.Location != best.Location {
			problems = append(problems, fmt.Sprintf("依赖冲突: %s 同时要求 %s 和 %s", dep.Name, prev.NEVRA(), best.NEVRA()))
			continue
		}
		selected[dep.Name] = *best
		if addedLoc[best.Location] {
			continue
		}
		addedLoc[best.Location] = true
		for _, r := range best.Requires {
			queue = append(queue, r)
		}
		if opt.WeakDeps {
			for _, r := range best.Recommends {
				queue = append(queue, r)
			}
		}
	}

	return selected, dedupe(problems)
}

func dedupe(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func providers(ix *upstream.Index, name string, archOK func(upstream.Pkg) bool) []upstream.Pkg {
	var out []upstream.Pkg
	for _, p := range ix.Packages {
		if !archOK(p) {
			continue
		}
		if p.Name == name {
			out = append(out, p)
			continue
		}
		for _, prov := range p.Provides {
			if providesMatch(prov, name) {
				out = append(out, p)
			}
		}
	}
	return out
}

// providesMatch reports whether a provided capability satisfies a required
// capability. Handles RPM soname/version markers like libc.so.6(GLIBC_2.28)(64bit).
// A requirement with a version marker is satisfied by a bare provider of the
// same base name (any version), or by a provider whose markers cover >= it.
func providesMatch(provided, required string) bool {
	if provided == required {
		return true
	}
	pBase, pVers := rpmsplit(provided)
	rBase, rVers := rpmsplit(required)
	if pBase == "" || pBase != rBase {
		// also accept plain prefixless exact compare on base
		return false
	}
	// If the requirement is bare (no version marker), any provider with the
	// same base name satisfies it.
	if len(rVers) == 0 {
		return true
	}
	// Requirement has version markers. A bare provider (no markers) satisfies
	// any version. Otherwise require the provider to have a marker >= request.
	if len(pVers) == 0 {
		return true
	}
	for _, rv := range rVers {
		ok := false
		for _, pv := range pVers {
			if rpmsymCompare(pv, rv) >= 0 {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// rpmsplit splits "name(VERSION)(ARCH)" into base name and the parenthesized
// marker contents (versions/arch tokens).
func rpmsplit(s string) (string, []string) {
	i := strings.Index(s, "(")
	if i < 0 {
		return s, nil
	}
	base := s[:i]
	rest := s[i:]
	var toks []string
	for {
		open := strings.Index(rest, "(")
		if open < 0 {
			break
		}
		close := strings.Index(rest[open:], ")")
		if close < 0 {
			break
		}
		tok := rest[open+1 : open+close]
		rest = rest[open+close+1:]
		if tok != "" {
			toks = append(toks, tok)
		}
	}
	return base, toks
}

// rpmsymCompare compares two marker tokens such as "GLIBC_2.28" or "64bit".
// Returns 0 if equal, 1 if a newer/higher.
func rpmsymCompare(a, b string) int {
	if a == b {
		return 0
	}
	// treat arch tokens (64bit/32bit) as equal-ish
	if (a == "64bit" || a == "32bit") && (b == "64bit" || b == "32bit") {
		return 0
	}
	// compare as version-ish
	return rpmvercmp(a, b)
}

func chooseBest(cands []upstream.Pkg, dep upstream.DependencyEntry, backend string) *upstream.Pkg {
	// Filter by version constraint.
	var valid []upstream.Pkg
	for i := range cands {
		if satisfies(cands[i], dep, backend) {
			valid = append(valid, cands[i])
		}
	}
	if len(valid) == 0 {
		return nil
	}
	best := valid[0]
	for _, c := range valid[1:] {
		if pkgNewer(c, best, backend) > 0 {
			best = c
		}
	}
	return &best
}

func satisfies(p upstream.Pkg, dep upstream.DependencyEntry, backend string) bool {
	if dep.Version == "" || dep.Op == "" {
		return true
	}
	cmp := 0
	if backend == "deb" {
		v := p.Version
		if p.Epoch != "" {
			v = p.Epoch + ":" + v
		}
		if p.Release != "" {
			v = v + "-" + p.Release
		}
		cmp = compareDEB(v, dep.Version)
	} else {
		v := p.Version
		if p.Epoch != "" {
			v = p.Epoch + ":" + v
		}
		if p.Release != "" {
			v = v + "-" + p.Release
		}
		cmp = compareRPM(v, dep.Version)
	}
	switch dep.Op {
	case "=", "eq":
		return cmp == 0
	case ">=", "ge":
		return cmp >= 0
	case "<=", "le":
		return cmp <= 0
	case ">", "gt":
		return cmp > 0
	case "<", "lt":
		return cmp < 0
	case "!=":
		return cmp != 0
	}
	return true
}

func pkgNewer(a, b upstream.Pkg, backend string) int {
	va := a.Version
	if a.Epoch != "" {
		va = a.Epoch + ":" + va
	}
	if a.Release != "" {
		va = va + "-" + a.Release
	}
	vb := b.Version
	if b.Epoch != "" {
		vb = b.Epoch + ":" + vb
	}
	if b.Release != "" {
		vb = vb + "-" + b.Release
	}
	if backend == "deb" {
		return compareDEB(va, vb)
	}
	return compareRPM(va, vb)
}

// fileProvider maps common RPM file-path dependencies to the package that
// provides them. primary.xml does not list file provides, so we resolve the
// well-known ones here to keep dependency solving complete.
var fileProvider = map[string]string{
	"/bin/sh":                "bash",
	"/usr/bin/sh":            "bash",
	"/usr/bin/bash":          "bash",
	"/bin/bash":              "bash",
	"/usr/bin/perl":          "perl",
	"/sbin/ldconfig":         "glibc",
	"/usr/sbin/ldconfig":     "glibc",
	"/usr/bin/which":         "which",
	"/bin/which":             "which",
	"/sbin/install-info":     "info",
	"/usr/sbin/install-info": "info",
	"/usr/bin/install-info":  "info",
	"/bin/cp":                "coreutils",
	"/bin/mv":                "coreutils",
	"/bin/rm":                "coreutils",
	"/usr/bin/cp":            "coreutils",
	"/usr/bin/env":           "coreutils",
	"/usr/bin/find":          "findutils",
	"/bin/find":              "findutils",
	"/usr/bin/awk":           "gawk",
	"/bin/awk":               "gawk",
	"/bin/mount":             "util-linux",
	"/usr/bin/grep":          "grep",
	"/bin/grep":              "grep",
	"/usr/bin/sed":           "sed",
	"/bin/sed":               "sed",
	"/usr/bin/tar":           "tar",
	"/bin/tar":               "tar",
}
