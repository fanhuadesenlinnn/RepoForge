package rpm

import (
	"reflect"
	"testing"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
)

func TestRPMUpgradeDownloadArgsDNF(t *testing.T) {
	profile := &config.ProfileConfig{
		Online: config.OnlineConfig{
			Releasever:      "8",
			DisableRepos:    []string{"*"},
			EnableRepos:     []string{"base", "updates"},
			IncludeWeakDeps: false,
		},
		Repository: config.RepositoryConfig{
			PackageDir: "/opt/repoforge/repos/kylin-v10-sp3-x86_64",
		},
	}

	got := rpmUpgradeDownloadArgs("/usr/bin/dnf", profile)
	want := []string{
		"--releasever=8",
		"--disablerepo=*",
		"--enablerepo=base",
		"--enablerepo=updates",
		"upgrade",
		"-y",
		"--downloadonly",
		"--downloaddir", "/opt/repoforge/repos/kylin-v10-sp3-x86_64",
		"--setopt=install_weak_deps=false",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rpmUpgradeDownloadArgs() = %#v, want %#v", got, want)
	}
}

func TestRPMUpgradeDownloadArgsYUMUsesUpdate(t *testing.T) {
	profile := &config.ProfileConfig{
		Online: config.OnlineConfig{
			IncludeWeakDeps: true,
		},
		Repository: config.RepositoryConfig{
			PackageDir: "/repo",
		},
	}

	got := rpmUpgradeDownloadArgs("yum", profile)
	want := []string{
		"update",
		"-y",
		"--downloadonly",
		"--downloaddir", "/repo",
		"--setopt=install_weak_deps=true",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rpmUpgradeDownloadArgs() = %#v, want %#v", got, want)
	}
}
