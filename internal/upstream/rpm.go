package upstream

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// RPMIndex fetches and parses an RPM repository's metadata into a unified Index
// (primary metadata only; no filelists). Use RPMIndexForSolve to also include
// filelists for dependency resolution.
func RPMIndex(ctx context.Context, baseURL string) (*Index, error) {
	return rpmIndex(ctx, baseURL, false)
}

// RPMIndexForSolve fetches primary plus filelists.xml (when present) so
// file-path dependencies can be resolved. Used by the make/install solver.
func RPMIndexForSolve(ctx context.Context, baseURL string) (*Index, error) {
	return rpmIndex(ctx, baseURL, true)
}

func rpmIndex(ctx context.Context, baseURL string, withFilelists bool) (*Index, error) {
	base := strings.TrimRight(baseURL, "/")
	repomdURL := base + "/repodata/repomd.xml"
	data, err := Fetch(ctx, repomdURL)
	if err != nil {
		return nil, fmt.Errorf("读取上游 repomd.xml: %w", err)
	}
	var rd repomd
	if err := xml.Unmarshal(data, &rd); err != nil {
		return nil, fmt.Errorf("解析 repomd.xml 失败: %w", err)
	}
	var primary *dataLoc
	for i := range rd.Data {
		if rd.Data[i].Type == "primary" {
			primary = &rd.Data[i]
			break
		}
	}
	if primary == nil {
		return nil, fmt.Errorf("repomd.xml 中未找到 primary 元数据")
	}
	href := strings.TrimPrefix(primary.Location.Href, "/")
	primaryURL := base + "/" + href
	raw, err := Fetch(ctx, primaryURL)
	if err != nil {
		return nil, fmt.Errorf("读取 %s: %w", primaryURL, err)
	}
	stream, err := openDecompressed(bytes.NewReader(raw), href)
	if err != nil {
		return nil, err
	}
	pkgs, err := parsePrimaryXMLReader(stream)
	stream.Close()
	if err != nil {
		return nil, err
	}

	// Fetch + merge filelists.xml (when resolving dependencies and the repo
	// provides it) so file-path dependencies (e.g. /usr/bin/killall) can be
	// resolved precisely — the same way YUM uses filelists. Skipped for pure
	// mirroring (sync) where dependency resolution is not needed, avoiding the
	// extra large download on slow mirrors. Missing filelists is not an error.
	if withFilelists {
		if fh := findData(&rd, "filelists"); fh != "" {
			if fl := fetchFilelists(ctx, base, fh); len(fl) > 0 {
				mergeFilelists(pkgs, fl)
			}
		}
	}

	return &Index{BaseURL: base, Backend: "rpm", Packages: pkgs}, nil
}

// findData returns the href for a repomd data type ("" if absent).
func findData(rd *repomd, typ string) string {
	for i := range rd.Data {
		if rd.Data[i].Type == typ {
			return strings.TrimPrefix(rd.Data[i].Location.Href, "/")
		}
	}
	return ""
}

// fetchFilelists downloads and stream-parses filelists.xml into a map keyed
// by pkgid. Streaming avoids materializing the uncompressed XML (Kylin
// filelists is ~10MB gzip / ~160MB raw) as a single buffer + DOM tree.
func fetchFilelists(ctx context.Context, base, href string) map[string][]string {
	url := base + "/" + href
	raw, err := Fetch(ctx, url)
	if err != nil {
		return nil
	}
	stream, err := openDecompressed(bytes.NewReader(raw), href)
	if err != nil {
		return nil
	}
	defer stream.Close()
	return parseFilelistsXMLReader(stream)
}

type repomd struct {
	Revision string    `xml:"revision"`
	Data     []dataLoc `xml:"data"`
}
type dataLoc struct {
	Type     string   `xml:"type,attr"`
	Location location `xml:"location"`
}
type location struct {
	Href string `xml:"href,attr"`
}

func decompressAny(data []byte, name string) ([]byte, error) {
	r, err := openDecompressed(bytes.NewReader(data), name)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func openDecompressed(r io.Reader, name string) (io.ReadCloser, error) {
	switch {
	case strings.HasSuffix(name, ".gz"):
		gz, err := gzip.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("解压 %s 失败: %w", name, err)
		}
		return gz, nil
	case strings.HasSuffix(name, ".zst"):
		return nil, fmt.Errorf("暂不支持 zstd 压缩元数据 %s（当前仅支持 gzip）", name)
	default:
		return io.NopCloser(r), nil
	}
}

// ---- primary.xml parsing ----

type primaryPkg struct {
	Name       string      `xml:"name"`
	Arch       string      `xml:"arch"`
	Summary    string      `xml:"summary"`
	Version    pkgVersion  `xml:"version"`
	Checksum   pkgChecksum `xml:"checksum"`
	Location   pkgLocation `xml:"location"`
	Size       pkgSize     `xml:"size"`
	Requires   []pkgRel    `xml:"format>requires>entry"`
	Provides   []pkgRel    `xml:"format>provides>entry"`
	Recommends []pkgRel    `xml:"format>recommends>entry"`
}
type pkgVersion struct {
	Epoch string `xml:"epoch,attr"`
	Ver   string `xml:"ver,attr"`
	Rel   string `xml:"rel,attr"`
}
type pkgChecksum struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}
type pkgLocation struct {
	Href string `xml:"href,attr"`
}
type pkgSize struct {
	Package int64 `xml:"package,attr"`
}
type pkgRel struct {
	Name  string `xml:"name,attr"`
	Flags string `xml:"flags,attr"`
	Epoch string `xml:"epoch,attr"`
	Ver   string `xml:"ver,attr"`
	Rel   string `xml:"rel,attr"`
}

func parsePrimaryXML(data []byte) ([]Pkg, error) {
	return parsePrimaryXMLReader(bytes.NewReader(data))
}

// parsePrimaryXMLReader stream-decodes one <package> at a time so a 40MB
// primary.xml (Kylin base) never sits in memory as a full DOM tree.
func parsePrimaryXMLReader(r io.Reader) ([]Pkg, error) {
	dec := xml.NewDecoder(r)
	var out []Pkg
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("解析 primary.xml 失败: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "package" {
			continue
		}
		var p primaryPkg
		if err := dec.DecodeElement(&p, &se); err != nil {
			return nil, fmt.Errorf("解析 primary.xml 失败: %w", err)
		}
		out = append(out, primaryToPkg(p))
	}
}

func primaryToPkg(p primaryPkg) Pkg {
	pkg := Pkg{
		Name:     p.Name,
		Epoch:    p.Version.Epoch,
		Version:  p.Version.Ver,
		Release:  p.Version.Rel,
		Arch:     p.Arch,
		Location: p.Location.Href,
		Checksum: p.Checksum.Text,
		Size:     p.Size.Package,
		Summary:  p.Summary,
	}
	for _, r := range p.Requires {
		pkg.Requires = append(pkg.Requires, DependencyEntry{Name: r.Name, Op: r.Flags, Version: r.Ver})
	}
	for _, r := range p.Recommends {
		pkg.Recommends = append(pkg.Recommends, DependencyEntry{Name: r.Name, Op: r.Flags, Version: r.Ver})
	}
	for _, r := range p.Provides {
		pkg.Provides = append(pkg.Provides, r.Name)
	}
	return pkg
}

// ---- filelists.xml parsing ----

type filelistPkg struct {
	PkgID   string   `xml:"pkgid,attr"`
	Name    string   `xml:"name,attr"`
	Arch    string   `xml:"arch,attr"`
	Version flVer    `xml:"version"`
	Files   []string `xml:"file"`
}
type flVer struct {
	Epoch string `xml:"epoch,attr"`
	Ver   string `xml:"ver,attr"`
	Rel   string `xml:"rel,attr"`
}

func parseFilelistsXML(data []byte) map[string][]string {
	return parseFilelistsXMLReader(bytes.NewReader(data))
}

func parseFilelistsXMLReader(r io.Reader) map[string][]string {
	dec := xml.NewDecoder(r)
	out := make(map[string][]string)
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				return out
			}
			return nil
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "package" {
			continue
		}
		var p filelistPkg
		if err := dec.DecodeElement(&p, &se); err != nil {
			return nil
		}
		if p.PkgID != "" {
			out[p.PkgID] = p.Files
		}
	}
}

// mergeFilelists attaches file provides to packages matched by pkgid (checksum).
func mergeFilelists(pkgs []Pkg, files map[string][]string) {
	for i := range pkgs {
		if f, ok := files[pkgs[i].Checksum]; ok && len(f) > 0 {
			pkgs[i].Files = f
		}
	}
}
