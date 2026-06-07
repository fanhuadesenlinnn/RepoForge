package rpm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgbackend "github.com/fanhuadesenlinnn/RepoForge/internal/backend"
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

func TestCollectPackageFilesEmptyDirs(t *testing.T) {
	files, err := pkgbackend.CollectPackageFiles(nil, ".rpm", false)
	if err != nil || files != nil {
		t.Fatalf("unexpected result for nil dirs: %v, %v", files, err)
	}
	files, err = pkgbackend.CollectPackageFiles([]string{}, ".rpm", true)
	if err != nil || files != nil {
		t.Fatalf("unexpected result for empty dirs: %v, %v", files, err)
	}
}

func TestCollectPackageFilesDirNotFound(t *testing.T) {
	_, err := pkgbackend.CollectPackageFiles([]string{"/nonexistent/path/for/test"}, ".rpm", false)
	if err == nil {
		t.Fatal("expected error for missing dir")
	}
}

func TestCollectPackageFilesNonRecursive(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "example.rpm"), []byte("rpm"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("txt"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "subdir", "nested.rpm"), []byte("nested"), 0o644)

	files, err := pkgbackend.CollectPackageFiles([]string{dir}, ".rpm", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !strings.HasSuffix(files[0], "example.rpm") {
		t.Fatalf("expected 1 file (example.rpm), got: %v", files)
	}
}

func TestCollectPackageFilesRecursive(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "example.rpm"), []byte("rpm"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "subdir", "nested.rpm"), []byte("nested"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "subdir", "readme.txt"), []byte("txt"), 0o644)

	files, err := pkgbackend.CollectPackageFiles([]string{dir}, ".rpm", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
}

func TestCollectPackageFilesNoMatchFound(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("txt"), 0o644)

	_, err := pkgbackend.CollectPackageFiles([]string{dir}, ".rpm", true)
	if err == nil {
		t.Fatal("expected error when no matching files found")
	}
}

func TestCopyPackagesToRepoCopyNewFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	src := filepath.Join(srcDir, "example.rpm")
	if err := os.WriteFile(src, []byte("rpm content"), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := pkgbackend.CopyPackagesToRepo([]string{src}, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || filepath.Base(results[0]) != "example.rpm" {
		t.Fatalf("unexpected results: %v", results)
	}
	data, err := os.ReadFile(results[0])
	if err != nil || string(data) != "rpm content" {
		t.Fatalf("copy failed: %v", err)
	}
}

func TestCopyPackagesToRepoSkipSameSize(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	src := filepath.Join(srcDir, "example.rpm")
	if err := os.WriteFile(src, []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dstDir, "example.rpm")
	if err := os.WriteFile(dst, []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := pkgbackend.CopyPackagesToRepo([]string{src}, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got: %v", results)
	}
	// File should not have been overwritten (same size skip).
	data, _ := os.ReadFile(dst)
	if string(data) != "same" {
		t.Fatal("file was unexpectedly overwritten")
	}
}

func TestCopyPackagesToRepoOverwriteDifferentSize(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	src := filepath.Join(srcDir, "example.rpm")
	if err := os.WriteFile(src, []byte("newer content"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dstDir, "example.rpm")
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := pkgbackend.CopyPackagesToRepo([]string{src}, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got: %v", results)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "newer content" {
		t.Fatalf("file not overwritten, got: %s", data)
	}
}
