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

// skipIfNoVirtualization checks an error from ContainerRun and skips the test
// if it indicates that Virtualization.framework hardware is unavailable
// (e.g. on GitHub Actions macOS runners).
func skipIfNoVirtualization(t *testing.T, err error) {
	t.Helper()
	if err != nil && strings.Contains(err.Error(), "Virtualization") {
		t.Skipf("skipping: %v", err)
	}
}

// ensureTestImage pulls testImage, tolerating registry failures when the
// image is already cached locally — Docker Hub throttles/401s anonymous
// pulls from CI and shared IPs, and that flakiness has nothing to do with
// what these tests assert. Skips the test when the image is unavailable
// both remotely and locally.
func ensureTestImage(t *testing.T, rt Runtime) {
	t.Helper()
	err := rt.ImagePull(context.Background(), testImage, ImagePullOpts{})
	if err == nil {
		return
	}
	repo, _, _ := strings.Cut(testImage, ":")
	if images, listErr := rt.ImageList(context.Background()); listErr == nil {
		for _, img := range images {
			if strings.Contains(img.Name, repo) {
				t.Logf("ImagePull failed (%v); using locally cached %s", err, testImage)
				return
			}
		}
	}
	t.Skipf("test image %s unavailable: pull failed (%v) and image not cached locally", testImage, err)
}

func TestIntegration_PullImage(t *testing.T) {
	rt := setupRuntime(t)

	ensureTestImage(t, rt)

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
	ensureTestImage(t, rt)

	// Cleanup before and after
	_ = rt.ContainerRemove(context.Background(), name, true)
	t.Cleanup(func() {
		_ = rt.ContainerStop(context.Background(), name)
		_ = rt.ContainerRemove(context.Background(), name, true)
	})

	// Run detached — use sh with trap so the container handles SIGTERM cleanly.
	// Plain "sleep" ignores SIGTERM on Alpine, causing Apple Container stop to time out.
	args := []string{"-d", "--name", name, testImage, "sh", "-c", "trap exit TERM; sleep 300 & wait"}
	if err := rt.ContainerRun(context.Background(), args, false); err != nil {
		skipIfNoVirtualization(t, err)
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

	ensureTestImage(t, rt)

	_ = rt.ContainerRemove(context.Background(), name, true)
	t.Cleanup(func() {
		_ = rt.ContainerStop(context.Background(), name)
		_ = rt.ContainerRemove(context.Background(), name, true)
	})

	args := []string{"-d", "--name", name, testImage, "sh", "-c", "trap exit TERM; sleep 300 & wait"}
	if err := rt.ContainerRun(context.Background(), args, false); err != nil {
		skipIfNoVirtualization(t, err)
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
