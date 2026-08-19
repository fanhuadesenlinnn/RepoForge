package templates

import "embed"

//go:embed *.yaml profiles/*.yaml *.tpl
var files embed.FS

// Asset maps an embedded source file to its initialized destination.
type Asset struct {
	Source      string
	Destination string
	Mode        uint32
}

// Assets returns all files managed by repoforge init.
func Assets() []Asset {
	return []Asset{
		{Source: "config.yaml", Destination: "config/config.yaml", Mode: 0o644},
		{Source: "packages.yaml", Destination: "config/packages.yaml", Mode: 0o644},
		{Source: "repo.yaml", Destination: "config/repo.yaml", Mode: 0o644},
		// RPM profiles
		{Source: "profiles/kylin-v10-sp3-x86_64.yaml", Destination: "config/profiles/kylin-v10-sp3-x86_64.yaml", Mode: 0o644},
		{Source: "profiles/kylin-v10-sp3-aarch64.yaml", Destination: "config/profiles/kylin-v10-sp3-aarch64.yaml", Mode: 0o644},
		{Source: "profiles/centos-7-x86_64.yaml", Destination: "config/profiles/centos-7-x86_64.yaml", Mode: 0o644},
		{Source: "profiles/rocky-8-x86_64.yaml", Destination: "config/profiles/rocky-8-x86_64.yaml", Mode: 0o644},
		{Source: "profiles/rocky-9-x86_64.yaml", Destination: "config/profiles/rocky-9-x86_64.yaml", Mode: 0o644},
		{Source: "profiles/openEuler-22.03-x86_64.yaml", Destination: "config/profiles/openEuler-22.03-x86_64.yaml", Mode: 0o644},
		{Source: "profiles/openEuler-24.03-x86_64.yaml", Destination: "config/profiles/openEuler-24.03-x86_64.yaml", Mode: 0o644},
		// DEB profiles
		{Source: "profiles/debian-11-amd64.yaml", Destination: "config/profiles/debian-11-amd64.yaml", Mode: 0o644},
		{Source: "profiles/debian-12-amd64.yaml", Destination: "config/profiles/debian-12-amd64.yaml", Mode: 0o644},
		{Source: "profiles/ubuntu-22.04-amd64.yaml", Destination: "config/profiles/ubuntu-22.04-amd64.yaml", Mode: 0o644},
		{Source: "profiles/ubuntu-24.04-amd64.yaml", Destination: "config/profiles/ubuntu-24.04-amd64.yaml", Mode: 0o644},
		// Templates
		{Source: "rpm-local.repo.tpl", Destination: "config/templates/rpm-local.repo.tpl", Mode: 0o644},
		{Source: "rpm-client.repo.tpl", Destination: "config/templates/rpm-client.repo.tpl", Mode: 0o644},
		{Source: "deb-local.list.tpl", Destination: "config/templates/deb-local.list.tpl", Mode: 0o644},
		{Source: "deb-client.list.tpl", Destination: "config/templates/deb-client.list.tpl", Mode: 0o644},
		{Source: "repoforge-server.service.tpl", Destination: "config/templates/repoforge-server.service.tpl", Mode: 0o644},
	}
}

// Read returns one embedded template.
func Read(name string) ([]byte, error) {
	return files.ReadFile(name)
}
