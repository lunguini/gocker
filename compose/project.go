package compose

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func composeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gocker", "compose")
}

func projectDir(name string) string {
	return filepath.Join(composeDir(), name)
}

func statePath(name string) string {
	return filepath.Join(projectDir(name), "state.json")
}

func SaveProject(p *ProjectState) error {
	dir := projectDir(p.Name)
	os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(p.Name), data, 0644)
}

func LoadProject(name string) (*ProjectState, error) {
	data, err := os.ReadFile(statePath(name))
	if err != nil {
		return nil, err
	}
	var p ProjectState
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func DeleteProject(name string) error {
	return os.RemoveAll(projectDir(name))
}
