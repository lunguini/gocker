package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents ~/.gocker/config.yaml.
type Config struct {
	Isolation string        `yaml:"isolation"` // full, hybrid, shared
	SharedVM  SharedVM      `yaml:"sharedVM,omitempty"`
	Compose   Subsystem     `yaml:"compose,omitempty"`
	Sandbox   SandboxConfig `yaml:"sandbox,omitempty"`
	Runtime   string        `yaml:"runtime,omitempty"`       // "container" or "nerdctl"
	Binary    string        `yaml:"runtimeBinary,omitempty"` // custom path to runtime binary
}

// SharedVM configures the persistent shared VM for hybrid/shared modes.
type SharedVM struct {
	Image         string   `yaml:"image,omitempty"`
	CPUs          int      `yaml:"cpus,omitempty"`
	Memory        string   `yaml:"memory,omitempty"`
	WorkspaceDirs []string `yaml:"workspaceDirs,omitempty"` // host dirs to mount into VM
}

// EffectiveWorkspaceDirs returns WorkspaceDirs or defaults to user home.
func (s *SharedVM) EffectiveWorkspaceDirs() []string {
	if len(s.WorkspaceDirs) > 0 {
		return s.WorkspaceDirs
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		return []string{home}
	}
	return nil
}

// Subsystem allows per-subsystem isolation override.
type Subsystem struct {
	Isolation string `yaml:"isolation,omitempty"`
}

// SandboxConfig extends Subsystem with sandbox-specific settings.
type SandboxConfig struct {
	Isolation         string `yaml:"isolation,omitempty"`
	SyncClaudeSession *bool  `yaml:"syncClaudeSession,omitempty"` // sync Claude Code sessions between host and sandbox (default: true)
}

// Defaults returns a Config with sensible defaults.
func Defaults() *Config {
	return &Config{
		Isolation: "full",
		SharedVM: SharedVM{
			Image:  "docker.io/adyjay/gocker:base-latest",
			CPUs:   4,
			Memory: "4G",
		},
		Sandbox: SandboxConfig{
			Isolation: "full",
		},
	}
}

// Load reads ~/.gocker/config.yaml. Returns defaults if the file doesn't exist.
func Load() *Config {
	cfg := Defaults()

	path := configPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}

	_ = yaml.Unmarshal(data, cfg)
	return cfg
}

// IsolationFor returns the effective isolation mode for a subsystem.
// Priority: CLI flag > subsystem config > global config > "full".
func (c *Config) IsolationFor(subsystem, cliOverride string) string {
	if cliOverride != "" {
		return cliOverride
	}
	switch subsystem {
	case "compose":
		if c.Compose.Isolation != "" {
			return c.Compose.Isolation
		}
	case "sandbox":
		if c.Sandbox.Isolation != "" {
			return c.Sandbox.Isolation
		}
	}
	if c.Isolation != "" {
		return c.Isolation
	}
	return "full"
}

// SyncClaudeSession returns whether Claude Code sessions should be synced between host and sandbox.
func (c *Config) SyncClaudeSession() bool {
	if c.Sandbox.SyncClaudeSession != nil {
		return *c.Sandbox.SyncClaudeSession
	}
	return true // default: enabled
}

// RuntimeBinary returns the path to the container runtime binary.
func (c *Config) RuntimeBinary() string {
	if c.Binary != "" {
		return c.Binary
	}
	return ""
}

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gocker", "config.yaml")
}
