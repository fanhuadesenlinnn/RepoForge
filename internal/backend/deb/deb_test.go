package deb

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
	"github.com/fanhuadesenlinnn/RepoForge/internal/initialize"
)

type fakeRunner struct {
	commands []executor.Command
}

func (f *fakeRunner) LookPath(name string) (string, error) {
	return "/usr/bin/" + name, nil
}

func (f *fakeRunner) Run(_ context.Context, command executor.Command) (executor.Result, error) {
	f.commands = append(f.commands, command)
	switch command.Name {
	case "dpkg-scanpackages":
		return executor.Result{Stdout: "Package: vim\nFilename: ./vim.deb\n"}, nil
	case "gzip":
		return executor.Result{Stdout: "compressed-index"}, nil
	default:
		return executor.Result{}, nil
	}
}

func TestMakeCreatesDEBIndexes(t *testing.T) {
	cfg, profile := initializedDEB(t)
	runner := &fakeRunner{}
	if err := New(runner).Make(context.Background(), cfg, profile, []string{"vim", "curl"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Packages", "Packages.gz"} {
		if _, err := os.Stat(filepath.Join(profile.Repository.PackageDir, name)); err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
	}
	if len(runner.commands) != 4 {
		t.Fatalf("command count = %d, want 4", len(runner.commands))
	}
	install := strings.Join(runner.commands[1].Args, " ")
	if !strings.Contains(install, "--download-only") || !strings.Contains(install, "vim curl") {
		t.Fatalf("unexpected apt install command: %s", install)
	}
}

func TestEnableDisableAndRemoveLocalRepo(t *testing.T) {
	cfg, profile := initializedDEB(t)
	if err := os.MkdirAll(profile.Repository.PackageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile.Repository.PackageDir, "Packages.gz"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	profile.LocalRepo.RepoFile = filepath.Join(t.TempDir(), "repoforge-local.list")
	profile.LocalRepo.UpdateAfterEnable = false
	backend := New(&fakeRunner{})
	if err := backend.EnableLocalRepo(context.Background(), cfg, profile); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(profile.LocalRepo.RepoFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "trusted=yes") || !strings.Contains(string(data), "file:") {
		t.Fatalf("unexpected list file: %s", data)
	}
	if err := backend.DisableLocalRepo(context.Background(), cfg, profile, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(profile.LocalRepo.RepoFile + ".disabled"); err != nil {
		t.Fatal(err)
	}
	if err := backend.DisableLocalRepo(context.Background(), cfg, profile, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(profile.LocalRepo.RepoFile + ".disabled"); !os.IsNotExist(err) {
		t.Fatalf("disabled file still exists: %v", err)
	}
}

func initializedDEB(t *testing.T) (*config.Config, *config.ProfileConfig) {
	t.Helper()
	home := t.TempDir()
	if err := initialize.Run(home, false); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := config.LoadProfile(home, "debian-12-amd64")
	if err != nil {
		t.Fatal(err)
	}
	return cfg, profile
}
