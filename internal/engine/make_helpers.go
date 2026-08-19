package engine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/fanhuadesenlinnn/RepoForge/internal/upstream"
)

// collectAndCopyInput scans input.package_dirs for existing rpm/deb files,
// copies them into root, and returns the package names to use as starting
// points (parsed from filenames) plus how many files were copied.
func collectAndCopyInput(r *repo.Repository, root string) ([]string, int, error) {
	var files []string
	for _, dir := range r.Input.PackageDirs {
		f, err := scanPackageFiles(dir, r.Backend, r.Input.Recursive)
		if err != nil {
			return nil, 0, err
		}
		files = append(files, f...)
	}
	sort.Strings(files)

	names := map[string]bool{}
	copied := 0
	for _, src := range files {
		name := pkgNameFromFile(src, r.Backend)
		if name != "" {
			names[name] = true
		}
		dst := filepath.Join(root, filepath.Base(src))
		if err := copyFileIfNeeded(src, dst); err != nil {
			return nil, 0, err
		}
		copied++
	}
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	return out, copied, nil
}

func scanPackageFiles(dir, backend string, recursive bool) ([]string, error) {
	var suffix string
	if backend == "rpm" {
		suffix = ".rpm"
	} else {
		suffix = ".deb"
	}
	var out []string
	var walk func(string) error
	walk = func(d string) error {
		entries, err := os.ReadDir(d)
		if err != nil {
			return fmt.Errorf("读取输入目录 %s 失败: %w", d, err)
		}
		for _, e := range entries {
			p := filepath.Join(d, e.Name())
			if e.IsDir() {
				if recursive {
					if err := walk(p); err != nil {
						return err
					}
				}
				continue
			}
			if strings.HasSuffix(strings.ToLower(e.Name()), suffix) {
				out = append(out, p)
			}
		}
		return nil
	}
	for _, d := range strings.Split(dir, ":") {
		if d == "" {
			continue
		}
		if err := walk(d); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// pkgNameFromFile extracts the package name from a file name.
// RPM: name-version-release.arch.rpm (name is before first '-' followed by digit).
// DEB: name_version_arch.deb.
func pkgNameFromFile(file, backend string) string {
	base := filepath.Base(file)
	if backend == "rpm" {
		// strip .rpm
		s := strings.TrimSuffix(base, ".rpm")
		// name-ver-rel.arch ; name ends at first "-N" where N is a digit
		for i := 0; i < len(s); i++ {
			if s[i] == '-' && i+1 < len(s) && isDigit(s[i+1]) {
				return s[:i]
			}
		}
		return s
	}
	// DEB: name_ver_arch.deb
	s := strings.TrimSuffix(base, ".deb")
	if i := strings.Index(s, "_"); i >= 0 {
		return s[:i]
	}
	return s
}

func copyFileIfNeeded(src, dst string) error {
	if st, err := os.Stat(dst); err == nil {
		ss, _ := os.Stat(src)
		if ss != nil && st.Size() == ss.Size() {
			return nil // already copied
		}
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// latestVersion returns the newest package in the index matching name.
func latestVersion(ix *upstream.Index, name string, opt SolveOptions) (upstream.Pkg, error) {
	cands := providers(ix, name, func(p upstream.Pkg) bool {
		return archOKFor(p, opt.Archs, opt.Backend)
	})
	if len(cands) == 0 {
		return upstream.Pkg{}, fmt.Errorf("未在上游找到 %q", name)
	}
	best := cands[0]
	for _, c := range cands[1:] {
		if pkgNewer(c, best, opt.Backend) > 0 {
			best = c
		}
	}
	return best, nil
}

func archOKFor(p upstream.Pkg, archs []string, backend string) bool {
	if len(archs) == 0 {
		return true
	}
	for _, a := range archs {
		if a == p.Arch || a == "*" {
			return true
		}
	}
	if backend == "rpm" && p.Arch == "noarch" {
		return true
	}
	if backend == "deb" && p.Arch == "all" {
		return true
	}
	return false
}

// archList returns the build architecture list (used by dependency solving).
func archList(r *repo.Repository) []string {
	if len(r.Upstream.Arch) > 0 {
		return append([]string{}, r.Upstream.Arch...)
	}
	if r.Target.Arch != "" {
		if r.Backend == "rpm" {
			return []string{r.Target.Arch, "noarch"}
		}
		return []string{r.Target.Arch, "all"}
	}
	if r.Backend == "rpm" {
		return []string{"x86_64", "noarch"}
	}
	return []string{"amd64", "all"}
}
