// Package sign provides OpenPGP (GPG) signing for generated repository
// metadata: repomd.xml.asc for yum, and Release/InRelease/Release.gpg for apt.
// It is pure Go (no gpg binary required) and generates Ed25519 keys, which
// are fast and produce small signatures.
package sign

import (
	"bytes"
	"crypto"
	"fmt"
	"os"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

// DefaultKeyName/Email are the identities used by `repoforge gpg init` unless
// overridden on the command line.
const (
	DefaultKeyName  = "RepoForge"
	DefaultKeyEmail = "repoforge@localhost"
)

// Generate creates a new Ed25519 OpenPGP key pair. It returns the armored
// private and public key blocks.
func Generate(name, email string) (priv, pub []byte, fingerprint string, err error) {
	if name == "" {
		name = DefaultKeyName
	}
	if email == "" {
		email = DefaultKeyEmail
	}
	cfg := &packet.Config{
		Algorithm:     packet.PubKeyAlgoEdDSA,
		DefaultHash:   crypto.SHA256,
		DefaultCipher: packet.CipherAES256,
		Time:          func() time.Time { return time.Now() },
	}
	entity, err := openpgp.NewEntity(name, "", email, cfg)
	if err != nil {
		return nil, nil, "", fmt.Errorf("生成 OpenPGP 密钥失败: %w", err)
	}
	fingerprint = fmt.Sprintf("%X", entity.PrimaryKey.Fingerprint)

	var pb, pubBuf bytes.Buffer
	{
		w, werr := armor.Encode(&pb, openpgp.PrivateKeyType, nil)
		if werr != nil {
			return nil, nil, "", werr
		}
		if serr := entity.SerializePrivate(w, nil); serr != nil {
			return nil, nil, "", serr
		}
		if cerr := w.Close(); cerr != nil {
			return nil, nil, "", cerr
		}
	}
	{
		w, werr := armor.Encode(&pubBuf, openpgp.PublicKeyType, nil)
		if werr != nil {
			return nil, nil, "", werr
		}
		if serr := entity.Serialize(w); serr != nil {
			return nil, nil, "", serr
		}
		if cerr := w.Close(); cerr != nil {
			return nil, nil, "", cerr
		}
	}
	return pb.Bytes(), pubBuf.Bytes(), fingerprint, nil
}

// LoadPrivateKey reads an ASCII-armored private key file.
func LoadPrivateKey(path string) (*openpgp.Entity, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取私钥 %s: %w", path, err)
	}
	block, err := armor.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("解析私钥 %s（需要 ASCII armored PGP 私钥）: %w", path, err)
	}
	el, err := openpgp.ReadKeyRing(block.Body)
	if err != nil {
		return nil, fmt.Errorf("解析私钥 %s: %w", path, err)
	}
	for _, e := range el {
		if e.PrivateKey != nil {
			return e, nil
		}
	}
	return nil, fmt.Errorf("私钥 %s 中没有可用的私钥实体", path)
}

// DetachSignASCII produces an ASCII-armored detached signature of data.
func DetachSignASCII(signer *openpgp.Entity, data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, openpgp.SignatureType, nil)
	if err != nil {
		return nil, err
	}
	if err := openpgp.DetachSign(w, signer, bytes.NewReader(data), &packet.Config{DefaultHash: crypto.SHA256}); err != nil {
		return nil, fmt.Errorf("签名失败: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ClearSign produces a clearsigned document (the body followed by an inline
// signature) — the format apt expects for InRelease. The body is signed in
// text mode (canonical line endings), matching what gpgv verifies.
func ClearSign(signer *openpgp.Entity, data []byte) ([]byte, error) {
	body := append(append([]byte{}, data...), '\n') // signature covers trailing newline
	var sig bytes.Buffer
	if err := openpgp.ArmoredDetachSignText(&sig, signer, bytes.NewReader(body), &packet.Config{DefaultHash: crypto.SHA256}); err != nil {
		return nil, fmt.Errorf("签名失败: %w", err)
	}
	var buf bytes.Buffer
	buf.WriteString("-----BEGIN PGP SIGNED MESSAGE-----\n")
	buf.WriteString("Hash: SHA256\n\n")
	buf.Write(body)
	buf.WriteString("\n")
	buf.Write(sig.Bytes())
	buf.WriteString("\n")
	return buf.Bytes(), nil
}

// Fingerprint returns the primary key fingerprint of an entity.
func Fingerprint(entity *openpgp.Entity) string {
	if entity == nil || entity.PrimaryKey == nil {
		return ""
	}
	return fmt.Sprintf("%X", entity.PrimaryKey.Fingerprint)
}
