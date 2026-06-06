package detect

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OS describes the Linux distribution reported by os-release.
type OS struct {
	ID              string
	IDLike          []string
	VersionID       string
	VersionCodename string
	PrettyName      string
	KylinRelease    string
}

// ReadOS reads Linux release information below root.
func ReadOS(root string) (OS, error) {
	values, err := parseKeyValueFile(filepath.Join(root, "etc", "os-release"))
	if err != nil {
		return OS{}, fmt.Errorf("读取系统信息失败: %w", err)
	}
	system := OS{
		ID:              strings.ToLower(values["ID"]),
		IDLike:          strings.Fields(strings.ToLower(values["ID_LIKE"])),
		VersionID:       values["VERSION_ID"],
		VersionCodename: values["VERSION_CODENAME"],
		PrettyName:      values["PRETTY_NAME"],
	}
	if data, readErr := os.ReadFile(filepath.Join(root, "etc", "kylin-release")); readErr == nil {
		system.KylinRelease = strings.TrimSpace(string(data))
	}
	return system, nil
}

// Backend returns rpm, deb, or unknown from Linux release information.
func (system OS) Backend(root string) string {
	if containsAny(append([]string{system.ID}, system.IDLike...), "rhel", "fedora", "centos", "rocky", "almalinux", "kylin", "openeuler") {
		return "rpm"
	}
	if _, err := os.Stat(filepath.Join(root, "etc", "debian_version")); err == nil {
		return "deb"
	}
	if containsAny(append([]string{system.ID}, system.IDLike...), "debian", "ubuntu") {
		return "deb"
	}
	return "unknown"
}

func parseKeyValueFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func containsAny(values []string, candidates ...string) bool {
	for _, value := range values {
		for _, candidate := range candidates {
			if value == candidate {
				return true
			}
		}
	}
	return false
}
