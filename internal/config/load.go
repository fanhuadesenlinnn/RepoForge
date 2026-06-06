package config

import (
	"fmt"
	"os"
	"path/filepath"
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
	if len(entry.Packages) == 0 {
		return nil, fmt.Errorf("profile %q 的软件包列表为空", profile)
	}
	return entry.Packages, nil
}

// LoadProfile reads and expands a profile configuration.
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
	expandProfile(home, &cfg)
	return &cfg, nil
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
}

func expandHome(home, value string) string {
	return strings.ReplaceAll(value, "${home}", home)
}
