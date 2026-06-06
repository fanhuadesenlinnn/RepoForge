package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
)

func TestReadOSAndBackend(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "ID=ubuntu\nID_LIKE=debian\nVERSION_ID=\"24.04\"\nPRETTY_NAME=\"Ubuntu 24.04\"\n"
	if err := os.WriteFile(filepath.Join(root, "etc", "os-release"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	system, err := ReadOS(root)
	if err != nil {
		t.Fatal(err)
	}
	if system.ID != "ubuntu" || system.Backend(root) != "deb" {
		t.Fatalf("unexpected OS: %#v", system)
	}
}

func TestNormalizeArch(t *testing.T) {
	_, rpm, deb, err := NormalizeArch("arm64")
	if err != nil {
		t.Fatal(err)
	}
	if rpm != "aarch64" || deb != "arm64" {
		t.Fatalf("NormalizeArch() = %s, %s", rpm, deb)
	}
}

func TestCheckCompatibility(t *testing.T) {
	system := System{
		OS:      OS{ID: "ubuntu", IDLike: []string{"debian"}, VersionID: "24.04"},
		RPMArch: "aarch64",
		DEBArch: "arm64",
		Backend: "deb",
	}
	profile := &config.ProfileConfig{
		Backend: "deb",
		Target: config.TargetConfig{
			OS:      "debian",
			Version: "24.04",
			Arch:    "arm64",
		},
		Compatibility: config.CompatibilityConfig{
			RequireSameOS:      true,
			RequireSameVersion: true,
			RequireSameArch:    true,
		},
	}
	if err := CheckCompatibility(system, profile); err != nil {
		t.Fatal(err)
	}
	profile.Target.Arch = "amd64"
	if err := CheckCompatibility(system, profile); err == nil {
		t.Fatal("CheckCompatibility() error = nil, want mismatch")
	}
}
