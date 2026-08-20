// Package repo defines the single-file RepoForge configuration (schema v2).
//
// One repo.yaml holds the entire configuration: global runtime environment,
// shared variables, and a list of repositories. Each repository carries its
// upstream URL (with variable placeholders), build strategy (sync/install),
// and distribution settings. Nothing relies on a host yum/apt.
package repo

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ConfigSchema is the single-file schema version.
const ConfigSchema = 2

// Config is the root of the single-file configuration.
type Config struct {
	SchemaVersion int          `yaml:"schema_version"`
	Vars          VarMap       `yaml:"vars"` // global shared variables
	App           App          `yaml:"app"`
	Paths         Paths        `yaml:"paths"`
	Server        Server       `yaml:"server"`
	Signing       Signing      `yaml:"signing"`
	Repositories  []Repository `yaml:"repositories"`
}

// App holds global application settings.
type App struct {
	Name     string `yaml:"name"`
	LogLevel string `yaml:"log_level"`
}

// Paths holds host runtime paths. auto expands to the detected home.
type Paths struct {
	HomeDir   string `yaml:"home_dir"`
	RepoDir   string `yaml:"repo_dir"` // global repository root (unique source of truth)
	CacheDir  string `yaml:"cache_dir"`
	ClientDir string `yaml:"client_dir"`
	LogDir    string `yaml:"log_dir"`
}

// Signing holds OpenPGP signing settings for generated repository metadata
// (repomd.xml.asc for yum, Release/InRelease for apt).
type Signing struct {
	Enabled bool `yaml:"enabled"`
	// PrivateKey is the path to the ASCII-armored OpenPGP private key used to
	// sign. Empty defaults to ${home}/config/signing/private.key.
	PrivateKey string `yaml:"private_key"`
}

// Server holds HTTP distribution settings.
type Server struct {
	Listen    string        `yaml:"listen"`
	Root      string        `yaml:"root"`
	PublicURL string        `yaml:"public_url"`
	Systemd   ServerSystemd `yaml:"systemd"`
}

// ServerSystemd holds systemd service settings.
type ServerSystemd struct {
	Enabled     bool   `yaml:"enabled"`
	ServiceName string `yaml:"service_name"`
	ServiceFile string `yaml:"service_file"`
	Restart     string `yaml:"restart"`
}

// VarMap maps a variable name to a single or multi value.
type VarMap map[string][]string

// Repository is one self-contained repository definition.
type Repository struct {
	Name       string     `yaml:"name"`
	Backend    string     `yaml:"backend"`  // rpm | deb
	RepoDir    string     `yaml:"repo_dir"` // optional content root override
	Target     Target     `yaml:"target"`
	Upstream   Upstream   `yaml:"upstream"`
	Sync       Sync       `yaml:"sync"`
	Input      Input      `yaml:"input"`
	Dependency Dependency `yaml:"dependency"`
	Client     ClientRepo `yaml:"client"`
}

// Target describes the OS/version/arch this repository targets.
type Target struct {
	OS      string `yaml:"os"`
	Version string `yaml:"version"`
	Arch    string `yaml:"arch"`
}

// Upstream is the remote repository, with variable placeholders in URL.
// For simplicity a single url/vars may be given; for real multi-repo
// distributions (e.g. CentOS BaseOS+AppStream) supply Sources to aggregate.
type Upstream struct {
	URL    string   `yaml:"url"`
	Vars   []Var    `yaml:"vars"`   // local variables, may override global
	Arch   []string `yaml:"arch"`   // DEB arch list (all = noarch equivalent)
	Suites []Suite  `yaml:"suites"` // DEB suite/component
	Verify string   `yaml:"verify"` // auto | sha256 | ...
	// Sources, when set, is a list of upstreams aggregated for dependency
	// resolution into a single output repository.
	Sources []Source `yaml:"sources"`
}

// Source is one aggregate upstream entry (a separate repo of the same distro).
type Source struct {
	URL    string   `yaml:"url"`
	Vars   []Var    `yaml:"vars"`
	Arch   []string `yaml:"arch"`
	Suites []Suite  `yaml:"suites"`
}

// Var is one named variable with either a single value or multiple values.
// Both forms are accepted for ergonomics:
//
//	value: x86_64                → single value
//	value: [x86_64, aarch64]     → multiple values (list)
//	values: [x86_64, aarch64]    → multiple values (canonical form)
type Var struct {
	Name   string   `yaml:"name"`
	Value  string   `yaml:"value"`
	Values []string `yaml:"values"`
}

// UnmarshalYAML accepts value as a scalar or a list. Writing
// value: [x86_64, aarch64] is stored into Values, so it expands into
// multiple variants instead of failing with a cryptic unmarshal error.
func (v *Var) UnmarshalYAML(node *yaml.Node) error {
	var raw struct {
		Name   string    `yaml:"name"`
		Value  yaml.Node `yaml:"value"`
		Values []string  `yaml:"values"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}
	v.Name = raw.Name
	v.Values = raw.Values
	if raw.Value.Kind != 0 {
		switch raw.Value.Kind {
		case yaml.ScalarNode:
			var s string
			if err := raw.Value.Decode(&s); err != nil {
				return err
			}
			v.Value = s
		case yaml.SequenceNode:
			var list []string
			if err := raw.Value.Decode(&list); err != nil {
				return err
			}
			v.Values = list
		default:
			return fmt.Errorf("变量 %s 的 value 需要是字符串或字符串列表", raw.Name)
		}
	}
	return nil
}

// UnmarshalYAML accepts each global variable as a scalar or a list:
//
//	vars: { basearch: x86_64 }            → [x86_64]
//	vars: { basearch: [x86_64, aarch64] } → [x86_64, aarch64]
func (m *VarMap) UnmarshalYAML(node *yaml.Node) error {
	*m = VarMap{}
	if node.Kind != yaml.MappingNode {
		return errors.New("vars 需要是映射（如 vars: { basearch: [x86_64, aarch64] }）")
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		var key string
		if err := keyNode.Decode(&key); err != nil {
			return err
		}
		switch valNode.Kind {
		case yaml.ScalarNode:
			var s string
			if err := valNode.Decode(&s); err != nil {
				return err
			}
			(*m)[key] = []string{s}
		case yaml.SequenceNode:
			var list []string
			if err := valNode.Decode(&list); err != nil {
				return err
			}
			(*m)[key] = list
		default:
			return fmt.Errorf("vars.%s 需要是字符串或字符串列表", key)
		}
	}
	return nil
}

// Suite is a DEB suite/component pair.
type Suite struct {
	Suite      string   `yaml:"suite"`
	Components []string `yaml:"components"`
	Arch       []string `yaml:"arch"`
}

// Sync enables full mirroring for a repository.
type Sync struct {
	Enabled      bool   `yaml:"enabled"`
	DeletePolicy string `yaml:"delete_policy"` // keep | prune | prune-expired
	// ExpireDays is the grace period (days) before prune-expired removes a
	// package that upstream no longer provides. 0 = default (30).
	ExpireDays int `yaml:"expire_days"`
	// Concurrency controls how many files download in parallel (multi-file).
	Concurrency int `yaml:"concurrency"`
	// SegmentThreshold is the base segment size (MiB). A single file larger
	// than this is split into segment(s) when segmentation is enabled.
	SegmentThreshold int64 `yaml:"segment_threshold"`
	// Segment is both a switch and a cap:
	//   false       → segmentation disabled (single connection per file)
	//   <n> (int)   → enabled, at most n segments per large file
	//   absent/true → smart default (auto segments, capped at 8)
	Segment SegmentMode `yaml:"segment"`
	Resume  bool        `yaml:"resume"`
}

// SegmentMode holds the segment setting.
//
//	 0  → unset (absent); load defaults it to smart
//	-1  → smart default (auto segments, capped)
//	-2  → disabled (false): single connection per file
//	>0  → fixed max segments per large file
type SegmentMode int

const (
	SegmentSmart    SegmentMode = -1
	SegmentDisabled SegmentMode = -2
)

// UnmarshalYAML accepts false, true, an integer, or null.
func (s *SegmentMode) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		switch value.Tag {
		case "!!bool":
			var b bool
			if err := value.Decode(&b); err != nil {
				return err
			}
			if b {
				*s = SegmentSmart // true → smart default
			} else {
				*s = SegmentDisabled // false → disabled
			}
			return nil
		case "!!int":
			var n int
			if err := value.Decode(&n); err != nil {
				return err
			}
			if n < 2 {
				*s = SegmentSmart // 0/1 → treat as smart default
			} else {
				*s = SegmentMode(n)
			}
			return nil
		case "!!null":
			*s = SegmentSmart
			return nil
		}
	}
	return errors.New("segment 需要 false / 正整数 / 缺省")
}

// Input holds the make command's starting points. Multiple may be set; the
// engine unionizes them and resolves the resulting dependency set.
type Input struct {
	// Packages are packages explicitly requested for the offline source
	// (their dependencies are resolved too). Formerly make.packages.
	Packages []string `yaml:"packages"`
	// PackageDirs are directories with pre-existing rpms/debs used as a
	// starting point: their missing dependencies are fetched into repo_dir.
	PackageDirs []string `yaml:"package_dirs"`
	Recursive   bool     `yaml:"recursive"`
	// UpgradePackages lists packages to fetch at their latest available version
	// from upstream (plus their dependencies) — i.e. build an upgrade source.
	UpgradePackages []string `yaml:"upgrade_packages"`
}

// Dependency holds dependency solving strategy.
type Dependency struct {
	WeakDeps  bool   `yaml:"weak_deps"` // include Recommends/Suggests
	Conflicts string `yaml:"conflicts"` // report | resolve
}

// ClientRepo holds settings for LAN/server distribution.
type ClientRepo struct {
	RepoID   string `yaml:"repo_id"`
	BaseURL  string `yaml:"baseurl"`
	GPGCheck bool   `yaml:"gpgcheck"`
}
