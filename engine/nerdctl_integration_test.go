//go:build integration && linux

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestIntegration_Nerdctl_ContainerLifecycle(t *testing.T) {
	rt := NewNerdctl("")
	ctx := context.Background()
	const name = "gocker-nerdctl-test-lifecycle"

	if err := rt.ImagePull(ctx, testImage, ImagePullOpts{}); err != nil {
		t.Fatalf("ImagePull failed: %v", err)
	}

	_ = rt.ContainerRemove(ctx, name, true)
	t.Cleanup(func() {
		_ = rt.ContainerStop(ctx, name)
		_ = rt.ContainerRemove(ctx, name, true)
	})

	args := []string{"-d", "--name", name, testImage, "sleep", "300"}
	if err := rt.ContainerRun(ctx, args, false); err != nil {
		t.Fatalf("ContainerRun failed: %v", err)
	}

	// Verify in list
	containers, err := rt.ContainerList(ctx, true)
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
	data, err := rt.ContainerInspect(ctx, name)
	if err != nil {
		t.Fatalf("ContainerInspect failed: %v", err)
	}
	if !json.Valid(data) {
		t.Errorf("ContainerInspect returned invalid JSON: %s", string(data))
	}

	// Stop
	if err := rt.ContainerStop(ctx, name); err != nil {
		t.Fatalf("ContainerStop failed: %v", err)
	}

	// Remove
	if err := rt.ContainerRemove(ctx, name, true); err != nil {
		t.Fatalf("ContainerRemove failed: %v", err)
	}
}

func TestIntegration_Nerdctl_ImageList(t *testing.T) {
	rt := NewNerdctl("")
	ctx := context.Background()

	if err := rt.ImagePull(ctx, testImage, ImagePullOpts{}); err != nil {
		t.Fatalf("ImagePull failed: %v", err)
	}

	images, err := rt.ImageList(ctx)
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

func TestIntegration_Nerdctl_VolumeLifecycle(t *testing.T) {
	rt := NewNerdctl("")
	ctx := context.Background()
	name := fmt.Sprintf("gocker-nerdctl-test-vol-%d", 1)

	_ = rt.VolumeRemove(ctx, name)
	t.Cleanup(func() {
		_ = rt.VolumeRemove(ctx, name)
	})

	if err := rt.VolumeCreate(ctx, name); err != nil {
		t.Fatalf("VolumeCreate failed: %v", err)
	}

	volumes, err := rt.VolumeList(ctx)
	if err != nil {
		t.Fatalf("VolumeList failed: %v", err)
	}

	found := false
	for _, v := range volumes {
		if v.Name == name {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("volume %q not found in list after create", name)
	}

	if err := rt.VolumeRemove(ctx, name); err != nil {
		t.Fatalf("VolumeRemove failed: %v", err)
	}
}

func TestIntegration_Nerdctl_NetworkLifecycle(t *testing.T) {
	rt := NewNerdctl("")
	ctx := context.Background()
	const name = "gocker-nerdctl-test-net"

	_ = rt.NetworkRemove(ctx, name)
	t.Cleanup(func() {
		_ = rt.NetworkRemove(ctx, name)
	})

	if err := rt.NetworkCreate(ctx, name); err != nil {
		t.Fatalf("NetworkCreate failed: %v", err)
	}

	networks, err := rt.NetworkList(ctx)
	if err != nil {
		t.Fatalf("NetworkList failed: %v", err)
	}

	found := false
	for _, n := range networks {
		if n.Name == name {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("network %q not found in list after create", name)
	}

	if err := rt.NetworkRemove(ctx, name); err != nil {
		t.Fatalf("NetworkRemove failed: %v", err)
	}
}
