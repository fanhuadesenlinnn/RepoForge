// Package repo defines the single-file RepoForge configuration (schema v2).
//
// One repo.yaml holds the entire configuration: global runtime environment,
// shared variables, and a list of repositories. Each repository carries its
// upstream URL (with variable placeholders), build strategy (sync/install),
// and distribution settings. Nothing relies on a host yum/apt.
package repo

// ConfigSchema is the single-file schema version.
const ConfigSchema = 2

// Config is the root of the single-file configuration.
type Config struct {
	SchemaVersion int          `yaml:"schema_version"`
	Vars          VarMap       `yaml:"vars"` // global shared variables
	App           App          `yaml:"app"`
	Paths         Paths        `yaml:"paths"`
	Server        Server       `yaml:"server"`
	Repositories  []Repository `yaml:"repositories"`
}

// App holds global application settings.
type App struct {
	Name     string `yaml:"name"`
	LogLevel string `yaml:"log_level"`
}

// Paths holds host runtime paths. auto expands to the detected home.
type Paths struct {
	HomeDir     string `yaml:"home_dir"`
	RepoDir     string `yaml:"repo_dir"` // global repository root (unique source of truth)
	CacheDir    string `yaml:"cache_dir"`
	ClientDir   string `yaml:"client_dir"`
	LogDir      string `yaml:"log_dir"`
	TemplateDir string `yaml:"template_dir"`
}

// Server holds HTTP distribution settings.
type Server struct {
	Listen    string        `yaml:"listen"`
	Root      string        `yaml:"root"`
	PublicURL string        `yaml:"public_url"`
	Readonly  bool          `yaml:"readonly"`
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
	Install    Install    `yaml:"install"`
	Dependency Dependency `yaml:"dependency"`
	Local      LocalRepo  `yaml:"local"`
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
type Var struct {
	Name   string   `yaml:"name"`
	Value  string   `yaml:"value"`
	Values []string `yaml:"values"`
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
	// Concurrency controls how many files download in parallel (multi-file).
	Concurrency int `yaml:"concurrency"`
	// SegmentThreshold is the base segment size (MiB). A single file larger
	// than this is split into ceil(size/threshold) segments automatically,
	// capped at MaxSegments. No per-file segment count needs to be configured.
	SegmentThreshold int64 `yaml:"segment_threshold"`
	// MaxSegments caps how many concurrent Range segments a single file uses.
	MaxSegments int  `yaml:"max_segments"`
	Resume      bool `yaml:"resume"`
}

// Install enables on-demand building (specified packages + dependency solving).
type Install struct {
	Packages []string `yaml:"packages"`
}

// Dependency holds dependency solving strategy.
type Dependency struct {
	WeakDeps  bool   `yaml:"weak_deps"` // include Recommends/Suggests
	Conflicts string `yaml:"conflicts"` // report | resolve
}

// LocalRepo holds settings for enabling the local file:// repository (use).
type LocalRepo struct {
	EnabledExternal bool `yaml:"enabled_external"`
}

// ClientRepo holds settings for LAN/server distribution.
type ClientRepo struct {
	RepoID   string `yaml:"repo_id"`
	BaseURL  string `yaml:"baseurl"`
	GPGCheck bool   `yaml:"gpgcheck"`
}
