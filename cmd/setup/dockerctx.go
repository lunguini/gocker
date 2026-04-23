package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type dockerContextEntry struct {
	Name      string
	Endpoints map[string]struct{ Host string }
	Current   bool
}

// parseCurrentContext returns the (name, host) of the context marked Current.
// Returns ("", "", nil) if none is current.
func parseCurrentContext(data []byte) (string, string, error) {
	var entries []dockerContextEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return "", "", fmt.Errorf("parsing docker context output: %w", err)
	}
	for _, e := range entries {
		if e.Current {
			return e.Name, e.Endpoints["docker"].Host, nil
		}
	}
	return "", "", nil
}

// gockerContextIsCurrent returns true if the docker context list output shows
// the gocker context already selected and pointing at socket.
func gockerContextIsCurrent(data []byte, socket string) bool {
	var entries []dockerContextEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return false
	}
	want := "unix://" + socket
	for _, e := range entries {
		if e.Current && strings.EqualFold(e.Name, "gocker") && e.Endpoints["docker"].Host == want {
			return true
		}
	}
	return false
}

// ConfigureDockerContext creates and selects a docker context named "gocker"
// pointing at the given socket. Returns (changed, err). No-op if already set.
func ConfigureDockerContext(ctx context.Context, socket string) (bool, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return false, nil // docker not installed — nothing to do
	}

	listOut, err := exec.CommandContext(ctx, "docker", "context", "ls", "--format", "json").Output()
	if err == nil && gockerContextIsCurrent(listOut, socket) {
		return false, nil
	}

	// Check if "gocker" context exists (regardless of current).
	inspectOut, _ := exec.CommandContext(ctx, "docker", "context", "inspect", "gocker").Output()
	host := "unix://" + socket
	if len(inspectOut) > 0 && strings.Contains(string(inspectOut), host) {
		// exists and points at the right place — just switch to it
		if err := exec.CommandContext(ctx, "docker", "context", "use", "gocker").Run(); err != nil {
			return false, fmt.Errorf("docker context use gocker: %w", err)
		}
		return true, nil
	}

	// Create fresh context (remove any old one first in case the endpoint drifted).
	_ = exec.CommandContext(ctx, "docker", "context", "rm", "gocker").Run()
	if err := exec.CommandContext(ctx, "docker", "context", "create", "gocker",
		"--docker", "host="+host,
		"--description", "gocker daemon socket").Run(); err != nil {
		return false, fmt.Errorf("docker context create: %w", err)
	}
	if err := exec.CommandContext(ctx, "docker", "context", "use", "gocker").Run(); err != nil {
		return false, fmt.Errorf("docker context use gocker: %w", err)
	}
	return true, nil
}
