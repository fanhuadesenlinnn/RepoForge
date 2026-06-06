package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fanhuadesenlinnn/RepoForge/internal/initialize"
)

func TestLoadExpandsHome(t *testing.T) {
	home := t.TempDir()
	if err := initialize.Run(home, false); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Paths.RepoDir != filepath.Join(home, "repos") {
		t.Fatalf("RepoDir = %q", cfg.Paths.RepoDir)
	}
	profile, err := LoadProfile(home, "debian-12-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Repository.PackageDir != filepath.Join(home, "repos", "debian-12-amd64") {
		t.Fatalf("PackageDir = %q", profile.Repository.PackageDir)
	}
}

func TestInitializePreservesAndForceOverwritesConfig(t *testing.T) {
	home := t.TempDir()
	if err := initialize.Run(home, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "config", "packages.yaml")
	if err := os.WriteFile(path, []byte("custom: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := initialize.Run(home, false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "custom: true\n" {
		t.Fatal("init without force overwrote user configuration")
	}
	if err := initialize.Run(home, true); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if string(data) == "custom: true\n" {
		t.Fatal("init --force did not overwrite managed configuration")
	}
}
