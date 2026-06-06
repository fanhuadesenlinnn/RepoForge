package detect

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
)

// System describes the current Linux operating environment.
type System struct {
	OS
	RawArch string
	RPMArch string
	DEBArch string
	Backend string
}

// Current detects the current Linux system without modifying it.
func Current(ctx context.Context, runner executor.Runner) (System, error) {
	systemOS, err := ReadOS("/")
	if err != nil {
		return System{}, err
	}
	result, err := runner.Run(ctx, executor.Command{
		Name:    "uname",
		Args:    []string{"-m"},
		Timeout: 10 * time.Second,
	})
	if err != nil {
		return System{}, fmt.Errorf("检测当前架构失败: %w", err)
	}
	raw, rpm, deb, err := NormalizeArch(strings.TrimSpace(result.Stdout))
	if err != nil {
		return System{}, err
	}
	return System{
		OS:      systemOS,
		RawArch: raw,
		RPMArch: rpm,
		DEBArch: deb,
		Backend: systemOS.Backend("/"),
	}, nil
}

// NormalizeArch maps common architecture aliases to RPM and DEB names.
func NormalizeArch(value string) (raw, rpm, deb string, err error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "x86_64", "amd64":
		return value, "x86_64", "amd64", nil
	case "aarch64", "arm64":
		return value, "aarch64", "arm64", nil
	default:
		return value, "", "", fmt.Errorf("不支持的系统架构: %s", value)
	}
}

// CheckCompatibility validates the current system against a profile.
func CheckCompatibility(system System, profile *config.ProfileConfig) error {
	if profile.Compatibility.AllowCrossBuild {
		return nil
	}
	target := profile.Target
	if profile.Compatibility.RequireSameOS && !matchesOS(system.OS, target.OS) {
		return compatibilityError(system, profile)
	}
	if profile.Compatibility.RequireSameVersion && !matchesVersion(system.OS, target.Version) {
		return compatibilityError(system, profile)
	}
	currentArch := system.RPMArch
	if profile.Backend == "deb" {
		currentArch = system.DEBArch
	}
	if profile.Compatibility.RequireSameArch && currentArch != target.Arch {
		return compatibilityError(system, profile)
	}
	if system.Backend != "unknown" && profile.Backend != system.Backend {
		return compatibilityError(system, profile)
	}
	return nil
}

func matchesOS(system OS, target string) bool {
	target = strings.ToLower(target)
	return containsAny(append([]string{system.ID}, system.IDLike...), target)
}

func matchesVersion(system OS, target string) bool {
	if target == "" {
		return true
	}
	return strings.EqualFold(system.VersionID, target) ||
		strings.Contains(strings.ToLower(system.KylinRelease), strings.ToLower(target))
}

func compatibilityError(system System, profile *config.ProfileConfig) error {
	currentArch := system.RPMArch
	if profile.Backend == "deb" {
		currentArch = system.DEBArch
	}
	return fmt.Errorf(`当前系统与 profile 目标系统不一致

当前系统：%s %s %s
profile 目标：%s %s %s

解决建议：
1. 请在与目标系统一致的在线机器上制作离线源；
2. 不建议跨发行版制作离线源；
3. 如确认风险，可在 profile 中设置 allow_cross_build: true`,
		system.ID, system.VersionID, currentArch,
		profile.Target.OS, profile.Target.Version, profile.Target.Arch,
	)
}
