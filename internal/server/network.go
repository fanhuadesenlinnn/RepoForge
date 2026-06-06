package server

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"

	"github.com/fanhuadesenlinnn/RepoForge/internal/config"
)

// ResolvePublicURL returns the configured or detected LAN URL and all IP candidates.
func ResolvePublicURL(cfg config.ServerConfig) (string, []string, error) {
	if cfg.PublicURL != "" && cfg.PublicURL != "auto" {
		parsed, err := url.ParseRequestURI(cfg.PublicURL)
		if err != nil {
			return "", nil, fmt.Errorf("server.public_url 无效: %w", err)
		}
		if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return "", nil, fmt.Errorf("server.public_url 必须包含 http/https 协议和主机名")
		}
		return strings.TrimRight(cfg.PublicURL, "/"), nil, nil
	}
	_, port, err := net.SplitHostPort(cfg.Listen)
	if err != nil {
		return "", nil, fmt.Errorf("server.listen 无效 %q: %w", cfg.Listen, err)
	}
	candidates, err := ipv4Candidates()
	if err != nil {
		return "", nil, err
	}
	if len(candidates) == 0 {
		return "", nil, fmt.Errorf("无法判断局域网 IPv4 地址；请手动配置 server.public_url")
	}
	selected := defaultRouteIPv4()
	if !contains(candidates, selected) {
		selected = candidates[0]
	}
	return "http://" + net.JoinHostPort(selected, port), candidates, nil
}

func ipv4Candidates() ([]string, error) {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil, fmt.Errorf("读取本机网络地址失败: %w", err)
	}
	seen := make(map[string]struct{})
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err != nil || ip.IsLoopback() || ip.To4() == nil {
			continue
		}
		seen[ip.String()] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func defaultRouteIPv4() string {
	connection, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer connection.Close()
	address, ok := connection.LocalAddr().(*net.UDPAddr)
	if !ok || address.IP.To4() == nil {
		return ""
	}
	return address.IP.String()
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
