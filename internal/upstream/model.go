// Package upstream reads a remote repository's metadata and produces a unified
// package index, regardless of RPM or DEB backend. This is the shared "source
// of truth" base for both sync (full mirror) and install (dependency solving).
package upstream

import (
	"fmt"
	"strings"
)

// DependencyEntry is one dependency relation of a package.
type DependencyEntry struct {
	Name    string // package or provides name (e.g. "libc.so.6" or "vim")
	Op      string // comparison operator: "" "=" ">=" ">" "<=" "<" (!=)
	Version string // version the operator applies to (may be empty)
}

// String renders a dependency as a readable string in the native syntax.
func (d DependencyEntry) String() string {
	if d.Op == "" || d.Version == "" {
		return d.Name
	}
	return fmt.Sprintf("%s %s %s", d.Name, normalizeOp(d.Op), d.Version)
}

func normalizeOp(op string) string {
	switch op {
	case "ge":
		return ">="
	case "le":
		return "<="
	case "gt":
		return ">"
	case "lt":
		return "<"
	case "eq":
		return "="
	}
	return op
}

// Pkg is a unified package entry shared by RPM and DEB indexes.
type Pkg struct {
	Name     string
	Epoch    string
	Version  string
	Release  string
	Arch     string
	Location string // relative path from repo root (or suite root for deb)
	Checksum string
	// ChecksumType is the digest algorithm of Checksum (sha256 | sha1 | md5).
	// Empty means sha256 (the common default).
	ChecksumType string
	Size         int64
	Summary      string
	Requires     []DependencyEntry // hard dependencies
	Recommends   []DependencyEntry // weak dependencies (recommends)
	Provides     []string          // names this package provides
	// MultiArch is the DEB Multi-Arch field (same|foreign|allowed). "foreign"
	// packages can satisfy dependencies from other architectures.
	MultiArch string
	// Files are the file paths this package provides (from RPM filelists.xml).
	// Used to resolve RPM file-path dependencies (e.g. /usr/bin/killall).
	Files []string
	// BaseURL, when non-empty, is the source this package came from (aggregate
	// repos). Used to resolve downloads from the correct upstream.
	BaseURL string
	// Local marks a package that came from input.package_dirs (its metadata was
	// read from the local file, not from upstream metadata). Local packages are
	// published into the repo as-is and their dependencies are still resolved.
	Local bool
}

// Resolve returns the absolute URL for this package's Location, honoring a
// per-package source base when present.
func (p Pkg) Resolve() string {
	base := p.BaseURL
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimPrefix(p.Location, "/")
}

// Key uniquely identifies a package file within a repo (by location).
func (p Pkg) Key() string { return p.Location }

// NEVRA returns name-epoch:version-release.arch for display/debug.
func (p Pkg) NEVRA() string {
	var b strings.Builder
	b.WriteString(p.Name)
	if p.Epoch != "" {
		b.WriteString(":")
		b.WriteString(p.Epoch)
	}
	b.WriteString("-")
	b.WriteString(p.Version)
	if p.Release != "" {
		b.WriteString("-")
		b.WriteString(p.Release)
	}
	b.WriteString(".")
	b.WriteString(p.Arch)
	return b.String()
}

// Index binds a name/identifier to a set of packages and knows how to fetch
// each package file's bytes.
type Index struct {
	// BaseURL without trailing slash, relative to which Location is resolved.
	BaseURL string
	// Packager describes which layout produced this index.
	Backend string // rpm | deb
	// Packages is the full list.
	Packages []Pkg
}

// ResolveLocation returns the absolute URL for a package's Location.
func (ix *Index) ResolveLocation(loc string) string {
	return strings.TrimRight(ix.BaseURL, "/") + "/" + strings.TrimPrefix(loc, "/")
}
