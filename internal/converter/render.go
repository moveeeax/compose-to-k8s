package converter

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Name returns the metadata.name of a rendered object, or "object" if absent.
func Name(obj map[string]any) string {
	if m, ok := obj["metadata"].(map[string]any); ok {
		if n, ok := m["name"].(string); ok {
			return n
		}
	}
	return "object"
}

// Render serialises the result into a single multi-document YAML stream,
// ordered ConfigMaps -> Deployments -> Services for readability.
func Render(res *Result) ([]byte, error) {
	var buf bytes.Buffer
	first := true
	emit := func(obj map[string]any) error {
		if !first {
			buf.WriteString("---\n")
		}
		first = false
		out, err := yaml.Marshal(obj)
		if err != nil {
			return err
		}
		buf.Write(out)
		return nil
	}
	for _, cm := range res.ConfigMaps {
		if err := emit(cm); err != nil {
			return nil, err
		}
	}
	for _, d := range res.Deployments {
		if err := emit(d); err != nil {
			return nil, err
		}
	}
	for _, s := range res.Services {
		if err := emit(s); err != nil {
			return nil, err
		}
	}
	if first {
		return nil, fmt.Errorf("nothing to render")
	}
	return buf.Bytes(), nil
}
