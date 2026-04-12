package compose

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// BuildConfig handles both string and object forms for build directives.
//
//	build: ./dir                        → {Context: "./dir"}
//	build:
//	  context: ./dir
//	  dockerfile: Dockerfile.custom     → {Context: "./dir", Dockerfile: "Dockerfile.custom"}
type BuildConfig struct {
	Context    string `yaml:"context"`
	Dockerfile string `yaml:"dockerfile,omitempty"`
}

func (b *BuildConfig) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		b.Context = value.Value
		return nil
	case yaml.MappingNode:
		// Decode into a temporary struct to avoid infinite recursion.
		var raw struct {
			Context    string `yaml:"context"`
			Dockerfile string `yaml:"dockerfile"`
		}
		if err := value.Decode(&raw); err != nil {
			return err
		}
		b.Context = raw.Context
		b.Dockerfile = raw.Dockerfile
		return nil
	default:
		return fmt.Errorf("build must be a string or map, got %v", value.Kind)
	}
}

// IsSet returns true if a build context was specified.
func (b BuildConfig) IsSet() bool {
	return b.Context != ""
}

// CommandOrArgs handles both string and []string forms for command/entrypoint.
//
//	command: "echo hello"       → ["echo", "hello"]
//	command: ["echo", "hello"]  → ["echo", "hello"]
type CommandOrArgs []string

func (c *CommandOrArgs) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*c = strings.Fields(value.Value)
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		*c = list
		return nil
	default:
		return fmt.Errorf("command must be a string or list, got %v", value.Kind)
	}
}

// Environment handles both map and list forms.
//
//	environment:
//	  FOO: bar        → {"FOO": "bar"}
//	environment:
//	  - FOO=bar       → {"FOO": "bar"}
//	  - BAZ           → {"BAZ": ""} (inherit from host)
type Environment map[string]string

func (e *Environment) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.MappingNode:
		m := make(map[string]string)
		if err := value.Decode(&m); err != nil {
			return err
		}
		*e = m
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		m := make(map[string]string)
		for _, item := range list {
			k, v, _ := strings.Cut(item, "=")
			m[k] = v
		}
		*e = m
		return nil
	default:
		return fmt.Errorf("environment must be a map or list, got %v", value.Kind)
	}
}

// DependsOn handles both list and map forms.
//
//	depends_on: [db, redis]
//	depends_on:
//	  db:
//	    condition: service_started
type DependsOn []string

func (d *DependsOn) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		*d = list
		return nil
	case yaml.MappingNode:
		// Map form: extract just the keys (service names)
		var m map[string]any
		if err := value.Decode(&m); err != nil {
			return err
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		*d = keys
		return nil
	default:
		return fmt.Errorf("depends_on must be a list or map, got %v", value.Kind)
	}
}

// StringOrSlice handles fields that accept both a single string and a list.
//
//	env_file: .env
//	env_file:
//	  - .env
//	  - .env.local
type StringOrSlice []string

func (s *StringOrSlice) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*s = []string{value.Value}
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		*s = list
		return nil
	default:
		return fmt.Errorf("value must be a string or list, got %v", value.Kind)
	}
}
