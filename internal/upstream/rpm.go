package upstream

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/gob"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fanhuadesenlinnn/RepoForge/internal/progress"
	"github.com/klauspost/compress/zstd"
)

// RPMIndex fetches and parses an RPM repository's metadata into a unified Index
// (primary metadata only; no filelists). Use RPMIndexForSolve to also include
// filelists for dependency resolution. cacheDir, when non-empty, caches the
// downloaded primary/filelists files by their repomd checksum (yum/apt style),
// so re-running against the same metadata skips the download.
func RPMIndex(ctx context.Context, baseURL, cacheDir string) (*Index, error) {
	return rpmIndex(ctx, baseURL, false, cacheDir)
}

// RPMIndexForSolve fetches primary plus filelists.xml (when present) so
// file-path dependencies can be resolved. Used by the make/install solver.
func RPMIndexForSolve(ctx context.Context, baseURL, cacheDir string) (*Index, error) {
	return rpmIndex(ctx, baseURL, true, cacheDir)
}

func rpmIndex(ctx context.Context, baseURL string, withFilelists bool, cacheDir string) (*Index, error) {
	base := strings.TrimRight(baseURL, "/")
	repomdURL := base + "/repodata/repomd.xml"
	progress.Infof(ctx, "[元数据] 读取 repomd.xml  %s", repomdURL)
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
	progress.Infof(ctx, "[元数据] 读取 primary.xml  %s", href)
	pkgs, err := parseCached(ctx, cacheDir, primary.Checksum.Text, func() ([]Pkg, error) {
		raw, err := fetchCached(ctx, primaryURL, cacheDir, primary.Checksum.Text, href)
		if err != nil {
			return nil, fmt.Errorf("读取 %s: %w", primaryURL, err)
		}
		stream, err := openDecompressed(bytes.NewReader(raw), href)
		if err != nil {
			return nil, err
		}
		defer stream.Close()
		return parsePrimaryXMLReader(stream)
	})
	if err != nil {
		return nil, err
	}
	progress.Infof(ctx, "[元数据] 已解析 primary.xml（%d 包）", len(pkgs))

	// Fetch + merge filelists.xml (when resolving dependencies and the repo
	// provides it) so file-path dependencies (e.g. /usr/bin/killall) can be
	// resolved precisely — the same way YUM uses filelists. Skipped for pure
	// mirroring (sync) where dependency resolution is not needed, avoiding the
	// extra large download on slow mirrors. Missing filelists is not an error.
	if withFilelists {
		if fh := findData(&rd, "filelists"); fh != "" {
			progress.Infof(ctx, "[元数据] 读取 filelists.xml  %s", fh)
			if fl := fetchFilelists(ctx, base, fh, cacheDir, filelistsChecksum(&rd)); len(fl) > 0 {
				mergeFilelists(pkgs, fl)
				progress.Infof(ctx, "[元数据] 已合并 filelists（%d 包含文件列表）", len(fl))
			}
		}
	}

	return &Index{BaseURL: base, Backend: "rpm", Packages: pkgs}, nil
}

// filelistsChecksum returns the sha256 checksum of the filelists entry, or "".
func filelistsChecksum(rd *repomd) string {
	for i := range rd.Data {
		if rd.Data[i].Type == "filelists" {
			return rd.Data[i].Checksum.Text
		}
	}
	return ""
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
// by pkgid, caching the parsed map as gob. Streaming avoids materializing the
// uncompressed XML (Kylin filelists is ~10MB gzip / ~160MB raw) as a single
// buffer + DOM tree.
func fetchFilelists(ctx context.Context, base, href, cacheDir, checksum string) map[string][]string {
	url := base + "/" + href
	fl, _ := parseCached(ctx, cacheDir, checksum+"-fl", func() (map[string][]string, error) {
		raw, err := fetchCached(ctx, url, cacheDir, checksum, href)
		if err != nil {
			return nil, err
		}
		stream, err := openDecompressed(bytes.NewReader(raw), href)
		if err != nil {
			return nil, err
		}
		defer stream.Close()
		return parseFilelistsXMLReader(stream), nil
	})
	return fl
}

// fetchCached downloads url and caches the raw bytes under cacheDir/metadata/
// keyed by the repomd sha256 checksum (yum/apt style). A cache hit skips the
// network entirely. Returns the raw (still compressed) file bytes.
func fetchCached(ctx context.Context, url, cacheDir, checksum, name string) ([]byte, error) {
	if cacheDir != "" && checksum != "" {
		if raw, ok := readCacheFile(cacheDir, checksum); ok {
			progress.Infof(ctx, "[元数据] 缓存命中 %s（跳过下载）", filepath.Base(name))
			return raw, nil
		}
	}
	raw, err := Fetch(ctx, url)
	if err != nil {
		return nil, err
	}
	if cacheDir != "" && checksum != "" {
		writeCacheFile(cacheDir, checksum, raw)
	}
	return raw, nil
}

// parseCached runs compute (which downloads + parses metadata) and caches its
// parsed result as gob, keyed by the repomd checksum — the way yum's sqlite
// cache and apt's pkgcache.bin store parsed metadata. Re-runs deserialize the
// gob instead of re-downloading and re-parsing multi-hundred-MB XML.
func parseCached[T any](ctx context.Context, cacheDir, checksum string, compute func() (T, error)) (T, error) {
	if cacheDir != "" && checksum != "" {
		if raw, ok := readCacheFile(cacheDir, checksum+".gob"); ok {
			var v T
			if err := gob.NewDecoder(bytes.NewReader(raw)).Decode(&v); err == nil {
				return v, nil
			}
			// Corrupt or incompatible cache: fall through and recompute.
		}
	}
	v, err := compute()
	if err != nil {
		return v, err
	}
	if cacheDir != "" && checksum != "" {
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(v); err == nil {
			writeCacheFile(cacheDir, checksum+".gob", buf.Bytes())
		}
	}
	return v, nil
}

func readCacheFile(cacheDir, checksum string) ([]byte, bool) {
	path := filepath.Join(cacheDir, "metadata", checksum)
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return nil, false
	}
	return raw, true
}

// writeCacheFile stores the metadata atomically (tmp + rename) so a crash
// mid-write never leaves a truncated cache entry.
func writeCacheFile(cacheDir, checksum string, raw []byte) {
	dir := filepath.Join(cacheDir, "metadata")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	tmp := filepath.Join(dir, "."+checksum+".tmp")
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, filepath.Join(dir, checksum))
}

type repomd struct {
	Revision string    `xml:"revision"`
	Data     []dataLoc `xml:"data"`
}
type dataLoc struct {
	Type     string   `xml:"type,attr"`
	Checksum chkSum   `xml:"checksum"`
	Location location `xml:"location"`
}
type chkSum struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
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
		zr, err := zstd.NewReader(r)
		if err != nil {
			return nil, fmt.Errorf("解压 %s 失败: %w", name, err)
		}
		return zr.IOReadCloser(), nil
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
	if ct := strings.ToLower(p.Checksum.Type); ct != "" {
		pkg.ChecksumType = ct
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
