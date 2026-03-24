package sharedvm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const vmName = "gocker-shared"

// VMState tracks the persistent shared VM.
type VMState struct {
	Name    string            `json:"name"`
	Status  string            `json:"status"` // running, stopped
	Image   string            `json:"image"`
	Created time.Time         `json:"created"`
	Mounts  map[string]string `json:"mounts"` // host path -> VM path
}

func stateDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gocker", "sharedvm")
}

func statePath() string {
	return filepath.Join(stateDir(), "state.json")
}

func SaveVMState(s *VMState) error {
	dir := stateDir()
	os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(), data, 0644)
}

func LoadVMState() (*VMState, error) {
	data, err := os.ReadFile(statePath())
	if err != nil {
		return nil, err
	}
	var s VMState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func DeleteVMState() error {
	return os.RemoveAll(stateDir())
}
