package home

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const markerName = ".repoforge-home"

// Detector contains injectable process lookups used by Detect and tests.
type Detector struct {
	Executable func() (string, error)
	Getwd      func() (string, error)
	Getenv     func(string) string
}

// Detect finds the RepoForge home for the current process.
func Detect(forInit bool) (string, error) {
	return Detector{
		Executable: os.Executable,
		Getwd:      os.Getwd,
		Getenv:     os.Getenv,
	}.Detect(forInit)
}

// Detect follows the documented home discovery order.
func (d Detector) Detect(forInit bool) (string, error) {
	if value := d.Getenv("REPOFORGE_HOME"); value != "" {
		return cleanAbsolute(value)
	}

	executable, err := d.Executable()
	if err != nil {
		return "", fmt.Errorf("无法获取 repoforge 可执行文件路径: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("无法解析 repoforge 可执行文件路径 %s: %w", executable, err)
	}
	executable, err = cleanAbsolute(executable)
	if err != nil {
		return "", err
	}
	executableDir := filepath.Dir(executable)

	if filepath.Base(executableDir) == "bin" && filepath.Base(executable) == "repoforge" {
		return filepath.Dir(executableDir), nil
	}
	if found := searchUp(executableDir, markerName, false); found != "" {
		return found, nil
	}
	if found := searchUp(executableDir, filepath.Join("config", "repo.yaml"), false); found != "" {
		return found, nil
	}

	workingDir, getwdErr := d.Getwd()
	if getwdErr == nil {
		if found := searchUp(workingDir, markerName, false); found != "" {
			return found, nil
		}
		if found := searchUp(workingDir, filepath.Join("config", "repo.yaml"), false); found != "" {
			return found, nil
		}
	}

	if forInit {
		return filepath.Dir(executableDir), nil
	}
	return "", errors.New("无法识别 RepoForge Home；请在 RepoForge 目录中运行，或设置 REPOFORGE_HOME")
}

func searchUp(start, relative string, wantDir bool) string {
	current, err := cleanAbsolute(start)
	if err != nil {
		return ""
	}
	for {
		info, err := os.Stat(filepath.Join(current, relative))
		if err == nil && info.IsDir() == wantDir {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func cleanAbsolute(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("无法规范化路径 %s: %w", path, err)
	}
	return filepath.Clean(absolute), nil
}
