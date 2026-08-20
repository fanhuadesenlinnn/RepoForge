// Package upstream parses both upstream repository metadata and local package
// files. ParseLocalPackage reads the metadata of a local rpm/deb file so that
// packages from input.package_dirs can be published into the offline repo with
// their real dependencies, and those dependencies can be resolved.
package upstream

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/cavaliergopher/rpm"
	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// ParseLocalPackage reads package metadata from a local rpm/deb file.
// location is the path recorded in the generated repodata (relative to the
// repo root, e.g. "Packages/vim.rpm"). The returned Pkg has Local set.
func ParseLocalPackage(path, location, backend string) (*Pkg, error) {
	switch backend {
	case "rpm":
		return parseLocalRPM(path, location)
	case "deb":
		return parseLocalDEB(path, location)
	default:
		return nil, fmt.Errorf("不支持的 backend %q", backend)
	}
}

func parseLocalRPM(path, location string) (*Pkg, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	p, err := rpm.Read(f)
	if err != nil {
		return nil, fmt.Errorf("解析本地 rpm %s 失败: %w", path, err)
	}
	return &Pkg{
		Name:       p.Name(),
		Epoch:      strconv.Itoa(p.Epoch()),
		Version:    p.Version(),
		Release:    p.Release(),
		Arch:       p.Architecture(),
		Location:   location,
		Size:       st.Size(),
		Summary:    p.Summary(),
		Requires:   rpmDeps(p.Requires()),
		Recommends: rpmDeps(p.Recommends()),
		Provides:   rpmProvides(p.Provides()),
		Local:      true,
	}, nil
}

// rpmDeps converts rpm dependencies into DependencyEntry list. rpmlib(...)
// and rtld(...) markers are rpm-internal virtual capabilities that yum ignores
// (createrepo filters them from repodata); local file headers still carry
// them, so drop them here.
func rpmDeps(deps []rpm.Dependency) []DependencyEntry {
	var out []DependencyEntry
	for _, d := range deps {
		name := d.Name()
		if strings.HasPrefix(name, "rpmlib(") || strings.HasPrefix(name, "rtld(") {
			continue
		}
		e := DependencyEntry{Name: name}
		if op := rpmOp(d.Flags()); op != "" {
			e.Op = op
			e.Version = d.Version()
		}
		out = append(out, e)
	}
	return out
}

func rpmOp(flags int) string {
	const mask = rpm.DepFlagLesser | rpm.DepFlagGreater | rpm.DepFlagEqual
	switch flags & mask {
	case rpm.DepFlagEqual:
		return "="
	case rpm.DepFlagGreaterOrEqual:
		return ">="
	case rpm.DepFlagLesserOrEqual:
		return "<="
	case rpm.DepFlagGreater:
		return ">"
	case rpm.DepFlagLesser:
		return "<"
	default:
		return ""
	}
}

// rpmProvides renders provides as names (optionally with version markers kept
// the same way primary.xml parsing does).
func rpmProvides(deps []rpm.Dependency) []string {
	var out []string
	for _, d := range deps {
		name := d.Name()
		if op := rpmOp(d.Flags()); op != "" && d.Version() != "" {
			name = fmt.Sprintf("%s %s %s", name, op, d.Version())
		}
		out = append(out, name)
	}
	return out
}

// ---- DEB (ar archive + control file) ----

type arMember struct {
	name string
	data []byte
}

// readArMembers parses a Unix ar archive (the container format of .deb files).
func readArMembers(r io.Reader) ([]arMember, error) {
	var magic [8]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return nil, err
	}
	if string(magic[:]) != "!<arch>\n" {
		return nil, fmt.Errorf("不是 ar 归档")
	}
	var members []arMember
	for {
		var hdr [60]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		name := strings.TrimRight(string(hdr[0:16]), " ")
		sizeStr := strings.TrimRight(string(hdr[48:58]), " ")
		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("ar 成员 %q 大小非法: %v", name, err)
		}
		data := make([]byte, size)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}
		if size%2 == 1 { // members are padded to even sizes
			var pad [1]byte
			if _, err := io.ReadFull(r, pad[:]); err != nil {
				return nil, err
			}
		}
		// GNU ar writes a trailing "/" on some member names; strip it.
		name = strings.TrimSuffix(name, "/")
		members = append(members, arMember{name: name, data: data})
	}
	return members, nil
}

// decompressControl extracts the control.tar member of a deb and returns the
// parsed control file contents.
func decompressControl(members []arMember) ([]byte, error) {
	for _, m := range members {
		if !strings.HasPrefix(m.name, "control.tar.") {
			continue
		}
		var rc io.ReadCloser
		switch {
		case strings.HasSuffix(m.name, ".gz"):
			gr, err := gzip.NewReader(bytes.NewReader(m.data))
			if err != nil {
				return nil, err
			}
			rc = gr
		case strings.HasSuffix(m.name, ".xz"):
			xr, err := xz.NewReader(bytes.NewReader(m.data))
			if err != nil {
				return nil, err
			}
			rc = io.NopCloser(xr)
		case strings.HasSuffix(m.name, ".zst"):
			zr, err := zstd.NewReader(bytes.NewReader(m.data))
			if err != nil {
				return nil, err
			}
			rc = zr.IOReadCloser()
		default:
			return nil, fmt.Errorf("不支持的 deb 控制文件压缩格式 %s", m.name)
		}
		tr := tar.NewReader(rc)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				rc.Close()
				return nil, err
			}
			name := strings.TrimPrefix(hdr.Name, "./")
			if name == "control" {
				data, err := io.ReadAll(tr)
				rc.Close()
				return data, err
			}
		}
		rc.Close()
	}
	return nil, fmt.Errorf("deb 中未找到 control.tar 成员")
}

// parseControlFields parses a Debian control file into a key/value map.
func parseControlFields(ctrl []byte) map[string]string {
	fields := map[string]string{}
	var curKey string
	for _, raw := range strings.Split(string(ctrl), "\n") {
		line := strings.TrimRight(raw, "\r")
		if line == "" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if curKey != "" {
				fields[curKey] += " " + strings.TrimSpace(line)
			}
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		curKey = line[:idx]
		fields[curKey] = strings.TrimSpace(line[idx+1:])
	}
	return fields
}

func parseLocalDEB(path, location string) (*Pkg, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	members, err := readArMembers(f)
	if err != nil {
		return nil, fmt.Errorf("解析本地 deb %s 失败: %w", path, err)
	}
	ctrl, err := decompressControl(members)
	if err != nil {
		return nil, fmt.Errorf("解析本地 deb %s 失败: %w", path, err)
	}
	fields := parseControlFields(ctrl)
	pkg := Pkg{
		Name:       fields["Package"],
		Version:    fields["Version"],
		Arch:       fields["Architecture"],
		Location:   location,
		Size:       st.Size(),
		Summary:    fields["Description"],
		MultiArch:  fields["Multi-Arch"],
		Requires:   parseDEBDeps(fields["Depends"]),
		Recommends: parseDEBDeps(fields["Recommends"]),
		Provides:   parseDEBProvides(fields["Provides"]),
		Local:      true,
	}
	if idx := strings.Index(pkg.Version, ":"); idx >= 0 {
		pkg.Epoch = pkg.Version[:idx]
		pkg.Version = pkg.Version[idx+1:]
	}
	if pkg.Name == "" {
		return nil, fmt.Errorf("本地 deb %s 缺少 Package 字段", path)
	}
	return &pkg, nil
}
