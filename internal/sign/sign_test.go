package sign

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
)

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}

func mustExportPub(entity *openpgp.Entity) []byte {
	var buf bytes.Buffer
	w, err := armor.Encode(&buf, openpgp.PublicKeyType, nil)
	if err != nil {
		panic(err)
	}
	if err := entity.Serialize(w); err != nil {
		panic(err)
	}
	_ = w.Close()
	return buf.Bytes()
}

func TestGenerateAndLoadPrivateKey(t *testing.T) {
	priv, pub, fp, err := Generate("Test Key", "test@example.com")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(string(priv), "BEGIN PGP PRIVATE KEY BLOCK") {
		t.Fatalf("private key missing armor header")
	}
	if !strings.Contains(string(pub), "BEGIN PGP PUBLIC KEY BLOCK") {
		t.Fatalf("public key missing armor header")
	}
	if len(fp) < 40 {
		t.Fatalf("unexpected fingerprint %q", fp)
	}

	dir := t.TempDir()
	path := dir + "/private.key"
	if err := writeFile(path, priv); err != nil {
		t.Fatal(err)
	}
	entity, err := LoadPrivateKey(path)
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}
	if entity.PrivateKey == nil {
		t.Fatalf("loaded entity has no private key")
	}
	if Fingerprint(entity) != fp {
		t.Fatalf("fingerprint mismatch: %s != %s", Fingerprint(entity), fp)
	}
}

func TestDetachSignASCIIAndVerify(t *testing.T) {
	priv, _, _, err := Generate("", "")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := dir + "/private.key"
	if err := writeFile(path, priv); err != nil {
		t.Fatal(err)
	}
	entity, err := LoadPrivateKey(path)
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("hello repoforge")
	asc, err := DetachSignASCII(entity, data)
	if err != nil {
		t.Fatalf("DetachSignASCII: %v", err)
	}
	if !strings.Contains(string(asc), "BEGIN PGP SIGNATURE") {
		t.Fatalf("signature missing armor header")
	}

	// Verify the detached signature against the original data.
	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(mustExportPub(entity)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openpgp.CheckArmoredDetachedSignature(keyring, bytes.NewReader(data), bytes.NewReader(asc), nil); err != nil {
		t.Fatalf("signature verification failed: %v", err)
	}
	// Tampered data must fail.
	if _, err := openpgp.CheckArmoredDetachedSignature(keyring, bytes.NewReader([]byte("tampered")), bytes.NewReader(asc), nil); err == nil {
		t.Fatalf("tampered data verified — expected failure")
	}
}

func TestClearSignContainsBodyAndSignature(t *testing.T) {
	priv, _, _, err := Generate("", "")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := dir + "/private.key"
	if err := writeFile(path, priv); err != nil {
		t.Fatal(err)
	}
	entity, err := LoadPrivateKey(path)
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("Origin: RepoForge\nLabel: RepoForge\n")
	out, err := ClearSign(entity, data)
	if err != nil {
		t.Fatalf("ClearSign: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "BEGIN PGP SIGNED MESSAGE") {
		t.Fatalf("missing signed-message header")
	}
	if !strings.Contains(s, "BEGIN PGP SIGNATURE") {
		t.Fatalf("missing signature block")
	}
	if !strings.Contains(s, "Origin: RepoForge") {
		t.Fatalf("clearsigned output missing body")
	}
	// Body must appear before the signature block.
	if strings.Index(s, "Origin: RepoForge") > strings.Index(s, "BEGIN PGP SIGNATURE") {
		t.Fatalf("body must precede signature")
	}
}
