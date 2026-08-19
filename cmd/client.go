package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fanhuadesenlinnn/RepoForge/internal/home"
	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/spf13/cobra"
)

// generateClient writes client repository config files (.repo for RPM,
// .list for DEB) for every repository into the client directory.
func generateClient(cfg *repo.Config) error {
	publicURL := cfg.Server.PublicURL
	if publicURL == "" || publicURL == "auto" {
		publicURL = "http://127.0.0.1:" + portOf(cfg.Server.Listen)
	}
	for i := range cfg.Repositories {
		r := &cfg.Repositories[i]
		root := cfg.ContentRoot(r)
		if err := os.MkdirAll(cfg.Paths.ClientDir, 0o755); err != nil {
			return err
		}
		if r.Backend == "rpm" {
			content := formatRPMRepo(r, publicURL, root)
			dst := filepath.Join(cfg.Paths.ClientDir, "repoforge-"+r.Name+".repo")
			if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
				return err
			}
		} else {
			content := formatDEBList(r, publicURL)
			dst := filepath.Join(cfg.Paths.ClientDir, "repoforge-"+r.Name+".list")
			if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

func formatRPMRepo(r *repo.Repository, publicURL, root string) string {
	repoID := r.Client.RepoID
	if repoID == "" {
		repoID = "repoforge-lan"
	}
	// BaseURL: prefer configured, else derive from public URL + repo name.
	base := r.Client.BaseURL
	if base == "" {
		base = publicURL + "/" + r.Name
	}
	return fmt.Sprintf("[%s]\nname=%s\nbaseurl=%s\nenabled=1\ngpgcheck=%d\n",
		repoID, r.Name, base, boolInt(r.Client.GPGCheck))
}

func formatDEBList(r *repo.Repository, publicURL string) string {
	base := r.Client.BaseURL
	if base == "" {
		base = publicURL + "/" + r.Name
	}
	var out string
	for _, s := range r.Upstream.Suites {
		for _, comp := range s.Components {
			out += fmt.Sprintf("deb %s %s %s\n", base, s.Suite, comp)
		}
	}
	if out == "" {
		out = fmt.Sprintf("deb %s ./\n", base)
	}
	return out
}

func portOf(listen string) string {
	for i := len(listen) - 1; i >= 0; i-- {
		if listen[i] == ':' {
			return listen[i+1:]
		}
	}
	return "8080"
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func newClientCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "client",
		Short: "按 repo.yaml 生成客户端软件源配置",
		Args:  noArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			homeDir, err := home.Detect(false)
			if err != nil {
				return err
			}
			cfg, err := repo.Load(homeDir)
			if err != nil {
				return err
			}
			if err := generateClient(cfg); err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "已生成客户端配置到: %s\n", cfg.Paths.ClientDir)
			return nil
		},
	}
	return command
}
