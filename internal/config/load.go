package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads and expands the global configuration.
func Load(home string) (*Config, error) {
	var cfg Config
	if err := loadYAML(filepath.Join(home, "config", "config.yaml"), &cfg); err != nil {
		return nil, err
	}
	if cfg.SchemaVersion != 1 {
		return nil, fmt.Errorf("不支持的 config.yaml schema_version: %d", cfg.SchemaVersion)
	}
	expandConfig(home, &cfg)
	return &cfg, nil
}

// LoadPackages reads the package list for every profile.
func LoadPackages(home string) (*PackagesConfig, error) {
	var cfg PackagesConfig
	if err := loadYAML(filepath.Join(home, "config", "packages.yaml"), &cfg); err != nil {
		return nil, err
	}
	if cfg.SchemaVersion != 1 {
		return nil, fmt.Errorf("不支持的 packages.yaml schema_version: %d", cfg.SchemaVersion)
	}
	return &cfg, nil
}

// PackagesForProfile returns the configured packages for one profile.
func PackagesForProfile(home, profile string) ([]string, error) {
	cfg, err := LoadPackages(home)
	if err != nil {
		return nil, err
	}
	entry, ok := cfg.Profiles[profile]
	if !ok {
		return nil, fmt.Errorf("packages.yaml 中不存在 profile %q", profile)
	}
	return entry.Packages, nil
}

// LoadProfile reads, defaults, and expands a profile configuration.
func LoadProfile(home, name string) (*ProfileConfig, error) {
	if name == "" || filepath.Base(name) != name {
		return nil, fmt.Errorf("profile 名称无效: %q", name)
	}
	var cfg ProfileConfig
	path := filepath.Join(home, "config", "profiles", name+".yaml")
	if err := loadYAML(path, &cfg); err != nil {
		return nil, err
	}
	if cfg.SchemaVersion != 1 {
		return nil, fmt.Errorf("不支持的 profile schema_version: %d", cfg.SchemaVersion)
	}
	if cfg.Profile != name {
		return nil, fmt.Errorf("profile 文件名与配置名称不一致: %s != %s", name, cfg.Profile)
	}
	if cfg.Backend != "rpm" && cfg.Backend != "deb" {
		return nil, fmt.Errorf("profile %q 使用了不支持的 backend %q", name, cfg.Backend)
	}
	applyProfileDefaults(&cfg)
	expandProfile(home, &cfg)
	return &cfg, nil
}

// LoadProfiles reads every configured profile in deterministic order.
func LoadProfiles(home string) ([]*ProfileConfig, error) {
	matches, err := filepath.Glob(filepath.Join(home, "config", "profiles", "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("查找 profile 配置失败: %w", err)
	}
	sort.Strings(matches)
	profiles := make([]*ProfileConfig, 0, len(matches))
	for _, path := range matches {
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		profile, err := LoadProfile(home, name)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

// applyProfileDefaults fills derived fields when the user has not set them.
func applyProfileDefaults(cfg *ProfileConfig) {
	name := cfg.Profile
	isRPM := cfg.Backend == "rpm"

	// Compatibility: relaxed for personal use.
	if cfg.Compatibility.RequireSameOS == false &&
		cfg.Compatibility.RequireSameVersion == false &&
		cfg.Compatibility.RequireSameArch == false {
		cfg.Compatibility.RequireSameOS = true
		cfg.Compatibility.RequireSameVersion = true
		cfg.Compatibility.RequireSameArch = true
	}
	if cfg.Compatibility.AllowCrossBuild == false {
		cfg.Compatibility.AllowCrossBuild = true
	}

	// Online: paths derived from profile name.
	if cfg.Online.Installroot == "" {
		cfg.Online.Installroot = fmt.Sprintf("${home}/cache/rpm/installroot/%s", name)
	}

	// Repository: paths derived from profile name.
	if cfg.Repository.ProfileDir == "" {
		cfg.Repository.ProfileDir = fmt.Sprintf("${home}/repos/%s", name)
	}
	if cfg.Repository.PackageDir == "" {
		cfg.Repository.PackageDir = fmt.Sprintf("${home}/repos/%s", name)
	}
	if cfg.Repository.MetadataTool == "" {
		if isRPM {
			cfg.Repository.MetadataTool = "createrepo_c"
		} else {
			cfg.Repository.MetadataTool = "dpkg-scanpackages"
		}
	}

	// LocalRepo: standard paths.
	if cfg.LocalRepo.RepoFile == "" {
		if isRPM {
			cfg.LocalRepo.RepoFile = "/etc/yum.repos.d/repoforge-local.repo"
		} else {
			cfg.LocalRepo.RepoFile = "/etc/apt/sources.list.d/repoforge-local.list"
		}
	}
	if cfg.LocalRepo.BaseURL == "" {
		cfg.LocalRepo.BaseURL = fmt.Sprintf("file://${home}/repos/%s", name)
	}
	if cfg.LocalRepo.RepoID == "" {
		cfg.LocalRepo.RepoID = "repoforge-local"
	}
	if cfg.LocalRepo.RepoName == "" {
		if isRPM {
			cfg.LocalRepo.RepoName = "RepoForge Local RPM Repo"
		} else {
			cfg.LocalRepo.RepoName = "RepoForge Local DEB Repo"
		}
	}
	if cfg.LocalRepo.Suite == "" && !isRPM {
		cfg.LocalRepo.Suite = "./"
	}

	// ClientRepo: derived paths.
	if cfg.ClientRepo.Output == "" {
		ext := ".repo"
		if !isRPM {
			ext = ".list"
		}
		cfg.ClientRepo.Output = fmt.Sprintf("${home}/client/repoforge-%s"+ext, name)
	}
	if cfg.ClientRepo.BaseURL == "" {
		cfg.ClientRepo.BaseURL = fmt.Sprintf("${server.public_url}/%s", name)
	}
	if cfg.ClientRepo.RepoID == "" {
		cfg.ClientRepo.RepoID = "repoforge-lan"
	}
	if cfg.ClientRepo.RepoName == "" {
		if isRPM {
			cfg.ClientRepo.RepoName = "RepoForge LAN RPM Repo"
		} else {
			cfg.ClientRepo.RepoName = "RepoForge LAN DEB Repo"
		}
	}
	if cfg.ClientRepo.Suite == "" && !isRPM {
		cfg.ClientRepo.Suite = "./"
	}

	// DEB-specific: apt root paths.
	if !isRPM {
		if cfg.Online.APTRoot == "" {
			cfg.Online.APTRoot = fmt.Sprintf("${home}/cache/deb/apt-root/%s", name)
		}
		if cfg.Online.APTCache == "" {
			cfg.Online.APTCache = fmt.Sprintf("${home}/cache/deb/apt-cache/%s", name)
		}
		if cfg.Online.APTState == "" {
			cfg.Online.APTState = fmt.Sprintf("${home}/cache/deb/apt-state/%s", name)
		}
		if cfg.Online.APTSourcesMode == "" {
			cfg.Online.APTSourcesMode = "copy_from_host"
		}
		cfg.Online.UseAPTRoot = true
		cfg.Online.RunAPTUpdateBeforeMake = true
		cfg.LocalRepo.Trusted = true
		cfg.LocalRepo.UpdateAfterEnable = true
	} else {
		cfg.Online.UseInstallroot = true
		cfg.Online.CleanInstallrootBeforeMake = true
		cfg.Repository.CreaterepoUpdate = true
		cfg.LocalRepo.MakecacheAfterEnable = true
	}
}

func loadYAML(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取配置文件 %s 失败: %w", path, err)
	}
	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
	}
	return nil
}

func expandConfig(home string, cfg *Config) {
	cfg.Paths.HomeDir = expandHome(home, cfg.Paths.HomeDir)
	if cfg.Paths.HomeDir == "auto" {
		cfg.Paths.HomeDir = home
	}
	cfg.Paths.ConfigDir = expandHome(home, cfg.Paths.ConfigDir)
	cfg.Paths.ProfileDir = expandHome(home, cfg.Paths.ProfileDir)
	cfg.Paths.TemplateDir = expandHome(home, cfg.Paths.TemplateDir)
	cfg.Paths.RepoDir = expandHome(home, cfg.Paths.RepoDir)
	cfg.Paths.CacheDir = expandHome(home, cfg.Paths.CacheDir)
	cfg.Paths.ClientDir = expandHome(home, cfg.Paths.ClientDir)
	cfg.Paths.LogDir = expandHome(home, cfg.Paths.LogDir)
	cfg.Server.Root = expandHome(home, cfg.Server.Root)
}

func expandProfile(home string, cfg *ProfileConfig) {
	cfg.Online.Installroot = expandHome(home, cfg.Online.Installroot)
	cfg.Online.APTRoot = expandHome(home, cfg.Online.APTRoot)
	cfg.Online.APTCache = expandHome(home, cfg.Online.APTCache)
	cfg.Online.APTState = expandHome(home, cfg.Online.APTState)
	cfg.Repository.ProfileDir = expandHome(home, cfg.Repository.ProfileDir)
	cfg.Repository.PackageDir = expandHome(home, cfg.Repository.PackageDir)
	cfg.LocalRepo.BaseURL = expandHome(home, cfg.LocalRepo.BaseURL)
	cfg.ClientRepo.Output = expandHome(home, cfg.ClientRepo.Output)
	for i := range cfg.Input.PackageDirs {
		cfg.Input.PackageDirs[i] = expandHome(home, cfg.Input.PackageDirs[i])
	}
}

func expandHome(home, value string) string {
	return strings.ReplaceAll(value, "${home}", home)
}

// FindMatchingProfiles returns profiles compatible with the given system.
func FindMatchingProfiles(home string, systemOS, systemArch, systemBackend string) ([]*ProfileConfig, error) {
	all, err := LoadProfiles(home)
	if err != nil {
		return nil, err
	}
	var matches []*ProfileConfig
	for _, profile := range all {
		if profile.Backend != systemBackend {
			continue
		}
		profileArch := profile.Target.Arch
		if profileArch == "amd64" {
			profileArch = "x86_64"
		}
		systemArchNorm := systemArch
		if systemArchNorm == "amd64" {
			systemArchNorm = "x86_64"
		}
		if profileArch != systemArchNorm {
			continue
		}
		if !matchesOSString(profile.Target.OS, systemOS) {
			continue
		}
		matches = append(matches, profile)
	}
	return matches, nil
}

func matchesOSString(targetOS, currentOS string) bool {
	targetOS = strings.ToLower(targetOS)
	currentOS = strings.ToLower(currentOS)
	if targetOS == currentOS {
		return true
	}
	rpmFamily := map[string]bool{"centos": true, "rhel": true, "rocky": true, "almalinux": true, "fedora": true}
	debFamily := map[string]bool{"debian": true, "ubuntu": true}
	if rpmFamily[targetOS] && rpmFamily[currentOS] {
		return true
	}
	if debFamily[targetOS] && debFamily[currentOS] {
		return true
	}
	return false
}
