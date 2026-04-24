package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/lunguini/gocker/engine"
)

func TestIsInUseError(t *testing.T) {
	cases := map[string]bool{
		"":                                         false,
		"some unrelated failure":                   false,
		"network X has active endpoints":           true,
		"Error: image is in use by container":      true,
		"volume is being used by a container":      true,
		"image has dependent child images":         true,
		"ERROR: ... IN USE ...":                    true, // case-insensitive
		// Apple Container CLI's opaque prune-time wrapper — we accept it as
		// a soft skip because the backend was right to refuse, but the
		// message doesn't tell us why.
		`Error: failed to delete one or more networks: ["foo"]: exit status 1`: true,
		// Apple CLI's in-use refusal for a single named network — observed
		// when a container still references a compose project's network.
		`failed to delete network: ["id": proxy_proxy, "error": invalidState: "cannot delete subnet proxy_proxy with referring containers: backend-redis-1"]`: true,
		`delete failed for one or more networks: ["proxy_proxy"]: exit status 1`:                                                                               true,
	}
	for msg, want := range cases {
		var err error
		if msg != "" {
			err = errors.New(msg)
		}
		if got := isInUseError(err); got != want {
			t.Errorf("isInUseError(%q) = %v, want %v", msg, got, want)
		}
	}
}

func TestIsDanglingImage(t *testing.T) {
	cases := []struct {
		img  engine.ImageInfo
		want bool
	}{
		{engine.ImageInfo{Name: "nginx", Tag: "1.25"}, false},
		{engine.ImageInfo{Name: "", Tag: "1.25"}, true},
		{engine.ImageInfo{Name: "nginx", Tag: ""}, true},
		{engine.ImageInfo{Name: "<none>", Tag: "<none>"}, true},
		{engine.ImageInfo{Name: "nginx", Tag: "<none>"}, true},
	}
	for _, tc := range cases {
		if got := isDanglingImage(tc.img); got != tc.want {
			t.Errorf("isDangling(%+v) = %v, want %v", tc.img, got, tc.want)
		}
	}
}

func TestPruneStoppedContainers_SkipsRunning(t *testing.T) {
	var removed []string
	rt := &engine.MockRuntime{
		ContainerListFunc: func(ctx context.Context, all bool) ([]engine.ContainerInfo, error) {
			return []engine.ContainerInfo{
				{ID: "a", Name: "web", State: "running"},
				{ID: "b", Name: "db", State: "exited"},
				{ID: "c", Name: "cache", State: "stopped"},
				{ID: "d", Name: "worker", State: "paused"},
			}, nil
		},
		ContainerRemoveFunc: func(ctx context.Context, id string, force bool) error {
			removed = append(removed, id)
			return nil
		},
	}

	report := pruneStoppedContainers(context.Background(), rt)

	if len(report.errors) != 0 {
		t.Errorf("unexpected errors: %v", report.errors)
	}
	if len(removed) != 2 {
		t.Errorf("removed wrong count: got %v", removed)
	}
	for _, id := range removed {
		if id != "b" && id != "c" {
			t.Errorf("should not have removed %q (only exited/stopped should be removed)", id)
		}
	}
}

func TestPruneImages_DanglingOnly(t *testing.T) {
	rt := &engine.MockRuntime{
		ImageListFunc: func(ctx context.Context) ([]engine.ImageInfo, error) {
			return []engine.ImageInfo{
				{ID: "1", Name: "nginx", Tag: "1.25"},          // not dangling
				{ID: "2", Name: "<none>", Tag: "<none>"},        // dangling
				{ID: "3", Name: "alpine", Tag: "3"},             // not dangling
				{ID: "4", Name: "", Tag: "1.0"},                 // dangling (no repo)
			}, nil
		},
		ImageRemoveFunc: func(ctx context.Context, ref string) error { return nil },
	}

	report := pruneImages(context.Background(), rt, false)

	if len(report.removed) != 2 {
		t.Errorf("removed wrong count: got %d (%v)", len(report.removed), report.removed)
	}
}

func TestPruneImages_AllRemovesEverything(t *testing.T) {
	rt := &engine.MockRuntime{
		ImageListFunc: func(ctx context.Context) ([]engine.ImageInfo, error) {
			return []engine.ImageInfo{
				{ID: "1", Name: "nginx", Tag: "1.25"},
				{ID: "2", Name: "alpine", Tag: "3"},
			}, nil
		},
		ImageRemoveFunc: func(ctx context.Context, ref string) error { return nil },
	}

	report := pruneImages(context.Background(), rt, true)

	if len(report.removed) != 2 {
		t.Errorf("removed wrong count: got %d (%v)", len(report.removed), report.removed)
	}
}

func TestPruneUnusedNetworks_SkipsDefaults(t *testing.T) {
	var removed []string
	rt := &engine.MockRuntime{
		NetworkListFunc: func(ctx context.Context) ([]engine.NetworkInfo, error) {
			return []engine.NetworkInfo{
				{Name: "bridge"},
				{Name: "host"},
				{Name: "my-project"},
				{Name: "another-project"},
			}, nil
		},
		NetworkRemoveFunc: func(ctx context.Context, name string) error {
			removed = append(removed, name)
			return nil
		},
	}

	report := pruneUnusedNetworks(context.Background(), rt)

	if len(report.removed) != 2 {
		t.Errorf("removed %d, want 2: %v", len(report.removed), report.removed)
	}
	for _, n := range removed {
		if n == "bridge" || n == "host" {
			t.Errorf("must not remove default network %q", n)
		}
	}
}

func TestPruneUnusedNetworks_InUseSkippedSilently(t *testing.T) {
	rt := &engine.MockRuntime{
		NetworkListFunc: func(ctx context.Context) ([]engine.NetworkInfo, error) {
			return []engine.NetworkInfo{
				{Name: "active"},
				{Name: "orphan"},
			}, nil
		},
		NetworkRemoveFunc: func(ctx context.Context, name string) error {
			if name == "active" {
				return errors.New("network has active endpoints")
			}
			return nil
		},
	}

	report := pruneUnusedNetworks(context.Background(), rt)

	if len(report.errors) != 0 {
		t.Errorf("in-use error should be silent, got %v", report.errors)
	}
	if len(report.removed) != 1 || report.removed[0] != "orphan" {
		t.Errorf("removed: got %v, want [orphan]", report.removed)
	}
}
