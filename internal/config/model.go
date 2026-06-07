package config

// Config is the global RepoForge configuration.
type Config struct {
	SchemaVersion int           `yaml:"schema_version"`
	App           AppConfig     `yaml:"app"`
	Paths         PathsConfig   `yaml:"paths"`
	Default       DefaultConfig `yaml:"default"`
	Server        ServerConfig  `yaml:"server"`
	Verify        VerifyConfig  `yaml:"verify"`
}

type AppConfig struct {
	Name     string `yaml:"name"`
	Language string `yaml:"language"`
	LogLevel string `yaml:"log_level"`
}

type PathsConfig struct {
	HomeDir     string `yaml:"home_dir"`
	ConfigDir   string `yaml:"config_dir"`
	ProfileDir  string `yaml:"profile_dir"`
	TemplateDir string `yaml:"template_dir"`
	RepoDir     string `yaml:"repo_dir"`
	CacheDir    string `yaml:"cache_dir"`
	ClientDir   string `yaml:"client_dir"`
	LogDir      string `yaml:"log_dir"`
}

type DefaultConfig struct {
	Backend string `yaml:"backend"`
	Profile string `yaml:"profile"`
}

type ServerConfig struct {
	Listen           string         `yaml:"listen"`
	Root             string         `yaml:"root"`
	PublicURL        string         `yaml:"public_url"`
	Readonly         bool           `yaml:"readonly"`
	DirectoryListing bool           `yaml:"directory_listing"`
	Systemd          SystemdConfig  `yaml:"systemd"`
	Firewall         FirewallConfig `yaml:"firewall"`
}

type SystemdConfig struct {
	Enabled     bool   `yaml:"enabled"`
	ServiceName string `yaml:"service_name"`
	ServiceFile string `yaml:"service_file"`
	Restart     string `yaml:"restart"`
}

type FirewallConfig struct {
	Manage   bool   `yaml:"manage"`
	Port     int    `yaml:"port"`
	Protocol string `yaml:"protocol"`
}

type VerifyConfig struct {
	CheckOS        bool `yaml:"check_os"`
	CheckArch      bool `yaml:"check_arch"`
	CheckIndex     bool `yaml:"check_index"`
	CheckLocalRepo bool `yaml:"check_local_repo"`
	CheckHTTP      bool `yaml:"check_http"`
}

type PackagesConfig struct {
	SchemaVersion int                       `yaml:"schema_version"`
	Profiles      map[string]PackageProfile `yaml:"profiles"`
}

type PackageProfile struct {
	Packages []string `yaml:"packages"`
}

// InputConfig describes package import sources.
type InputConfig struct {
	PackageDirs []string `yaml:"package_dirs"`
	Recursive   bool     `yaml:"recursive"`
}

type ProfileConfig struct {
	SchemaVersion int                 `yaml:"schema_version"`
	Profile       string              `yaml:"profile"`
	Backend       string              `yaml:"backend"`
	Target        TargetConfig        `yaml:"target"`
	Compatibility CompatibilityConfig `yaml:"compatibility"`
	Online        OnlineConfig        `yaml:"online"`
	Repository    RepositoryConfig    `yaml:"repository"`
	LocalRepo     LocalRepoConfig     `yaml:"local_repo"`
	ClientRepo    ClientRepoConfig    `yaml:"client_repo"`
	Input         InputConfig         `yaml:"input"`
}

type TargetConfig struct {
	OS       string `yaml:"os"`
	Version  string `yaml:"version"`
	Codename string `yaml:"codename"`
	Arch     string `yaml:"arch"`
}

type CompatibilityConfig struct {
	RequireSameOS      bool `yaml:"require_same_os"`
	RequireSameVersion bool `yaml:"require_same_version"`
	RequireSameArch    bool `yaml:"require_same_arch"`
	AllowCrossBuild    bool `yaml:"allow_cross_build"`
}

type OnlineConfig struct {
	PackageManager             string   `yaml:"package_manager"`
	Resolver                   string   `yaml:"resolver"`
	Releasever                 string   `yaml:"releasever"`
	EnableRepos                []string `yaml:"enable_repos"`
	DisableRepos               []string `yaml:"disable_repos"`
	IncludeWeakDeps            bool     `yaml:"include_weak_deps"`
	UseInstallroot             bool     `yaml:"use_installroot"`
	Installroot                string   `yaml:"installroot"`
	CleanInstallrootBeforeMake bool     `yaml:"clean_installroot_before_make"`
	IncludeRecommends          bool     `yaml:"include_recommends"`
	IncludeSuggests            bool     `yaml:"include_suggests"`
	UseAPTRoot                 bool     `yaml:"use_apt_root"`
	APTRoot                    string   `yaml:"apt_root"`
	APTCache                   string   `yaml:"apt_cache"`
	APTState                   string   `yaml:"apt_state"`
	APTSourcesMode             string   `yaml:"apt_sources_mode"`
	RunAPTUpdateBeforeMake     bool     `yaml:"run_apt_update_before_make"`
}

type RepositoryConfig struct {
	ProfileDir       string `yaml:"profile_dir"`
	PackageDir       string `yaml:"package_dir"`
	MetadataTool     string `yaml:"metadata_tool"`
	CreaterepoUpdate bool   `yaml:"createrepo_update"`
	GPGCheck         bool   `yaml:"gpgcheck"`
	Trusted          bool   `yaml:"trusted"`
}

type LocalRepoConfig struct {
	RepoID               string `yaml:"repo_id"`
	RepoName             string `yaml:"repo_name"`
	RepoFile             string `yaml:"repo_file"`
	BaseURL              string `yaml:"baseurl"`
	MakecacheAfterEnable bool   `yaml:"makecache_after_enable"`
	Suite                string `yaml:"suite"`
	Trusted              bool   `yaml:"trusted"`
	UpdateAfterEnable    bool   `yaml:"update_after_enable"`
}

type ClientRepoConfig struct {
	RepoID   string `yaml:"repo_id"`
	RepoName string `yaml:"repo_name"`
	Output   string `yaml:"output"`
	BaseURL  string `yaml:"baseurl"`
	GPGCheck bool   `yaml:"gpgcheck"`
	Suite    string `yaml:"suite"`
	Trusted  bool   `yaml:"trusted"`
}
