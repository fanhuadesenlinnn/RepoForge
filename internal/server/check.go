package server

import (
	"fmt"
	"net"
	"net/http"
	"path"
	"time"

	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
)

// CheckRepo probes the local HTTP endpoint for one repository.
func CheckRepo(serverConfig repo.Server, name, backend string) error {
	_, port, err := net.SplitHostPort(serverConfig.Listen)
	if err != nil {
		return fmt.Errorf("server.listen 无效 %q: %w", serverConfig.Listen, err)
	}
	index := "Packages"
	if backend == "rpm" {
		index = "repodata/repomd.xml"
	}
	target := "http://" + net.JoinHostPort("127.0.0.1", port) + path.Join("/", name, index)
	request, err := http.NewRequest(http.MethodHead, target, nil)
	if err != nil {
		return fmt.Errorf("创建 HTTP 检查请求失败: %w", err)
	}
	response, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
	if err != nil {
		return fmt.Errorf("HTTP 软件源不可访问 %s: %w", target, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP 软件源返回状态 %s: %s", response.Status, target)
	}
	return nil
}
