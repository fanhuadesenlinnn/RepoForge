// Package templates embeds the files `repoforge init` generates and the
// systemd service unit used by `repoforge server enable`.
package templates

import "embed"

//go:embed *.yaml *.tpl
var files embed.FS

// Asset maps an embedded source file to its initialized destination.
type Asset struct {
	Source      string
	Destination string
	Mode        uint32
}

// Assets returns the files managed by repoforge init. Only the single-file
// configuration is materialized: the systemd unit is read directly from the
// embedded FS (see ReadSystemdService), and client/local repo snippets are
// generated inline by the use/client commands.
func Assets() []Asset {
	return []Asset{
		{Source: "repo.yaml", Destination: "config/repo.yaml", Mode: 0o644},
	}
}

// Read returns one embedded file.
func Read(name string) ([]byte, error) {
	return files.ReadFile(name)
}

// ReadSystemdService returns the embedded repoforge-server.service unit
// template, rendered later by the server command with the actual home,
// executable and repo dir values.
func ReadSystemdService() ([]byte, error) {
	return files.ReadFile("repoforge-server.service.tpl")
}
