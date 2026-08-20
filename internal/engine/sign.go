package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/fanhuadesenlinnn/RepoForge/internal/progress"
	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/fanhuadesenlinnn/RepoForge/internal/sign"
)

// signRepodata signs the just-generated repository metadata when signing is
// enabled and a private key is available:
//   - RPM: writes repodata/repomd.xml.asc (ASCII detached signature) — the
//     file yum's repo_gpgcheck=1 verifies.
//   - DEB: writes Release, InRelease (clearsigned) and Release.gpg (detached)
//     in the repository root — apt's Signed-By mechanism.
//
// A missing key is a warning, not an error: an unsigned repo still works with
// trusted=yes / gpgcheck=0 clients, and the user may enable signing later.
func signRepodata(ctx context.Context, cfg *repo.Config, root, backend string) error {
	if !cfg.Signing.Enabled {
		return nil
	}
	keyPath := cfg.Signing.PrivateKey
	if keyPath == "" {
		keyPath = filepath.Join(cfg.Paths.HomeDir, "config", "signing", "private.key")
	}
	if _, err := os.Stat(keyPath); err != nil {
		progress.Warnf(ctx, "[签名] 未找到私钥 %s，跳过签名（可运行 repoforge gpg init 生成）", keyPath)
		return nil
	}
	signer, err := sign.LoadPrivateKey(keyPath)
	if err != nil {
		return fmt.Errorf("加载签名私钥失败: %w", err)
	}
	progress.Infof(ctx, "[签名] 使用密钥 %s 签名元数据", sign.Fingerprint(signer))
	if backend == "deb" {
		return signDEBRelease(ctx, signer, root)
	}
	return signRPMRepomd(signer, root)
}

// signRPMRepomd writes repodata/repomd.xml.asc next to the generated repomd.
func signRPMRepomd(signer *openpgp.Entity, root string) error {
	data, err := os.ReadFile(filepath.Join(root, "repodata", "repomd.xml"))
	if err != nil {
		return fmt.Errorf("读取 repomd.xml 以签名: %w", err)
	}
	asc, err := sign.DetachSignASCII(signer, data)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "repodata", "repomd.xml.asc"), asc, 0o644); err != nil {
		return err
	}
	return nil
}

// signDEBRelease generates a flat Release file (SHA256 of Packages) plus
// InRelease (clearsigned) and Release.gpg (detached) for apt Signed-By use.
func signDEBRelease(ctx context.Context, signer *openpgp.Entity, root string) error {
	release, err := buildFlatRelease(root)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "Release"), release, 0o644); err != nil {
		return err
	}
	in, err := sign.ClearSign(signer, release)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "InRelease"), in, 0o644); err != nil {
		return err
	}
	gpg, err := sign.DetachSignASCII(signer, release)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "Release.gpg"), gpg, 0o644); err != nil {
		return err
	}
	progress.Infof(ctx, "[签名] 已生成 Release / InRelease / Release.gpg")
	return nil
}

// buildFlatRelease writes the Release stanza for a flat (non-dists) DEB repo:
// it lists every Packages file found under root with its SHA256 and size.
func buildFlatRelease(root string) ([]byte, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Origin: RepoForge\n")
	fmt.Fprintf(&b, "Label: RepoForge\n")
	fmt.Fprintf(&b, "Suite: repoforge\n")
	fmt.Fprintf(&b, "Codename: repoforge\n")
	fmt.Fprintf(&b, "Date: %s\n", time.Now().UTC().Format(time.RFC1123))
	fmt.Fprintf(&b, "Architectures: all\n")
	fmt.Fprintf(&b, "Components: main\n")
	fmt.Fprintf(&b, "Description: RepoForge offline repository\n")
	fmt.Fprintf(&b, "SHA256:\n")

	var packageFiles []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == "Packages" || name == "Packages.gz" || name == "Packages.xz" || name == "Packages.zst" {
			packageFiles = append(packageFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, p := range packageFiles {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		rel, err := filepath.Rel(root, p)
		if err != nil {
			rel = p
		}
		fmt.Fprintf(&b, " %s %d %s\n", hex.EncodeToString(sum[:]), len(data), filepath.ToSlash(rel))
	}
	return []byte(b.String()), nil
}
