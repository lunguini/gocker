package sandbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/lunguini/gocker/internal/fsutil"
)

type SandboxState struct {
	Name          string    `json:"name"`
	Agent         string    `json:"agent"`
	Workspace     string    `json:"workspace"`
	ContainerID   string    `json:"container_id"`
	Status        string    `json:"status"` // running, stopped
	Created       time.Time `json:"created"`
	NetworkPolicy string    `json:"network_policy"`
	AllowedHosts  []string  `json:"allowed_hosts,omitempty"`
	ContainerIP   string    `json:"container_ip,omitempty"`
}

func sandboxDir() string {
	return filepath.Join(fsutil.HomeDir(), ".gocker", "sandboxes")
}

func statePath(name string) string {
	return filepath.Join(sandboxDir(), name+".json")
}

func SaveState(s *SandboxState) error {
	dir := sandboxDir()
	_ = os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(statePath(s.Name), data, 0644)
}

func LoadState(name string) (*SandboxState, error) {
	data, err := os.ReadFile(statePath(name))
	if err != nil {
		return nil, err
	}
	var s SandboxState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func ListStates() ([]*SandboxState, error) {
	dir := sandboxDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var states []*SandboxState
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		name := e.Name()[:len(e.Name())-5]
		s, err := LoadState(name)
		if err != nil {
			continue
		}
		states = append(states, s)
	}
	return states, nil
}

func DeleteState(name string) error {
	return os.Remove(statePath(name))
}
