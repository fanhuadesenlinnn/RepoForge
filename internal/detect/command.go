package detect

import (
	"fmt"

	"github.com/fanhuadesenlinnn/RepoForge/internal/executor"
)

// FindAny returns the first available command.
func FindAny(runner executor.Runner, names ...string) (string, error) {
	for _, name := range names {
		if _, err := runner.LookPath(name); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("未找到可用命令：%v", names)
}
