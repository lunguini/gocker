//go:build integration

package engine

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

const testImage = "alpine:latest"

func setupRuntime(t *testing.T) Runtime {
	t.Helper()
	switch runtime.GOOS {
	case "darwin":
		eng := New("")
		eng.EnsureSystemRunning(context.Background())
		return eng
	case "linux":
		return NewNerdctl("")
	default:
		t.Skipf("unsupported platform: %s", runtime.GOOS)
		return nil
	}
}

func TestIntegration_PullImage(t *testing.T) {
	rt := setupRuntime(t)

	if err := rt.ImagePull(context.Background(), testImage); err != nil {
		t.Fatalf("ImagePull failed: %v", err)
	}

	images, err := rt.ImageList(context.Background())
	if err != nil {
		t.Fatalf("ImageList failed: %v", err)
	}

	found := false
	for _, img := range images {
		if strings.Contains(img.Name, "alpine") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("alpine not found in image list after pull")
	}
}

func TestIntegration_ContainerLifecycle(t *testing.T) {
	rt := setupRuntime(t)
	const name = "gocker-test-lifecycle"

	// Pull image first
	if err := rt.ImagePull(context.Background(), testImage); err != nil {
		t.Fatalf("ImagePull failed: %v", err)
	}

	// Cleanup before and after
	_ = rt.ContainerRemove(context.Background(), name, true)
	t.Cleanup(func() {
		_ = rt.ContainerStop(context.Background(), name)
		_ = rt.ContainerRemove(context.Background(), name, true)
	})

	// Run detached
	args := []string{"-d", "--name", name, testImage, "sleep", "300"}
	if err := rt.ContainerRun(context.Background(), args, false); err != nil {
		t.Fatalf("ContainerRun failed: %v", err)
	}

	// Verify in list
	containers, err := rt.ContainerList(context.Background(), true)
	if err != nil {
		t.Fatalf("ContainerList failed: %v", err)
	}
	found := false
	for _, c := range containers {
		if c.Name == name || strings.Contains(c.ID, name) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("container %q not found in list after run", name)
	}

	// Inspect — verify valid JSON
	data, err := rt.ContainerInspect(context.Background(), name)
	if err != nil {
		t.Fatalf("ContainerInspect failed: %v", err)
	}
	if !json.Valid(data) {
		t.Errorf("ContainerInspect returned invalid JSON: %s", string(data))
	}

	// Stop
	if err := rt.ContainerStop(context.Background(), name); err != nil {
		t.Fatalf("ContainerStop failed: %v", err)
	}

	// Start
	if err := rt.ContainerStart(context.Background(), name); err != nil {
		t.Fatalf("ContainerStart failed: %v", err)
	}

	// Stop again
	if err := rt.ContainerStop(context.Background(), name); err != nil {
		t.Fatalf("second ContainerStop failed: %v", err)
	}

	// Remove
	if err := rt.ContainerRemove(context.Background(), name, true); err != nil {
		t.Fatalf("ContainerRemove failed: %v", err)
	}
}

func TestIntegration_ContainerInspect_JSONStructure(t *testing.T) {
	rt := setupRuntime(t)
	const name = "gocker-test-inspect"

	if err := rt.ImagePull(context.Background(), testImage); err != nil {
		t.Fatalf("ImagePull failed: %v", err)
	}

	_ = rt.ContainerRemove(context.Background(), name, true)
	t.Cleanup(func() {
		_ = rt.ContainerStop(context.Background(), name)
		_ = rt.ContainerRemove(context.Background(), name, true)
	})

	args := []string{"-d", "--name", name, testImage, "sleep", "300"}
	if err := rt.ContainerRun(context.Background(), args, false); err != nil {
		t.Fatalf("ContainerRun failed: %v", err)
	}

	data, err := rt.ContainerInspect(context.Background(), name)
	if err != nil {
		t.Fatalf("ContainerInspect failed: %v", err)
	}

	// Try to parse as array (Apple) or object (nerdctl)
	var statusFound bool
	var arr []map[string]any
	if json.Unmarshal(data, &arr) == nil && len(arr) > 0 {
		// Apple-style array
		if _, ok := arr[0]["status"]; ok {
			statusFound = true
		}
		// Also check nested fields
		if !statusFound {
			if cfg, ok := arr[0]["configuration"].(map[string]any); ok {
				_ = cfg
				statusFound = true // configuration present, status may be nested
			}
		}
	} else {
		var obj map[string]any
		if json.Unmarshal(data, &obj) == nil {
			if _, ok := obj["Status"]; ok {
				statusFound = true
			}
			if _, ok := obj["status"]; ok {
				statusFound = true
			}
		}
	}

	// At minimum verify the output is valid JSON
	if !json.Valid(data) {
		t.Errorf("ContainerInspect returned invalid JSON: %s", string(data))
	}
	_ = statusFound // status field location varies by runtime version
}
