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
	Name       string
	Epoch      string
	Version    string
	Release    string
	Arch       string
	Location   string // relative path from repo root (or suite root for deb)
	Checksum   string
	Size       int64
	Summary    string
	Requires   []DependencyEntry // hard dependencies
	Recommends []DependencyEntry // weak dependencies (recommends)
	Provides   []string          // names this package provides
	// BaseURL, when non-empty, is the source this package came from (aggregate
	// repos). Used to resolve downloads from the correct upstream.
	BaseURL string
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

// ByName returns all packages matching a name (across versions/arches).
func (ix *Index) ByName(name string) []Pkg {
	var out []Pkg
	for _, p := range ix.Packages {
		if p.Name == name {
			out = append(out, p)
		}
	}
	return out
}

// Provider returns the package(s) that satisfy a provides name.
func (ix *Index) Provider(provName string) []Pkg {
	var out []Pkg
	for _, p := range ix.Packages {
		if p.Name == provName {
			out = append(out, p)
		}
		for _, prov := range p.Provides {
			if prov == provName {
				out = append(out, p)
			}
		}
	}
	return out
}
