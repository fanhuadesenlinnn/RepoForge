package home

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectorUsesEnvironment(t *testing.T) {
	detector := Detector{
		Executable: func() (string, error) { return "/ignored/repoforge", nil },
		Getwd:      os.Getwd,
		Getenv: func(key string) string {
			if key == "REPOFORGE_HOME" {
				return "./example"
			}
			return ""
		},
	}
	got, err := detector.Detect(false)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs("./example")
	if got != want {
		t.Fatalf("Detect() = %q, want %q", got, want)
	}
}

func TestDetectorFindsBinLayout(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "bin", "repoforge")
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	detector := Detector{
		Executable: func() (string, error) { return binary, nil },
		Getwd:      os.Getwd,
		Getenv:     func(string) string { return "" },
	}
	got, err := detector.Detect(false)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Detect() = %q, want %q", got, want)
	}
}

func TestDetectorFindsMarkerFromWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "one", "two")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, markerName), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "repoforge-dev")
	if err := os.WriteFile(executable, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	detector := Detector{
		Executable: func() (string, error) { return executable, nil },
		Getwd:      func() (string, error) { return nested, nil },
		Getenv:     func(string) string { return "" },
	}
	got, err := detector.Detect(false)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Clean(root)
	if got != want {
		t.Fatalf("Detect() = %q, want %q", got, want)
	}
}
