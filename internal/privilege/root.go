package privilege

import (
	"fmt"
	"os"
)

// RequireRoot rejects system-changing commands for non-root users.
func RequireRoot(reason, example string) error {
	if os.Geteuid() == 0 {
		return nil
	}
	return fmt.Errorf(`当前命令需要 root 权限

原因：%s

请使用：
  %s`, reason, example)
}
