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

	"github.com/fanhuadesenlinnn/RepoForge/internal/upstream"
)

// genRepodata writes a yum/apt-compatible index for the subset of packages.
// It returns the path to the generated repodata entry point.
func genRepodata(ctx context.Context, root string, subset []upstream.Pkg, backend string) (string, error) {
	if backend == "deb" {
		return genDEBPackages(ctx, root, subset)
	}
	return genRPMMetadata(ctx, root, subset)
}

// ---- RPM: write repodata/repomd.xml + <id>-primary.xml.gz ----

func genRPMMetadata(_ context.Context, root string, subset []upstream.Pkg) (string, error) {
	dir := filepath.Join(root, "repodata")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ts := time.Now().Unix()
	primaryName := fmt.Sprintf("%d-primary.xml.gz", ts)
	digest := generatePrimaryXML(subset)

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write(digest)
	gw.Close()
	primaryBytes := buf.Bytes()

	primaryChecksum := sha256.Sum256(primaryBytes)
	primaryPath := filepath.Join(dir, primaryName)
	if err := os.WriteFile(primaryPath, primaryBytes, 0o644); err != nil {
		return "", err
	}

	openSum := sha256.Sum256(digest)
	repomd := buildRepomd(primaryName, primaryChecksum, openSum, int64(len(primaryBytes)), int64(len(digest)))
	repomdPath := filepath.Join(dir, "repomd.xml")
	if err := os.WriteFile(repomdPath, repomd, 0o644); err != nil {
		return "", err
	}
	return repomdPath, nil
}

func generatePrimaryXML(subset []upstream.Pkg) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	fmt.Fprintf(&b, `<metadata xmlns="http://linux.duke.edu/metadata/common" xmlns:rpm="http://linux.duke.edu/metadata/rpm" packages="%d">`+"\n", len(subset))
	for _, p := range subset {
		b.WriteString("  <package type=\"rpm\">\n")
		fmt.Fprintf(&b, "    <name>%s</name>\n", xmlEscape(p.Name))
		fmt.Fprintf(&b, "    <arch>%s</arch>\n", xmlEscape(p.Arch))
		fmt.Fprintf(&b, "    <version epoch=\"%s\" ver=\"%s\" rel=\"%s\"/>\n", xmlEscape(p.Epoch), xmlEscape(p.Version), xmlEscape(p.Release))
		href := strings.TrimPrefix(p.Location, "/")
		fmt.Fprintf(&b, "    <checksum type=\"sha256\" pkgid=\"YES\">%s</checksum>\n", xmlEscape(p.Checksum))
		fmt.Fprintf(&b, "    <location href=\"%s\"/>\n", xmlEscape(href))
		fmt.Fprintf(&b, "    <size package=\"%d\"/>\n", p.Size)
		fmt.Fprintf(&b, "    <summary>%s</summary>\n", xmlEscape(p.Summary))
		b.WriteString("  </package>\n")
	}
	b.WriteString("</metadata>\n")
	return []byte(b.String())
}

func buildRepomd(primaryName string, primarySum, openSum [32]byte, primarySize, openSize int64) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString("<repomd xmlns=\"http://linux.duke.edu/metadata/repo\" xmlns:rpm=\"http://linux.duke.edu/metadata/rpm\">\n")
	fmt.Fprintf(&b, "  <revision>%d</revision>\n", time.Now().Unix())
	fmt.Fprintf(&b, "  <data type=\"primary\">\n")
	fmt.Fprintf(&b, "    <location href=\"repodata/%s\"/>\n", xmlEscape(primaryName))
	fmt.Fprintf(&b, "    <checksum type=\"sha256\">%s</checksum>\n", hex.EncodeToString(primarySum[:]))
	fmt.Fprintf(&b, "    <open-checksum type=\"sha256\">%s</open-checksum>\n", hex.EncodeToString(openSum[:]))
	fmt.Fprintf(&b, "    <timestamp>%d</timestamp>\n", time.Now().Unix())
	fmt.Fprintf(&b, "    <size>%d</size>\n", primarySize)
	fmt.Fprintf(&b, "    <open-size>%d</open-size>\n", openSize)
	b.WriteString("  </data>\n")
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
