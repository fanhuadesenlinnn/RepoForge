package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultFileName is the single-file configuration name.
const DefaultFileName = "repo.yaml"

// FindFile locates repo.yaml under home, preferring config/ then root.
func FindFile(home string) (string, error) {
	for _, candidate := range []string{
		filepath.Join(home, "config", DefaultFileName),
		filepath.Join(home, DefaultFileName),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("未找到单文件配置 %s（支持 %s 或 %s）",
		DefaultFileName,
		filepath.Join(home, "config", DefaultFileName),
		filepath.Join(home, DefaultFileName))
}

// Load reads the single-file configuration under home.
func Load(home string) (*Config, error) {
	path, err := FindFile(home)
	if err != nil {
		return nil, err
	}
	return LoadFile(path, home)
}

// LoadFile reads and validates a single-file configuration from an explicit path.
func LoadFile(path, home string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置 %s 失败: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置 %s 失败: %w", path, err)
	}
	if cfg.SchemaVersion != ConfigSchema {
		return nil, fmt.Errorf("不支持的 %s schema_version: %d（需要 %d）", filepath.Base(path), cfg.SchemaVersion, ConfigSchema)
	}
	if len(cfg.Repositories) == 0 {
		return nil, fmt.Errorf("%s 中没有配置任何 repositories", filepath.Base(path))
	}
	applyDefaults(home, &cfg)
	if err := validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Resolve returns the repository by name.
func (c *Config) Resolve(name string) (*Repository, error) {
	for i := range c.Repositories {
		if c.Repositories[i].Name == name {
			return &c.Repositories[i], nil
		}
	}
	return nil, fmt.Errorf("配置中不存在 repository %q", name)
}

// SingleOrDefault returns the named repository, or the only one when name is empty.
func (c *Config) SingleOrDefault(name string) (*Repository, error) {
	if name != "" {
		return c.Resolve(name)
	}
	if len(c.Repositories) == 1 {
		return &c.Repositories[0], nil
	}
	names := make([]string, len(c.Repositories))
	for i, r := range c.Repositories {
		names[i] = r.Name
	}
	return nil, fmt.Errorf("有 %d 个 repository，请用 --repo 指定：%s", len(c.Repositories), strings.Join(names, ", "))
}

// ContentRoot returns the resolved content root for the repository, expanding
// repo_dir (global fallback: <repo_dir>/<name>). Multi-arch subdirectories are
// appended by Expand when the upstream expands to multiple variants.
func (c *Config) ContentRoot(r *Repository) string {
	if r.RepoDir != "" {
		return expandHome(c.Paths.HomeDir, r.RepoDir)
	}
	return filepath.Join(c.Paths.RepoDir, r.Name)
}

func applyDefaults(home string, cfg *Config) {
	if cfg.App.Name == "" {
		cfg.App.Name = "repoforge"
	}
	if cfg.App.LogLevel == "" {
		cfg.App.LogLevel = "info"
	}
	if cfg.Paths.HomeDir == "" || cfg.Paths.HomeDir == "auto" {
		cfg.Paths.HomeDir = home
	}
	if cfg.Paths.RepoDir == "" {
		cfg.Paths.RepoDir = filepath.Join(home, "repos")
	}
	if cfg.Paths.CacheDir == "" {
		cfg.Paths.CacheDir = filepath.Join(home, "cache")
	}
	if cfg.Paths.ClientDir == "" {
		cfg.Paths.ClientDir = filepath.Join(home, "client")
	}
	if cfg.Paths.LogDir == "" {
		cfg.Paths.LogDir = filepath.Join(home, "logs")
	}
	if cfg.Paths.TemplateDir == "" {
		cfg.Paths.TemplateDir = filepath.Join(home, "config", "templates")
	}
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = "0.0.0.0:8080"
	}
	if cfg.Server.PublicURL == "" {
		cfg.Server.PublicURL = "auto"
	}
	if cfg.Server.Systemd.ServiceName == "" {
		cfg.Server.Systemd.ServiceName = "repoforge-server"
	}
	if cfg.Server.Systemd.ServiceFile == "" {
		cfg.Server.Systemd.ServiceFile = "/etc/systemd/system/repoforge-server.service"
	}
	if cfg.Server.Systemd.Restart == "" {
		cfg.Server.Systemd.Restart = "always"
	}
	if cfg.Server.Root == "" {
		cfg.Server.Root = cfg.Paths.RepoDir
	}
	// Resolve ${home} in global paths.
	cfg.Paths.RepoDir = expandHome(home, cfg.Paths.RepoDir)
	cfg.Paths.CacheDir = expandHome(home, cfg.Paths.CacheDir)
	cfg.Paths.ClientDir = expandHome(home, cfg.Paths.ClientDir)
	cfg.Paths.LogDir = expandHome(home, cfg.Paths.LogDir)
	cfg.Paths.TemplateDir = expandHome(home, cfg.Paths.TemplateDir)
	cfg.Server.Root = expandHome(home, cfg.Server.Root)

	for i := range cfg.Repositories {
		r := &cfg.Repositories[i]
		if r.Name == "" {
			r.Name = r.Target.OS + "-" + r.Target.Version
		}
		if r.Backend == "" {
			r.Backend = "rpm"
		}
		if r.Upstream.Verify == "" {
			r.Upstream.Verify = "auto"
		}
		if r.Sync.DeletePolicy == "" {
			r.Sync.DeletePolicy = "keep"
		}
		if r.Sync.Concurrency <= 0 {
			r.Sync.Concurrency = 8
		}
		if r.Sync.SegmentThreshold <= 0 {
			r.Sync.SegmentThreshold = 20 // MiB per segment
		}
		if r.Sync.Segment == 0 {
			r.Sync.Segment = SegmentSmart // absent → smart default
		}
		if r.Dependency.Conflicts == "" {
			r.Dependency.Conflicts = "report"
		}
		if r.Client.RepoID == "" {
			r.Client.RepoID = "repoforge-lan"
		}
	}
}

func expandHome(home, value string) string {
	return strings.ReplaceAll(value, "${home}", home)
}

func validate(cfg *Config) error {
	for i := range cfg.Repositories {
		r := &cfg.Repositories[i]
		if r.Backend != "rpm" && r.Backend != "deb" {
			return fmt.Errorf("repository %q 使用了不支持的 backend %q", r.Name, r.Backend)
		}
		if r.Upstream.URL == "" && len(r.Upstream.Sources) == 0 {
			return fmt.Errorf("repository %q 缺少 upstream.url（或 upstream.sources）", r.Name)
		}
		switch r.Sync.DeletePolicy {
		case "keep", "prune", "prune-expired":
		default:
			return fmt.Errorf("repository %q 的 delete_policy %q 无效（keep|prune|prune-expired）", r.Name, r.Sync.DeletePolicy)
		}
		if !r.Sync.Enabled && len(r.Input.Packages) == 0 &&
			len(r.Input.PackageDirs) == 0 && len(r.Input.UpgradePackages) == 0 {
			return fmt.Errorf("repository %q 未启用 sync 也未配置 make.packages / input，无事可做", r.Name)
		}
		for _, v := range r.Upstream.Vars {
			if v.Name == "" {
				return fmt.Errorf("repository %q 存在未命名变量", r.Name)
			}
		}
		for _, s := range r.Upstream.Sources {
			for _, v := range s.Vars {
				if v.Name == "" {
					return fmt.Errorf("repository %q 存在未命名变量", r.Name)
				}
			}
		}
	}
	return nil
}

// ---- variable expansion ----

var placeholderRe = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}|\$([A-Za-z0-9_]+)`)

// Expand iterates a repository across its multi-valued variables, returning one
// Expanded variant per combination. Single values produce a single variant. URLs
// are substituted with the resolved variable values. When upstream.sources is
// set, each source is expanded into the same variant (aggregate repo).
func Expand(cfg *Config, r *Repository) ([]Expanded, error) {
	sources := r.Upstream.Sources
	if len(sources) == 0 {
		sources = []Source{{URL: r.Upstream.URL, Vars: r.Upstream.Vars, Arch: r.Upstream.Arch, Suites: r.Upstream.Suites}}
	}
	// Merge all vars referenced across any source URL for the cartesian product.
	vars := mergeVars(cfg.Vars, allVars(r))
	keys := make([]string, 0, len(vars))
	for k := range vars {
		if anySourceContains(sources, "$"+k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var result []Expanded
	acc := map[string]string{}
	var walk func(i int)
	walk = func(i int) {
		if i == len(keys) {
			var expanded []SourceURL
			for _, s := range sources {
				out := expandHome(cfg.Paths.HomeDir, s.URL)
				out = substitute(out, acc)
				expanded = append(expanded, SourceURL{URL: out, Arch: s.Arch, Suites: s.Suites})
			}
			result = append(result, Expanded{
				Repository: r,
				URL:        expanded[0].URL,
				Sources:    expanded,
				Vars:       copyMap(acc),
			})
			return
		}
		k := keys[i]
		for _, v := range vars[k] {
			acc[k] = v
			walk(i + 1)
		}
		delete(acc, k)
	}
	walk(0)
	return result, nil
}

func allVars(r *Repository) []Var {
	if len(r.Upstream.Sources) == 0 {
		return r.Upstream.Vars
	}
	var out []Var
	for _, s := range r.Upstream.Sources {
		out = append(out, s.Vars...)
	}
	return out
}

func anySourceContains(sources []Source, needle string) bool {
	for _, s := range sources {
		if strings.Contains(s.URL, needle) {
			return true
		}
	}
	return false
}

// SourceURL is one resolved aggregate source.
type SourceURL struct {
	URL    string
	Arch   []string
	Suites []Suite
}

// Expanded is one resolved variant of a repository.
type Expanded struct {
	Repository *Repository
	URL        string      // primary URL (first source) for display/back-compat
	Sources    []SourceURL // all resolved source URLs (>=1)
	Vars       map[string]string
}

// ContentRoot returns the output directory for this expanded variant. When the
// repository has more than one variant (multiple multi-valued variables), the
// variant's values are appended as subdirectories.
func (e *Expanded) ContentRoot(cfg *Config) string {
	base := cfg.ContentRoot(e.Repository)
	// Collect the multi-valued keys relevant to layout. We use the variables
	// that vary across variants. For simplicity, append the value of any
	// variable that was expanded to more than one value.
	multi := multiValuedKeys(cfg, e.Repository)
	if len(multi) == 0 {
		return base
	}
	var parts []string
	for _, k := range multi {
		if v, ok := e.Vars[k]; ok {
			parts = append(parts, v)
		}
	}
	return filepath.Join(append([]string{base}, parts...)...)
}

func multiValuedKeys(cfg *Config, r *Repository) []string {
	vars := mergeVars(cfg.Vars, r.Upstream.Vars)
	var keys []string
	for k, vs := range vars {
		vals := filterVals(vs)
		if len(vals) > 1 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func filterVals(vs []string) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func mergeVars(global VarMap, local []Var) map[string][]string {
	merged := make(map[string][]string, len(global))
	for k, v := range global {
		merged[k] = append([]string{}, v...)
	}
	for _, lv := range local {
		if lv.Value != "" {
			merged[lv.Name] = []string{lv.Value}
		} else if len(lv.Values) > 0 {
			merged[lv.Name] = append([]string{}, lv.Values...)
		}
	}
	return merged
}

func substitute(s string, vars map[string]string) string {
	return placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
		name := ""
		if strings.HasPrefix(m, "${") {
			name = m[2 : len(m)-1]
		} else {
			name = m[1:]
		}
		if v, ok := vars[name]; ok {
			return v
		}
		return m
	})
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
