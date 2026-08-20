package engine

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fanhuadesenlinnn/RepoForge/internal/progress"
	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/fanhuadesenlinnn/RepoForge/internal/upstream"
)

// collectAndCopyInput scans input.package_dirs for existing rpm/deb files,
// copies the ones whose architecture matches the variant's arch set into root,
// and returns the package names to use as starting points (parsed from
// filenames), how many files were copied, how many were skipped because their
// architecture does not match this variant, and the skipped arch breakdown.
// A single directory may mix architectures (e.g. x86_64 + noarch); different
// directories may target different architectures. noarch/all always match.
func collectAndCopyInput(ctx context.Context, r *repo.Repository, root string, archs []string, home string) (names []string, localCopies map[string]string, copied, skipped int, skippedByArch map[string]int, err error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, nil, 0, 0, nil, err
	}
	var files []string
	for _, dir := range r.Input.PackageDirs {
		resolved := resolveInputDir(dir, home)
		f, err := scanPackageFiles(resolved, r.Backend, r.Input.Recursive)
		if err != nil {
			return nil, nil, 0, 0, nil, err
		}
		progress.Infof(ctx, "[输入] 目录 %s：找到 %d 个 %s 文件", dir, len(f), r.Backend)
		files = append(files, f...)
	}
	sort.Strings(files)

	namesMap := map[string]bool{}
	localCopies = map[string]string{} // package name -> flat file name in root
	skippedByArch = map[string]int{}
	for _, src := range files {
		fileArch := pkgArchFromFile(src, r.Backend)
		if !archMatch(fileArch, archs, r.Backend) {
			skipped++
			skippedByArch[fileArch]++
			continue
		}
		name := pkgNameFromFile(src, r.Backend)
		if name != "" {
			namesMap[name] = true
			localCopies[name] = filepath.Base(src)
		}
		dst := filepath.Join(root, filepath.Base(src))
		if err := copyFileIfNeeded(src, dst); err != nil {
			return nil, nil, 0, 0, nil, err
		}
		copied++
	}
	out := make([]string, 0, len(namesMap))
	for n := range namesMap {
		out = append(out, n)
	}
	if copied > 0 {
		progress.Infof(ctx, "[输入] 复制 %d 个本地包（架构匹配）", copied)
	}
	if skipped > 0 {
		var parts []string
		for a, n := range skippedByArch {
			parts = append(parts, fmt.Sprintf("%s: %d", a, n))
		}
		sort.Strings(parts)
		progress.Infof(ctx, "[输入] 跳过 %d 个架构不匹配的包（%s）", skipped, strings.Join(parts, ", "))
	}
	return out, localCopies, copied, skipped, skippedByArch, nil
}

// resolveInputDir resolves a package_dirs entry. Relative paths are first
// tried against the current working directory, then relative to the
// RepoForge home, so a bare name like "tem-rpm-x86" works from anywhere.
func resolveInputDir(dir, home string) string {
	if filepath.IsAbs(dir) {
		return dir
	}
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	candidate := filepath.Join(home, dir)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return dir // let the caller surface the original path in the error
}

// pkgArchFromFile extracts the package architecture from a file name.
// RPM: name-version-release.arch.rpm (arch is the last dot segment)
// DEB: name_version_arch.deb        (arch is the last underscore segment)
// Returns "" when the name has no arch segment (unusual / test fixtures).
func pkgArchFromFile(file, backend string) string {
	base := filepath.Base(file)
	if backend == "rpm" {
		s := strings.TrimSuffix(base, ".rpm")
		if i := strings.LastIndex(s, "."); i >= 0 && i < len(s)-1 {
			return s[i+1:]
		}
		return ""
	}
	s := strings.TrimSuffix(base, ".deb")
	if i := strings.LastIndex(s, "_"); i >= 0 && i < len(s)-1 {
		return s[i+1:]
	}
	return ""
}

// archMatch reports whether a local package file's architecture belongs to the
// variant's architecture set. noarch (rpm) and all (deb) always match; a file
// whose architecture cannot be parsed is not rejected.
func archMatch(fileArch string, archs []string, backend string) bool {
	if fileArch == "" {
		return true
	}
	if len(archs) == 0 {
		return true
	}
	for _, a := range archs {
		if fileArch == a || fileArch == "*" {
			return true
		}
	}
	if backend == "rpm" && fileArch == "noarch" {
		return true
	}
	if backend == "deb" && fileArch == "all" {
		return true
	}
	return false
}

// splitPathList splits a possibly colon-separated path list (Unix PATH style),
// preserving Windows drive letters (e.g. C:\pkgs stays a single entry).
func splitPathList(s string) []string {
	if filepath.VolumeName(s) != "" {
		return []string{s}
	}
	return strings.Split(s, ":")
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
	for _, d := range splitPathList(dir) {
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
// Priority: upstream.arch > target.arch > the expanded variant's $basearch
// > backend default. The $basearch step matters for multi-arch configs that
// expand the same repo into x86_64 and aarch64 variants: without it, the
// aarch64 variant would be solved against x86_64-only packages and resolve
// nothing.
func archList(r *repo.Repository, vars map[string]string) []string {
	if len(r.Upstream.Arch) > 0 {
		return append([]string{}, r.Upstream.Arch...)
	}
	if r.Target.Arch != "" {
		if r.Backend == "rpm" {
			return []string{r.Target.Arch, "noarch"}
		}
		return []string{r.Target.Arch, "all"}
	}
	if base := vars["basearch"]; base != "" {
		if r.Backend == "rpm" {
			return []string{base, "noarch"}
		}
		return []string{base, "all"}
	}
	if r.Backend == "rpm" {
		return []string{"x86_64", "noarch"}
	}
	return []string{"amd64", "all"}
}
