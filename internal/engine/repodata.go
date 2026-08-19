package engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/RepoForge/internal/progress"
	"github.com/fanhuadesenlinnn/RepoForge/internal/upstream"
)

// genRepodata writes a yum/apt-compatible index for the subset of packages.
// It returns the path to the generated repodata entry point.
func genRepodata(ctx context.Context, root string, subset []upstream.Pkg, backend string) (string, error) {
	progress.Infof(ctx, "[索引] 生成 %s 元数据（%d 包）", backend, len(subset))
	if backend == "deb" {
		return genDEBPackages(ctx, root, subset)
	}
	return genRPMMetadata(ctx, root, subset)
}

// presentPkgs keeps only packages whose file is already on disk.
func presentPkgs(root string, pkgs []upstream.Pkg) []upstream.Pkg {
	out := make([]upstream.Pkg, 0, len(pkgs))
	for _, p := range pkgs {
		loc := strings.TrimPrefix(p.Location, "/")
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(loc))); err == nil {
			out = append(out, p)
		}
	}
	return out
}

type repoDataFile struct {
	Type     string
	Name     string
	Bytes    []byte
	OpenSum  [32]byte
	OpenSize int64
}

// ---- RPM: write repodata/repomd.xml + primary.xml.gz [+ filelists.xml.gz] ----

func genRPMMetadata(_ context.Context, root string, subset []upstream.Pkg) (string, error) {
	dir := filepath.Join(root, "repodata")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := clearGeneratedRPM(dir); err != nil {
		return "", err
	}

	ts := time.Now().Unix()
	var files []repoDataFile
	primary, err := gzipXML(generatePrimaryXML(subset))
	if err != nil {
		return "", err
	}
	files = append(files, repoDataFile{
		Type:     "primary",
		Name:     fmt.Sprintf("%d-primary.xml.gz", ts),
		Bytes:    primary.raw,
		OpenSum:  primary.openSum,
		OpenSize: primary.openSize,
	})
	if hasFiles(subset) {
		fl, ferr := gzipXML(generateFilelistsXML(subset))
		if ferr != nil {
			return "", ferr
		}
		files = append(files, repoDataFile{
			Type:     "filelists",
			Name:     fmt.Sprintf("%d-filelists.xml.gz", ts),
			Bytes:    fl.raw,
			OpenSum:  fl.openSum,
			OpenSize: fl.openSize,
		})
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f.Name), f.Bytes, 0o644); err != nil {
			return "", err
		}
	}
	repomdPath := filepath.Join(dir, "repomd.xml")
	if err := os.WriteFile(repomdPath, buildRepomd(files), 0o644); err != nil {
		return "", err
	}
	return repomdPath, nil
}

func clearGeneratedRPM(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if name == "repomd.xml" || strings.HasSuffix(name, "-primary.xml.gz") || strings.HasSuffix(name, "-filelists.xml.gz") {
			if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func hasFiles(subset []upstream.Pkg) bool {
	for _, p := range subset {
		if len(p.Files) > 0 {
			return true
		}
	}
	return false
}

type gzBlob struct {
	raw      []byte
	openSum  [32]byte
	openSize int64
}

func gzipXML(digest []byte) (gzBlob, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(digest); err != nil {
		return gzBlob{}, err
	}
	if err := gw.Close(); err != nil {
		return gzBlob{}, err
	}
	return gzBlob{raw: buf.Bytes(), openSum: sha256.Sum256(digest), openSize: int64(len(digest))}, nil
}

func generatePrimaryXML(subset []upstream.Pkg) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b, `<metadata xmlns="http://linux.duke.edu/metadata/common" xmlns:rpm="http://linux.duke.edu/metadata/rpm" packages="%d">`+"\n", len(subset))
	for _, p := range subset {
		b.WriteString("  <package type=\"rpm\">\n")
		fmt.Fprintf(&b, "    <name>%s</name>\n", xmlEscape(p.Name))
		fmt.Fprintf(&b, "    <arch>%s</arch>\n", xmlEscape(p.Arch))
		fmt.Fprintf(&b, "    <version epoch=\"%s\" ver=\"%s\" rel=\"%s\"/>\n", xmlEscape(emptyEpoch(p.Epoch)), xmlEscape(p.Version), xmlEscape(p.Release))
		href := strings.TrimPrefix(p.Location, "/")
		fmt.Fprintf(&b, "    <checksum type=\"sha256\" pkgid=\"YES\">%s</checksum>\n", xmlEscape(p.Checksum))
		fmt.Fprintf(&b, "    <location href=\"%s\"/>\n", xmlEscape(href))
		fmt.Fprintf(&b, "    <size package=\"%d\"/>\n", p.Size)
		fmt.Fprintf(&b, "    <summary>%s</summary>\n", xmlEscape(p.Summary))
		b.WriteString("    <format>\n")
		writeRPMEntries(&b, "provides", rpmProvides(p))
		writeRPMEntries(&b, "requires", p.Requires)
		if len(p.Recommends) > 0 {
			writeRPMEntries(&b, "recommends", p.Recommends)
		}
		b.WriteString("    </format>\n")
		b.WriteString("  </package>\n")
	}
	b.WriteString("</metadata>\n")
	return []byte(b.String())
}

func rpmProvides(p upstream.Pkg) []upstream.DependencyEntry {
	seen := map[string]bool{}
	var out []upstream.DependencyEntry
	add := func(name, op, ver string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, upstream.DependencyEntry{Name: name, Op: op, Version: ver})
	}
	add(p.Name, "EQ", joinVerRel(p.Version, p.Release))
	for _, name := range p.Provides {
		add(name, "", "")
	}
	return out
}

func joinVerRel(ver, rel string) string {
	if rel == "" {
		return ver
	}
	return ver + "-" + rel
}

func writeRPMEntries(b *strings.Builder, kind string, entries []upstream.DependencyEntry) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintf(b, "      <rpm:%s>\n", kind)
	for _, e := range entries {
		fmt.Fprintf(b, "        <rpm:entry name=\"%s\"", xmlEscape(e.Name))
		if flags := rpmFlags(e.Op); flags != "" && e.Version != "" {
			fmt.Fprintf(b, " flags=\"%s\" ver=\"%s\"", flags, xmlEscape(e.Version))
		}
		b.WriteString("/>\n")
	}
	fmt.Fprintf(b, "      </rpm:%s>\n", kind)
}

func rpmFlags(op string) string {
	switch strings.ToUpper(strings.TrimSpace(op)) {
	case "GE", ">=":
		return "GE"
	case "LE", "<=":
		return "LE"
	case "GT", ">":
		return "GT"
	case "LT", "<":
		return "LT"
	case "EQ", "=", "==":
		return "EQ"
	default:
		return ""
	}
}

func emptyEpoch(e string) string {
	if e == "" {
		return "0"
	}
	return e
}

func generateFilelistsXML(subset []upstream.Pkg) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b, `<filelists xmlns="http://linux.duke.edu/metadata/filelists" packages="%d">`+"\n", len(subset))
	for _, p := range subset {
		if len(p.Files) == 0 {
			continue
		}
		fmt.Fprintf(&b, "  <package pkgid=\"%s\" name=\"%s\" arch=\"%s\">\n", xmlEscape(p.Checksum), xmlEscape(p.Name), xmlEscape(p.Arch))
		fmt.Fprintf(&b, "    <version epoch=\"%s\" ver=\"%s\" rel=\"%s\"/>\n", xmlEscape(emptyEpoch(p.Epoch)), xmlEscape(p.Version), xmlEscape(p.Release))
		for _, f := range p.Files {
			fmt.Fprintf(&b, "    <file>%s</file>\n", xmlEscape(f))
		}
		b.WriteString("  </package>\n")
	}
	b.WriteString("</filelists>\n")
	return []byte(b.String())
}

func buildRepomd(files []repoDataFile) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString("<repomd xmlns=\"http://linux.duke.edu/metadata/repo\" xmlns:rpm=\"http://linux.duke.edu/metadata/rpm\">\n")
	fmt.Fprintf(&b, "  <revision>%d</revision>\n", time.Now().Unix())
	for _, f := range files {
		sum := sha256.Sum256(f.Bytes)
		fmt.Fprintf(&b, "  <data type=\"%s\">\n", xmlEscape(f.Type))
		fmt.Fprintf(&b, "    <location href=\"repodata/%s\"/>\n", xmlEscape(f.Name))
		fmt.Fprintf(&b, "    <checksum type=\"sha256\">%s</checksum>\n", hex.EncodeToString(sum[:]))
		fmt.Fprintf(&b, "    <open-checksum type=\"sha256\">%s</open-checksum>\n", hex.EncodeToString(f.OpenSum[:]))
		fmt.Fprintf(&b, "    <timestamp>%d</timestamp>\n", time.Now().Unix())
		fmt.Fprintf(&b, "    <size>%d</size>\n", len(f.Bytes))
		fmt.Fprintf(&b, "    <open-size>%d</open-size>\n", f.OpenSize)
		b.WriteString("  </data>\n")
	}
	b.WriteString("</repomd>\n")
	return []byte(b.String())
}

// ---- DEB: write Packages file ----

func genDEBPackages(_ context.Context, root string, subset []upstream.Pkg) (string, error) {
	var b strings.Builder
	for _, p := range subset {
		fmt.Fprintf(&b, "Package: %s\n", p.Name)
		ver := p.Version
		if p.Epoch != "" {
			ver = p.Epoch + ":" + ver
		}
		if p.Release != "" {
			ver = ver + "-" + p.Release
		}
		fmt.Fprintf(&b, "Version: %s\n", ver)
		fmt.Fprintf(&b, "Architecture: %s\n", archOrDefault(p.Arch))
		fmt.Fprintf(&b, "Filename: %s\n", strings.TrimPrefix(p.Location, "/"))
		fmt.Fprintf(&b, "SHA256: %s\n", p.Checksum)
		fmt.Fprintf(&b, "Size: %d\n", p.Size)
		if p.Summary != "" {
			fmt.Fprintf(&b, "Description: %s\n", strings.ReplaceAll(p.Summary, "\n", " "))
		}
		if len(p.Requires) > 0 {
			fmt.Fprintf(&b, "Depends: %s\n", formatDebDeps(p.Requires))
		}
		if len(p.Provides) > 0 {
			fmt.Fprintf(&b, "Provides: %s\n", strings.Join(p.Provides, ", "))
		}
		b.WriteString("\n")
	}
	path := filepath.Join(root, "Packages")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func formatDebDeps(deps []upstream.DependencyEntry) string {
	parts := make([]string, 0, len(deps))
	for _, d := range deps {
		parts = append(parts, d.String())
	}
	return strings.Join(parts, ", ")
}

func archOrDefault(a string) string {
	if a == "" {
		return "all"
	}
	return a
}

func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}
