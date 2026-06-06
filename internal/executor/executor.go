package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Command describes an external command invocation.
type Command struct {
	Name    string
	Args    []string
	Dir     string
	Env     []string
	Timeout time.Duration

	// ProgressOut, when non-nil, receives a copy of stdout in real time.
	// Use this to show download progress etc. while still capturing output.
	ProgressOut io.Writer
	// ProgressErr, when non-nil, receives a copy of stderr in real time.
	ProgressErr io.Writer
}

// Result captures an external command result.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	DryRun   bool
}

// Runner is the external command boundary used throughout RepoForge.
type Runner interface {
	Run(context.Context, Command) (Result, error)
	LookPath(string) (string, error)
}

// OSRunner executes commands on the host operating system.
type OSRunner struct {
	DryRun bool
}

// New returns a host command runner.
func New(dryRun bool) *OSRunner {
	return &OSRunner{DryRun: dryRun}
}

// LookPath finds an executable without running it.
func (r *OSRunner) LookPath(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("未找到 %s 命令", name)
	}
	return path, nil
}

// Run executes one command and wraps failures with Chinese context.
func (r *OSRunner) Run(ctx context.Context, command Command) (Result, error) {
	if command.Name == "" {
		return Result{ExitCode: -1}, errors.New("外部命令名称不能为空")
	}
	if r.DryRun {
		return Result{ExitCode: 0, DryRun: true}, nil
	}

	cancel := func() {}
	if command.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, command.Timeout)
	}
	defer cancel()

	process := exec.CommandContext(ctx, command.Name, command.Args...)
	process.Dir = command.Dir
	if len(command.Env) > 0 {
		process.Env = append(os.Environ(), command.Env...)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if command.ProgressOut != nil {
		process.Stdout = io.MultiWriter(&stdout, command.ProgressOut)
	} else {
		process.Stdout = &stdout
	}
	if command.ProgressErr != nil {
		process.Stderr = io.MultiWriter(&stderr, command.ProgressErr)
	} else {
		process.Stderr = &stderr
	}
	err := process.Run()

	result := Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode(err),
	}
	if err == nil {
		return result, nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return result, fmt.Errorf("执行命令超时：%s", formatCommand(command))
	}
	return result, fmt.Errorf(
		"执行命令失败\n\n命令：%s\n退出码：%d\n错误输出：%s",
		formatCommand(command),
		result.ExitCode,
		strings.TrimSpace(result.Stderr),
	)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return -1
}

func formatCommand(command Command) string {
	return strings.Join(append([]string{command.Name}, command.Args...), " ")
}
