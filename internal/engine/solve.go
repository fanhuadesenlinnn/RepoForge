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
	// PreProvided lists package names already available locally (e.g. from
	// input.package_dirs). They are treated as satisfied — not re-downloaded —
	// but their dependencies are still resolved and fetched.
	PreProvided []string
	// LocalPkgs are complete package entries parsed from local package files
	// (input.package_dirs). They are selected as-is (local version wins over
	// any upstream version), their dependencies are resolved, and they are
	// published into the repo. Prefer LocalPkgs over PreProvided.
	LocalPkgs []upstream.Pkg
	// SoftRequests are package names that should be resolved if upstream has a
	// matching version, but must not fail the build when it does not. Used for
	// cross-arch complementing: package_dirs only carries one architecture, so
	// the other variants request the same names from upstream, and a
	// third-party package with no upstream counterpart becomes a notice.
	SoftRequests []string
}

// Solve computes the set of package locations needed to satisfy the requested
// names, following hard dependencies (and weak deps when requested).
// It returns the selected packages and any unresolvable dependencies, split
// into hard problems and tolerated notices (soft/conditional/virtual deps that
// a packaged mirror may legitimately not fully resolve).
func Solve(ix *upstream.Index, request []string, opt SolveOptions) (map[string]upstream.Pkg, []string, []string) {
	idx := buildSolveIndex(ix)
	selected := map[string]upstream.Pkg{} // by provides-name -> package
	addedLoc := map[string]bool{}
	soft := map[string]bool{}
	for _, n := range opt.SoftRequests {
		soft[n] = true
	}
	var queue []upstream.DependencyEntry
	var problems []string
	var notices []string

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

	// Pre-provided (local) packages are already available; mark them satisfied
	// and enqueue their dependencies for resolution.
	for _, name := range opt.PreProvided {
		pkgs := idx.providers(name, archOK)
		if len(pkgs) == 0 {
			notices = append(notices, fmt.Sprintf("本地已有包 %q 未在上游找到（仅复制自身，不补外部依赖）", name))
			continue
		}
		best := chooseBest(pkgs, upstream.DependencyEntry{Name: name}, opt.Backend)
		if best == nil {
			continue
		}
		selected[name] = *best
		addedLoc[best.Location] = true
		for _, r := range best.Requires {
			queue = append(queue, r)
		}
	}

	// Local packages parsed from package_dirs files: select them as-is (local
	// version wins), enqueue their dependencies, and never download them again.
	for _, p := range opt.LocalPkgs {
		if !archOK(p) {
			continue
		}
		if prev, ok := selected[p.Name]; ok {
			// A non-local package was already selected for this name (e.g. an
			// explicit request); the local copy still wins.
			delete(addedLoc, prev.Location)
		}
		selected[p.Name] = p
		addedLoc[p.Location] = true
		for _, r := range p.Requires {
			queue = append(queue, r)
		}
		if opt.WeakDeps {
			queue = append(queue, p.Recommends...)
		}
	}

	for len(queue) > 0 {
		dep := queue[0]
		queue = queue[1:]

		// File-path requirement: match via the file list first (from
		// filelists.xml). Fall back to the built-in fileProvider table when the
		// repo has no filelists or the file was not listed.
		if opt.Backend == "rpm" && strings.HasPrefix(dep.Name, "/") {
			resolved := false
			cands := idx.providers(dep.Name, archOK)
			if len(cands) == 0 {
				if owner, ok := fileProvider[dep.Name]; ok {
					if p, ok2 := idx.findByName(owner, archOK); ok2 {
						cands = []upstream.Pkg{p}
					}
				}
			}
			if best3 := chooseBest(cands, dep, opt.Backend); best3 != nil {
				selected[dep.Name] = *best3
				if !addedLoc[best3.Location] {
					addedLoc[best3.Location] = true
					queue = append(queue, best3.Requires...)
					if opt.WeakDeps {
						queue = append(queue, best3.Recommends...)
					}
				}
				resolved = true
			}
			if !resolved {
				problems = append(problems, fmt.Sprintf("无法满足依赖: %s", dep.String()))
			}
			continue
		}

		// already satisfied?
		if existing, ok := selected[dep.Name]; ok {
			if satisfies(existing, dep, opt.Backend) {
				continue
			}
			// A local package always wins: keep the local version even when it
			// does not satisfy a versioned requirement, and report it.
			if existing.Local {
				notices = append(notices, fmt.Sprintf("本地包 %s 不满足依赖 %s（已保留本地版本）", existing.NEVRA(), dep.String()))
				continue
			}
			// still try to find a better provider
		}

		candidates := idx.providers(dep.Name, archOK)
		best := chooseBest(candidates, dep, opt.Backend)
		if best == nil {
			// Cross-arch complement: the name came from package_dirs on another
			// architecture; upstream has no counterpart, so it cannot be
			// complemented — inform, do not fail the build.
			if soft[dep.Name] {
				notices = append(notices, fmt.Sprintf("本地包 %q 在上游无对应架构版本，未补全", dep.Name))
				continue
			}
			if toleratedDep(dep, opt.Backend) {
				notices = append(notices, fmt.Sprintf("未匹配(可忽略): %s", dep.String()))
			} else {
				problems = append(problems, fmt.Sprintf("无法满足依赖: %s", dep.String()))
			}
			continue
		}
		if prev, ok := selected[dep.Name]; ok && prev.Location != best.Location {
			if prev.Local {
				notices = append(notices, fmt.Sprintf("本地包 %s 优先于上游 %s（已保留本地版本）", prev.NEVRA(), best.NEVRA()))
				continue
			}
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

	// Reconcile: some "unresolved" deps may actually be satisfied by an already
	// selected package (e.g. a file path whose owning package was pulled in by
	// another dependency, but filelists weren't available to connect them).
	// Downgrade those to informational notices instead of hard problems.
	problems = downgradeSatisfied(dedupe(problems), selected)

	return selected, dedupe(problems), dedupe(notices)
}

// downgradeSatisfied moves problems whose dependency is satisfiable by an
// already-selected package into notices.
func downgradeSatisfied(problems []string, selected map[string]upstream.Pkg) []string {
	var kept []string
	for _, p := range problems {
		if strings.HasPrefix(p, "无法满足依赖: ") {
			dep := strings.TrimPrefix(p, "无法满足依赖: ")
			if satisfiedBySelected(dep, selected) {
				continue // downgrade: provider already selected
			}
		}
		kept = append(kept, p)
	}
	return kept
}

// satisfiedBySelected reports whether any selected package provides the name
// (by package name, capability, or file path).
func satisfiedBySelected(name string, selected map[string]upstream.Pkg) bool {
	for _, p := range selected {
		if p.Name == name {
			return true
		}
		for _, prov := range p.Provides {
			if prov == name {
				return true
			}
		}
		for _, f := range p.Files {
			if f == name {
				return true
			}
		}
	}
	return false
}

// toleratedDep reports whether an unresolvable dependency is a soft/conditional
// or virtual dependency that a packaged mirror may legitimately not fully
// resolve, and should be treated as an informational notice rather than a hard
// error.
func toleratedDep(dep upstream.DependencyEntry, backend string) bool {
	name := dep.Name
	if backend == "rpm" {
		// RPM conditional dependencies: "package if feature".
		if strings.Contains(name, " if ") {
			return true
		}
	}
	if backend == "deb" {
		// DEB virtual test-provides with -max/-min suffixes, e.g.
		// python3-cffi-backend-api-max. These are build/test markers, not
		// install-time requirements we must satisfy.
		if strings.HasSuffix(name, "-max") || strings.HasSuffix(name, "-min") {
			return true
		}
	}
	return false
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

// solveIndex pre-indexes an upstream.Index the way yum/apt do: exact hashes
// for package names, file paths, and full provides capability strings, so a
// dependency lookup is O(matches) instead of scanning every package. The base
// name index is only a fallback for version-marked soname requirements that
// need a fuzzy match.
type solveIndex struct {
	byName     map[string][]upstream.Pkg
	byFile     map[string][]upstream.Pkg
	byProvides map[string][]upstream.Pkg // full provides string -> packages
	byBase     map[string][]upstream.Pkg // rpmsplit base -> packages (fallback)
}

func buildSolveIndex(ix *upstream.Index) *solveIndex {
	idx := &solveIndex{
		byName:     make(map[string][]upstream.Pkg, len(ix.Packages)),
		byFile:     map[string][]upstream.Pkg{},
		byProvides: map[string][]upstream.Pkg{},
		byBase:     map[string][]upstream.Pkg{},
	}
	for _, p := range ix.Packages {
		idx.byName[p.Name] = append(idx.byName[p.Name], p)
		for _, f := range p.Files {
			idx.byFile[f] = append(idx.byFile[f], p)
		}
		for _, prov := range p.Provides {
			idx.byProvides[prov] = append(idx.byProvides[prov], p)
			base, _ := rpmsplit(prov)
			if base != prov {
				idx.byBase[base] = append(idx.byBase[base], p)
			}
		}
	}
	return idx
}

// providers returns packages matching name: by exact package name, by file
// path (when name starts with "/"), by an exact provides capability, or by a
// fuzzy base-name match for version-marked soname requirements.
func (idx *solveIndex) providers(name string, archOK func(upstream.Pkg) bool) []upstream.Pkg {
	isFile := strings.HasPrefix(name, "/")
	var out []upstream.Pkg
	if isFile {
		for _, p := range idx.byFile[name] {
			if archOK(p) {
				out = append(out, p)
			}
		}
		return out
	}
	for _, p := range idx.byName[name] {
		if archOK(p) {
			out = append(out, p)
		}
	}
	for _, p := range idx.byProvides[name] {
		if archOK(p) {
			out = append(out, p)
		}
	}
	// Fallback: version-marked requirement (e.g. libX11.so.6(GLIBC_2.28)) that
	// does not appear verbatim in any provides; match by base and verify.
	base, _ := rpmsplit(name)
	if base == "" || base == name {
		return out
	}
	for _, p := range idx.byBase[base] {
		if !archOK(p) {
			continue
		}
		for _, prov := range p.Provides {
			if providesMatch(prov, name) {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// findByName returns the best package with the given name, honoring arch filter.
func (idx *solveIndex) findByName(name string, archOK func(upstream.Pkg) bool) (upstream.Pkg, bool) {
	var best *upstream.Pkg
	for i := range idx.byName[name] {
		p := &idx.byName[name][i]
		if !archOK(*p) {
			continue
		}
		if best == nil || pkgNewer(*p, *best, "rpm") > 0 {
			best = p
		}
	}
	if best == nil {
		return upstream.Pkg{}, false
	}
	return *best, true
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
	"/bin/sh":                       "bash",
	"/usr/bin/sh":                   "bash",
	"/usr/bin/bash":                 "bash",
	"/bin/bash":                     "bash",
	"/usr/bin/perl":                 "perl",
	"/sbin/ldconfig":                "glibc",
	"/usr/sbin/ldconfig":            "glibc",
	"/usr/bin/which":                "which",
	"/bin/which":                    "which",
	"/sbin/install-info":            "info",
	"/usr/sbin/install-info":        "info",
	"/usr/bin/install-info":         "info",
	"/bin/cp":                       "coreutils",
	"/bin/mv":                       "coreutils",
	"/bin/rm":                       "coreutils",
	"/usr/bin/cp":                   "coreutils",
	"/usr/bin/env":                  "coreutils",
	"/usr/bin/find":                 "findutils",
	"/bin/find":                     "findutils",
	"/usr/bin/awk":                  "gawk",
	"/bin/awk":                      "gawk",
	"/bin/mount":                    "util-linux",
	"/usr/bin/grep":                 "grep",
	"/bin/grep":                     "grep",
	"/usr/bin/sed":                  "sed",
	"/bin/sed":                      "sed",
	"/usr/bin/tar":                  "tar",
	"/bin/tar":                      "tar",
	"/usr/bin/file":                 "file",
	"/bin/file":                     "file",
	"/usr/bin/xargs":                "findutils",
	"/bin/xargs":                    "findutils",
	"/usr/bin/gpg2":                 "gnupg2",
	"/bin/gpg2":                     "gnupg2",
	"/usr/sbin/update-alternatives": "chkconfig",
	"/etc/crypto-policies/back-ends/krb5.config": "crypto-policies",
}
