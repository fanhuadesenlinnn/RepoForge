package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Handler returns a read-only repository file handler.
func Handler(root string, directoryListing bool) (http.Handler, error) {
	resolvedRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("软件源根目录不可用 %s: %w", root, err)
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			response.Header().Set("Allow", "GET, HEAD")
			http.Error(response, "只允许 GET 和 HEAD", http.StatusMethodNotAllowed)
			return
		}
		target, err := secureTarget(resolvedRoot, request.URL.Path)
		if err != nil {
			http.Error(response, "禁止访问", http.StatusForbidden)
			return
		}
		info, err := os.Stat(target)
		if os.IsNotExist(err) {
			http.NotFound(response, request)
			return
		}
		if err != nil {
			http.Error(response, "读取文件失败", http.StatusInternalServerError)
			return
		}
		if info.IsDir() {
			if !directoryListing {
				http.Error(response, "目录浏览已关闭", http.StatusForbidden)
				return
			}
			http.FileServer(http.Dir(resolvedRoot)).ServeHTTP(response, request)
			return
		}
		http.ServeFile(response, request, target)
	}), nil
}

// Serve runs the foreground repository HTTP server until the context is canceled.
func Serve(ctx context.Context, listen, root string, directoryListing bool) error {
	handler, err := Handler(root, directoryListing)
	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Addr:              listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errs := make(chan error, 1)
	go func() {
		errs <- httpServer.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf(`HTTP 服务启动失败，监听地址 %s

解决建议：
1. 请检查端口是否被占用；
2. 可使用 ss -lntp 查看监听端口；
3. 或修改 config/repo.yaml 中的 server.listen: %w`, listen, err)
	}
}

func secureTarget(root, requestPath string) (string, error) {
	decoded, err := url.PathUnescape(requestPath)
	if err != nil || strings.ContainsRune(decoded, '\x00') {
		return "", errors.New("invalid path")
	}
	for _, segment := range strings.Split(strings.ReplaceAll(decoded, "\\", "/"), "/") {
		if segment == ".." {
			return "", errors.New("path traversal")
		}
	}
	relative := strings.TrimPrefix(filepath.Clean(filepath.FromSlash(decoded)), string(filepath.Separator))
	return filepath.Join(root, relative), nil
}
