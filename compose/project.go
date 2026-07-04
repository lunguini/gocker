package compose

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/lunguini/gocker/internal/fsutil"
)

func composeDir() string {
	return filepath.Join(fsutil.HomeDir(), ".gocker", "compose")
}

func projectDir(name string) string {
	return filepath.Join(composeDir(), name)
}

func statePath(name string) string {
	return filepath.Join(projectDir(name), "state.json")
}

func lockPath() string {
	return filepath.Join(fsutil.HomeDir(), ".gocker", "compose.lock")
}

func SaveProject(p *ProjectState) error {
	return fsutil.WithLock(lockPath(), func() error {
		dir := projectDir(p.Name)
		_ = os.MkdirAll(dir, 0755)
		data, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return err
		}
		return fsutil.WriteFileAtomic(statePath(p.Name), data, 0644)
	})
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
	return fsutil.WithLock(lockPath(), func() error {
		return os.RemoveAll(projectDir(name))
	})
}
