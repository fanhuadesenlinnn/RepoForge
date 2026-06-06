package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
	"github.com/fanhuadesenlinnn/RepoForge/internal/home"
	"github.com/spf13/cobra"
)

func newCheckCommand() *cobra.Command {
	var profileName string
	command := &cobra.Command{
		Use:   "check",
		Short: "检查环境和仓库状态",
		RunE: func(command *cobra.Command, _ []string) error {
			homeDir, err := home.Detect(false)
			if err != nil {
				return err
			}
			output := command.OutOrStdout()
			fmt.Fprintf(output, "[OK] RepoForge Home: %s\n", homeDir)

			required := []string{
				filepath.Join(homeDir, "config", "config.yaml"),
				filepath.Join(homeDir, "config", "packages.yaml"),
				filepath.Join(homeDir, "repos"),
				filepath.Join(homeDir, "cache"),
			}
			for _, path := range required {
				if _, err := os.Stat(path); err != nil {
					return fmt.Errorf("必需路径不可用 %s: %w", path, err)
				}
				fmt.Fprintf(output, "[OK] 路径存在: %s\n", path)
			}

			cfg, err := config.Load(homeDir)
			if err != nil {
				return err
			}
			fmt.Fprintf(output, "[OK] 配置文件有效: schema_version=%d\n", cfg.SchemaVersion)

			if profileName != "" {
				profile, err := config.LoadProfile(homeDir, profileName)
				if err != nil {
					return err
				}
				fmt.Fprintf(output, "[OK] profile 配置有效: %s (%s)\n", profile.Profile, profile.Backend)
			}
			return nil
		},
	}
	command.Flags().StringVar(&profileName, "profile", "", "要检查的 profile 名称")
	return command
}
