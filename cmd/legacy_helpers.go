package cmd

// Shared helpers for the legacy (dnf/apt profile) command path. Kept so the
// legacy list / check / make-upgrade commands still compile while the new
// engine-based make replaces the old dnf make.

import (
	"context"
	"fmt"
	"strings"

	"github.com/fanhuadesenlinnn/RepoForge/internal/backend"
	"github.com/fanhuadesenlinnn/RepoForge/internal/backend/deb"
	"github.com/fanhuadesenlinnn/RepoForge/internal/backend/rpm"
	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/detect"
	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
	"github.com/fanhuadesenlinnn/RepoForge/internal/home"
)

func loadMakeInputs(ctx context.Context, requestedProfile string) (string, *config.Config, *config.ProfileConfig, []string, executor.Runner, error) {
	homeDir, cfg, profile, runner, err := loadProfileInputs(ctx, requestedProfile)
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	packages, err := config.PackagesForProfile(homeDir, profile.Profile)
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	return homeDir, cfg, profile, packages, runner, nil
}

func loadProfileInputs(ctx context.Context, requestedProfile string) (string, *config.Config, *config.ProfileConfig, executor.Runner, error) {
	homeDir, err := home.Detect(false)
	if err != nil {
		return "", nil, nil, nil, err
	}
	cfg, err := config.Load(homeDir)
	if err != nil {
		return "", nil, nil, nil, err
	}
	runner := executor.New(false)
	system, err := detect.Current(ctx, runner)
	if err != nil {
		return "", nil, nil, nil, err
	}

	profileName, err := resolveProfile(homeDir, requestedProfile, system)
	if err != nil {
		return "", nil, nil, nil, err
	}

	profile, err := config.LoadProfile(homeDir, profileName)
	if err != nil {
		return "", nil, nil, nil, err
	}
	if err := detect.CheckCompatibility(system, profile); err != nil {
		return "", nil, nil, nil, err
	}
	return homeDir, cfg, profile, runner, nil
}

// resolveProfile resolves the profile to use: explicit > auto-detect > error.
func resolveProfile(homeDir, requestedProfile string, system detect.System) (string, error) {
	if requestedProfile != "" {
		return requestedProfile, nil
	}
	matches, err := config.FindMatchingProfiles(homeDir, system.ID, system.RawArch, system.Backend)
	if err != nil {
		return "", err
	}
	if len(matches) == 1 {
		fmt.Printf("自动匹配 profile: %s\n\n", matches[0].Profile)
		return matches[0].Profile, nil
	}
	if len(matches) > 1 {
		names := make([]string, len(matches))
		for i, p := range matches {
			names[i] = p.Profile
		}
		return "", fmt.Errorf("检测到多个匹配的 profile，请用 --profile 指定:\n  %s", strings.Join(names, "\n  "))
	}
	all, err := config.LoadProfiles(homeDir)
	if err != nil {
		return "", err
	}
	names := make([]string, len(all))
	for i, p := range all {
		names[i] = p.Profile
	}
	return "", fmt.Errorf("未找到与当前系统匹配的 profile，请用 --profile 指定。\n可用 profile:\n  %s", strings.Join(names, "\n  "))
}

func selectBackend(name string, runner executor.Runner) (backend.Backend, error) {
	switch name {
	case "rpm":
		return rpm.New(runner), nil
	case "deb":
		return deb.New(runner), nil
	default:
		return nil, fmt.Errorf("不支持的 backend: %s", name)
	}
}
