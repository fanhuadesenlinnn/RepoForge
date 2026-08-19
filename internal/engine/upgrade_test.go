package engine

import (
	"testing"

	"github.com/fanhuadesenlinnn/RepoForge/internal/upstream"
)

func TestUpgradesFromInstalledPicksNewer(t *testing.T) {
	ix := &upstream.Index{
		Backend: "rpm",
		Packages: []upstream.Pkg{
			{Name: "vim", Epoch: "0", Version: "8.2", Release: "1", Arch: "x86_64", Location: "vim.rpm"},
			{Name: "curl", Epoch: "0", Version: "7.0", Release: "1", Arch: "x86_64", Location: "curl.rpm"},
		},
	}
	installed := []InstalledPkg{
		{Name: "vim", Epoch: "0", Version: "8.1", Release: "1", Arch: "x86_64"},
		{Name: "curl", Epoch: "0", Version: "7.0", Release: "1", Arch: "x86_64"},
		{Name: "local-only", Version: "1", Arch: "x86_64"},
	}
	names, _ := upgradesFromInstalled(ix, installed, SolveOptions{Backend: "rpm"})
	if len(names) != 1 || names[0] != "vim" {
		t.Fatalf("names = %v, want [vim]", names)
	}
}
