package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Save writes cfg to ~/.gocker/config.yaml, creating the directory if needed.
func Save(cfg *Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home: %w", err)
	}
	dir := filepath.Join(home, ".gocker")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
