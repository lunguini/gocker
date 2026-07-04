package sharedvm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/lunguini/gocker/internal/fsutil"
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

var stateDir = func() string {
	return filepath.Join(fsutil.HomeDir(), ".gocker", "sharedvm")
}

var statePath = func() string {
	return filepath.Join(stateDir(), "state.json")
}

func lockPath() string {
	return filepath.Join(stateDir(), ".lock")
}

func SaveVMState(s *VMState) error {
	return fsutil.WithLock(lockPath(), func() error {
		dir := stateDir()
		_ = os.MkdirAll(dir, 0755)
		data, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return err
		}
		return fsutil.WriteFileAtomic(statePath(), data, 0644)
	})
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
	return fsutil.WithLock(lockPath(), func() error {
		return os.RemoveAll(stateDir())
	})
}
