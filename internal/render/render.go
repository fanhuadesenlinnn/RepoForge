package render

import (
	"bytes"
	"fmt"
	"os"
	"text/template"
)

// File renders a Go text template from disk.
func File(path string, data any) ([]byte, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取模板 %s 失败: %w", path, err)
	}
	parsed, err := template.New("repoforge").Option("missingkey=error").Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("解析模板 %s 失败: %w", path, err)
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, data); err != nil {
		return nil, fmt.Errorf("渲染模板 %s 失败: %w", path, err)
	}
	return output.Bytes(), nil
}
