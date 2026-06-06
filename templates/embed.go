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
		{Source: "profiles/kylin-v10-sp3-x86_64.yaml", Destination: "config/profiles/kylin-v10-sp3-x86_64.yaml", Mode: 0o644},
		{Source: "profiles/debian-12-amd64.yaml", Destination: "config/profiles/debian-12-amd64.yaml", Mode: 0o644},
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
