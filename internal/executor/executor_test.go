package executor

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunCapturesOutputAndExitCode(t *testing.T) {
	runner := New(false)
	result, err := runner.Run(context.Background(), Command{
		Name: "sh",
		Args: []string{"-c", "printf output; printf failure >&2; exit 7"},
	})
	if err == nil {
		t.Fatal("Run() error = nil, want failure")
	}
	if result.Stdout != "output" || result.Stderr != "failure" || result.ExitCode != 7 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !strings.Contains(err.Error(), "退出码：7") {
		t.Fatalf("error lacks exit code: %v", err)
	}
}

func TestRunDryRunDoesNotExecute(t *testing.T) {
	result, err := New(true).Run(context.Background(), Command{Name: "missing-command"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun {
		t.Fatal("DryRun = false, want true")
	}
}

func TestRunTimeout(t *testing.T) {
	_, err := New(false).Run(context.Background(), Command{
		Name:    "sh",
		Args:    []string{"-c", "sleep 1"},
		Timeout: 10 * time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("Run() error = %v, want timeout", err)
	}
}
