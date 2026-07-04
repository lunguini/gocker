package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"

	"github.com/lunguini/gocker/internal/fsutil"
)

// cleanSemverTag matches exactly vX.Y.Z with no pre-release / build suffix.
// git describe on a commit ahead of the tag appends -<n>-g<sha>; on a dirty
// tree, -dirty — both fail this match and are treated as dev builds.
var cleanSemverTag = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// IsDevVersion reports whether the given ldflags-injected version string
// represents a non-release build. Used to pick :base-dev over :base-latest
// by default so `go install @main` gets a matching in-VM gocker binary.
func IsDevVersion(v string) bool {
	if v == "" || v == "dev" {
		return true
	}
	return !cleanSemverTag.MatchString(v)
}

// Config represents ~/.gocker/config.yaml.
type Config struct {
	Isolation string        `yaml:"isolation"` // full, hybrid, shared
	SharedVM  SharedVM      `yaml:"sharedVM,omitempty"`
	Compose   Subsystem     `yaml:"compose,omitempty"`
	Sandbox   SandboxConfig `yaml:"sandbox,omitempty"`
	Runtime   string        `yaml:"runtime,omitempty"`       // "container" or "nerdctl"
	Binary    string        `yaml:"runtimeBinary,omitempty"` // custom path to runtime binary

	// LegacyWorkspaceDirs accepts top-level `workspaceDirs:` for back-compat.
	// Old configs (and hand-edited ones) placed it here instead of under
	// sharedVM; Load() migrates it into SharedVM.WorkspaceDirs.
	LegacyWorkspaceDirs []string `yaml:"workspaceDirs,omitempty"`
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

// Load reads ~/.gocker/config.yaml. Returns defaults if the file doesn't
// exist. A malformed file falls back to defaults with a warning on stderr —
// hard-failing here would break every command (including the docker alias)
// over a config typo.
func Load() *Config {
	path := configPath()
	cfg, err := LoadFrom(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: ignoring invalid config %s: %v (using defaults: isolation=%s)\n", path, err, cfg.Isolation)
	}
	return cfg
}

// LoadFrom reads a config file from an explicit path. A missing file returns
// defaults with no error; a malformed file returns defaults alongside the
// parse error.
func LoadFrom(path string) (*Config, error) {
	cfg := Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, nil
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return Defaults(), fmt.Errorf("parsing YAML: %w", err)
	}

	// Migrate top-level `workspaceDirs` into sharedVM.workspaceDirs.
	if len(cfg.LegacyWorkspaceDirs) > 0 && len(cfg.SharedVM.WorkspaceDirs) == 0 {
		cfg.SharedVM.WorkspaceDirs = cfg.LegacyWorkspaceDirs
	}
	cfg.LegacyWorkspaceDirs = nil

	return cfg, nil
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
	return filepath.Join(fsutil.HomeDir(), ".gocker", "config.yaml")
}

// Path returns the location gocker reads its config from (~/.gocker/config.yaml).
func Path() string {
	return configPath()
}
