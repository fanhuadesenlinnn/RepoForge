// Package render renders Go text templates from strings or files.
package render

import (
	"bytes"
	"fmt"
	"text/template"
)

// Text renders a Go text template from a string.
func Text(source string, data any) ([]byte, error) {
	parsed, err := template.New("repoforge").Option("missingkey=error").Parse(source)
	if err != nil {
		return nil, fmt.Errorf("解析模板失败: %w", err)
	}
	var output bytes.Buffer
	if err := parsed.Execute(&output, data); err != nil {
		return nil, fmt.Errorf("渲染模板失败: %w", err)
	}
	return output.Bytes(), nil
}
