package upstream

import (
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
	xmlData, err := decompressAny(raw, href)
	if err != nil {
		return nil, err
	}
	pkgs, err := parsePrimaryXML(xmlData)
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

// fetchFilelists downloads and parses filelists.xml into a map keyed by pkgid.
func fetchFilelists(ctx context.Context, base, href string) map[string][]string {
	url := base + "/" + href
	raw, err := Fetch(ctx, url)
	if err != nil {
		return nil
	}
	data, err := decompressAny(raw, href)
	if err != nil {
		return nil
	}
	return parseFilelistsXML(data)
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
	switch {
	case strings.HasSuffix(name, ".gz"):
		r, err := gzip.NewReader(strings.NewReader(string(data)))
		if err != nil {
			return nil, fmt.Errorf("解压 %s 失败: %w", name, err)
		}
		defer r.Close()
		return io.ReadAll(r)
	case strings.HasSuffix(name, ".zst"):
		return nil, fmt.Errorf("暂不支持 zstd 压缩元数据 %s（当前仅支持 gzip）", name)
	default:
		return data, nil
	}
}

// ---- primary.xml parsing ----

type primaryXML struct {
	Packages []primaryPkg `xml:"package"`
}
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
	var pm primaryXML
	if err := xml.Unmarshal(data, &pm); err != nil {
		return nil, fmt.Errorf("解析 primary.xml 失败: %w", err)
	}
	out := make([]Pkg, 0, len(pm.Packages))
	for _, p := range pm.Packages {
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
		out = append(out, pkg)
	}
	return out, nil
}

// ---- filelists.xml parsing ----

type filelistsXML struct {
	Packages []filelistPkg `xml:"package"`
}
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
	var fl filelistsXML
	if xml.Unmarshal(data, &fl) != nil {
		return nil
	}
	out := make(map[string][]string, len(fl.Packages))
	for _, p := range fl.Packages {
		out[p.PkgID] = p.Files
	}
	return out
}

// mergeFilelists attaches file provides to packages matched by pkgid (checksum).
func mergeFilelists(pkgs []Pkg, files map[string][]string) {
	for i := range pkgs {
		if f, ok := files[pkgs[i].Checksum]; ok && len(f) > 0 {
			pkgs[i].Files = f
		}
	}
}
