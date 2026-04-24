package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/lunguini/gocker/engine"
)

// These tests mirror what lazydocker (and anything using the Docker Go SDK)
// asks gocker for. They decode every response into the real SDK struct, so a
// silent field-name drift or wrong JSON type fails here — catching the class
// of bug where lazydocker just shows an empty pane.

// lazydocker polls /containers/json?all=true every tick. Key fields it reads:
// ID, Names, Image, Status, State, Ports (for the ports column). State must
// be one of "running" / "exited" / "paused" / ... — empty string makes the UI
// treat the row as unknown.
func TestLazydocker_ContainerList_Populated(t *testing.T) {
	srv := NewServer(&stubRuntime{
		containerList: func(ctx context.Context, all bool) ([]engine.ContainerInfo, error) {
			return []engine.ContainerInfo{
				{ID: "c1", Name: "web", Image: "nginx:alpine", Status: "Up 2 minutes", Command: "nginx -g", Ports: "0.0.0.0:8080->80/tcp", IP: "10.0.0.5", Created: time.Now().Add(-2 * time.Minute)},
				{ID: "c2", Name: "db", Image: "postgres:16", Status: "Exited (0) 10 seconds ago", Command: "postgres", Created: time.Now().Add(-5 * time.Minute)},
				{ID: "c3", Name: "app", Image: "nginx:alpine", Status: "Created", Created: time.Now()},
				{ID: "c4", Name: "paused-svc", Image: "busybox", Status: "Paused", Created: time.Now()},
			}, nil
		},
	}, "")

	rr := doGET(t, srv, "/containers/json?all=1")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	var list []dockertypes.Container
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("docker SDK cannot decode ContainerList: %v\nbody=%s", err, rr.Body.String())
	}
	if len(list) != 4 {
		t.Fatalf("got %d containers, want 4", len(list))
	}

	want := map[string]struct {
		state, status, name, image string
	}{
		"c1": {"running", "Up", "/web", "nginx:alpine"},
		"c2": {"exited", "Exited", "/db", "postgres:16"},
		"c3": {"created", "Created", "/app", "nginx:alpine"},
		"c4": {"paused", "Paused", "/paused-svc", "busybox"},
	}
	for _, got := range list {
		w, ok := want[got.ID]
		if !ok {
			t.Errorf("unexpected container %q", got.ID)
			continue
		}
		if got.State != w.state {
			t.Errorf("%s.State = %q, want %q", got.ID, got.State, w.state)
		}
		if !strings.Contains(got.Status, w.status) {
			t.Errorf("%s.Status = %q, want to contain %q", got.ID, got.Status, w.status)
		}
		if len(got.Names) == 0 || got.Names[0] != w.name {
			t.Errorf("%s.Names = %v, want %q", got.ID, got.Names, w.name)
		}
		if got.Image != w.image {
			t.Errorf("%s.Image = %q, want %q", got.ID, got.Image, w.image)
		}
	}

	// Ports for the nginx container: 0.0.0.0:8080->80/tcp
	for _, c := range list {
		if c.ID != "c1" {
			continue
		}
		if len(c.Ports) != 1 {
			t.Fatalf("c1.Ports = %+v, want 1 entry", c.Ports)
		}
		p := c.Ports[0]
		if p.PrivatePort != 80 || p.PublicPort != 8080 || p.IP != "0.0.0.0" || p.Type != "tcp" {
			t.Errorf("c1.Ports[0] = %+v, want 0.0.0.0:8080->80/tcp", p)
		}
	}
}

// Port-string parsing covers the shapes nerdctl emits: IP:pub->priv/proto,
// range-less published, container-only, and IPv6 duplication.
func TestLazydocker_Ports_WireFormat(t *testing.T) {
	cases := []struct {
		raw                 string
		entries             int
		priv, pub           uint16
		ip, proto           string
		firstOnlyExpectsPub bool
	}{
		{"0.0.0.0:8080->80/tcp", 1, 80, 8080, "0.0.0.0", "tcp", true},
		{"9000/tcp", 1, 9000, 0, "", "tcp", false},
		{"0.0.0.0:5432->5432/tcp,[::]:5432->5432/tcp", 2, 5432, 5432, "0.0.0.0", "tcp", true},
		{"53/udp", 1, 53, 0, "", "udp", false},
		{"", 0, 0, 0, "", "", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.raw, func(t *testing.T) {
			srv := NewServer(&stubRuntime{
				containerList: func(ctx context.Context, all bool) ([]engine.ContainerInfo, error) {
					return []engine.ContainerInfo{{ID: "x", Name: "x", Status: "Up", Ports: tc.raw}}, nil
				},
			}, "")
			rr := doGET(t, srv, "/containers/json?all=1")
			var list []dockertypes.Container
			if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
				t.Fatalf("decode: %v\nbody=%s", err, rr.Body.String())
			}
			if len(list[0].Ports) != tc.entries {
				t.Fatalf("ports=%+v, want %d entries", list[0].Ports, tc.entries)
			}
			if tc.entries == 0 {
				return
			}
			p := list[0].Ports[0]
			if p.PrivatePort != tc.priv || p.IP != tc.ip || p.Type != tc.proto {
				t.Errorf("ports[0] = %+v, want priv=%d ip=%q proto=%q", p, tc.priv, tc.ip, tc.proto)
			}
			if tc.firstOnlyExpectsPub && p.PublicPort != tc.pub {
				t.Errorf("ports[0].PublicPort = %d, want %d", p.PublicPort, tc.pub)
			}
		})
	}
}

// Empty list must serialize as "[]" (not "null") — Docker SDK handles both,
// but some compose versions and lazydocker's shell-outs choke on null.
func TestLazydocker_ContainerList_EmptyIsJSONArray(t *testing.T) {
	srv := NewServer(&stubRuntime{}, "")
	rr := doGET(t, srv, "/containers/json?all=1")
	if body := strings.TrimSpace(rr.Body.String()); body == "null" {
		t.Fatalf("empty list serialized as null; want []")
	}
	var list []dockertypes.Container
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("SDK decode empty: %v", err)
	}
	if list == nil {
		t.Fatal("decoded to nil; lazydocker checks len() which is fine, but some tools compare != nil")
	}
}

// lazydocker's "Images" tab hits /images/json. If Created is the wrong type
// or RepoTags is missing, image rows render blank.
func TestLazydocker_ImageList_Populated(t *testing.T) {
	srv := NewServer(&stubRuntime{
		imageList: func(ctx context.Context) ([]engine.ImageInfo, error) {
			return []engine.ImageInfo{
				{ID: "sha256:abc", Name: "nginx", Tag: "alpine", Created: time.Now()},
				{ID: "sha256:def", Name: "postgres", Tag: "16", Created: time.Now()},
			}, nil
		},
	}, "")
	rr := doGET(t, srv, "/images/json")
	var list []dockerimage.Summary
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("SDK decode ImageList: %v\nbody=%s", err, rr.Body.String())
	}
	if len(list) != 2 {
		t.Fatalf("got %d images, want 2", len(list))
	}
	for _, img := range list {
		if img.ID == "" {
			t.Errorf("image has empty ID: %+v", img)
		}
		if img.Created == 0 {
			t.Errorf("image has zero Created: %+v", img)
		}
	}
}

// /_ping response must carry an API-Version header — the SDK reads it during
// version negotiation. If missing, the SDK falls back to a pinned version
// which may not match the handlers we register.
func TestLazydocker_Ping_IncludesAPIVersionHeader(t *testing.T) {
	srv := NewServer(&stubRuntime{}, "")
	rr := doGET(t, srv, "/_ping")
	if rr.Code != http.StatusOK {
		t.Fatalf("ping status: %d", rr.Code)
	}
	if rr.Header().Get("API-Version") == "" {
		t.Error("/_ping missing API-Version header — SDK version negotiation will not work")
	}
}

// /version must decode into types.Version with real, non-empty fields. We saw
// regressions where a placeholder marshaler left field values blank.
func TestLazydocker_Version_HasRealValues(t *testing.T) {
	srv := NewServer(&stubRuntime{}, "")
	rr := doGET(t, srv, "/version")
	var v dockertypes.Version
	if err := json.Unmarshal(rr.Body.Bytes(), &v); err != nil {
		t.Fatalf("SDK decode Version: %v\nbody=%s", err, rr.Body.String())
	}
	if v.APIVersion == "" || v.Version == "" || v.Os == "" || v.Arch == "" {
		t.Errorf("version has empty fields: %+v", v)
	}
}

// Container inspect is called when a row is focused. If the response isn't a
// single object with a State object, the detail pane stays blank.
func TestLazydocker_ContainerInspect_ShapeIsObjectNotArray(t *testing.T) {
	srv := NewServer(&stubRuntime{
		containerInspect: func(ctx context.Context, nameOrID string) ([]byte, error) {
			return []byte(`[{"id":"c1","status":"running","configuration":{"image":{"reference":"nginx:alpine"}}}]`), nil
		},
	}, "")
	rr := doGET(t, srv, "/containers/c1/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rr.Code, rr.Body.String())
	}
	trimmed := strings.TrimSpace(rr.Body.String())
	if strings.HasPrefix(trimmed, "[") {
		t.Fatalf("inspect returned array; Docker SDK expects an object\nbody=%s", trimmed)
	}
	var info dockertypes.ContainerJSON
	if err := json.Unmarshal(rr.Body.Bytes(), &info); err != nil {
		t.Fatalf("SDK decode Inspect: %v\nbody=%s", err, rr.Body.String())
	}
}
