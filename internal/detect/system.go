package detect

import (
	"context"
	"fmt"
	"strings"
	"time"

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
