// Package converter turns a docker-compose spec into Kubernetes manifests.
package converter

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// Compose is the subset of the compose spec we understand.
type Compose struct {
	Version  string             `yaml:"version"`
	Services map[string]Service `yaml:"services"`
	// Everything below top-level services that we don't model yet.
	Networks map[string]any `yaml:"networks"`
	Volumes  map[string]any `yaml:"volumes"`
}

// Service is a single compose service.
type Service struct {
	Image       string        `yaml:"image"`
	Command     StringOrSlice `yaml:"command"`
	Ports       []string      `yaml:"ports"`
	Environment Environment   `yaml:"environment"`
	Volumes     []string      `yaml:"volumes"`
	Restart     string        `yaml:"restart"`
	Replicas    *int          `yaml:"-"`
	// keys we explicitly recognise as unsupported for a warning
	Build     any `yaml:"build"`
	DependsOn any `yaml:"depends_on"`
	Deploy    any `yaml:"deploy"`
}

// Parse reads a compose YAML document.
func Parse(data []byte) (*Compose, error) {
	var c Compose
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse compose: %w", err)
	}
	if len(c.Services) == 0 {
		return nil, fmt.Errorf("no services found in compose file")
	}
	return &c, nil
}

// ServiceNames returns service names in deterministic order.
func (c *Compose) ServiceNames() []string {
	names := make([]string, 0, len(c.Services))
	for n := range c.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// StringOrSlice accepts either a scalar string or a YAML sequence.
type StringOrSlice []string

func (s *StringOrSlice) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*s = StringOrSlice{value.Value}
	case yaml.SequenceNode:
		var out []string
		if err := value.Decode(&out); err != nil {
			return err
		}
		*s = out
	default:
		return fmt.Errorf("command must be a string or list")
	}
	return nil
}

// Environment accepts either a map (KEY: value) or a list (KEY=value) form.
type Environment map[string]string

func (e *Environment) UnmarshalYAML(value *yaml.Node) error {
	out := Environment{}
	switch value.Kind {
	case yaml.MappingNode:
		// Content is a flat [key, value, key, value, ...] sequence of nodes.
		for i := 0; i+1 < len(value.Content); i += 2 {
			out[value.Content[i].Value] = value.Content[i+1].Value
		}
	case yaml.SequenceNode:
		var items []string
		if err := value.Decode(&items); err != nil {
			return err
		}
		for _, it := range items {
			k, v := splitEnv(it)
			out[k] = v
		}
	default:
		return fmt.Errorf("environment must be a map or list")
	}
	*e = out
	return nil
}

func splitEnv(s string) (string, string) {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}
