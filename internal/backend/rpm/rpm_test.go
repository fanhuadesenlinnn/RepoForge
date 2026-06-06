package rpm

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
	onRun    func(executor.Command) error
}

func (f *fakeRunner) LookPath(name string) (string, error) {
	return "/usr/bin/" + name, nil
}

func (f *fakeRunner) Run(_ context.Context, command executor.Command) (executor.Result, error) {
	f.commands = append(f.commands, command)
	if f.onRun != nil {
		if err := f.onRun(command); err != nil {
			return executor.Result{}, err
		}
	}
	return executor.Result{}, nil
}

func TestMakeRunsDownloadAndMetadataCommands(t *testing.T) {
	cfg, profile := initializedRPM(t)
	runner := &fakeRunner{
		onRun: func(command executor.Command) error {
			if command.Name == "createrepo_c" {
				index := filepath.Join(profile.Repository.PackageDir, "repodata", "repomd.xml")
				if err := os.MkdirAll(filepath.Dir(index), 0o755); err != nil {
					return err
				}
				return os.WriteFile(index, nil, 0o644)
			}
			return nil
		},
	}
	if err := New(runner).Make(context.Background(), cfg, profile, []string{"vim", "curl"}); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("command count = %d, want 2", len(runner.commands))
	}
	download := strings.Join(runner.commands[0].Args, " ")
	if !strings.Contains(download, "--downloadonly") || !strings.Contains(download, "vim curl") {
		t.Fatalf("unexpected download command: %s", download)
	}
}

func TestEnableAndDisableLocalRepo(t *testing.T) {
	cfg, profile := initializedRPM(t)
	index := filepath.Join(profile.Repository.PackageDir, "repodata", "repomd.xml")
	if err := os.MkdirAll(filepath.Dir(index), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(index, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	profile.LocalRepo.RepoFile = filepath.Join(t.TempDir(), "repoforge-local.repo")
	profile.LocalRepo.MakecacheAfterEnable = false
	backend := New(&fakeRunner{})
	if err := backend.EnableLocalRepo(context.Background(), cfg, profile); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(profile.LocalRepo.RepoFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "enabled=1") || !strings.Contains(string(data), "file://") {
		t.Fatalf("unexpected repo file: %s", data)
	}
	if err := backend.DisableLocalRepo(context.Background(), cfg, profile, false); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(profile.LocalRepo.RepoFile)
	if !strings.Contains(string(data), "enabled=0") {
		t.Fatalf("repo was not disabled: %s", data)
	}
}

func initializedRPM(t *testing.T) (*config.Config, *config.ProfileConfig) {
	t.Helper()
	home := t.TempDir()
	if err := initialize.Run(home, false); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := config.LoadProfile(home, "kylin-v10-sp3-x86_64")
	if err != nil {
		t.Fatal(err)
	}
	return cfg, profile
}
