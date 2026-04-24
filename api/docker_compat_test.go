package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	dockerevents "github.com/docker/docker/api/types/events"
	dockerimage "github.com/docker/docker/api/types/image"
	dockervolume "github.com/docker/docker/api/types/volume"

	"github.com/lunguini/gocker/engine"
)

// The tests in this file decode our API inspect responses into the *real*
// Docker SDK types. This guards against the class of bug where the Apple
// container CLI returns payloads (arrays, lowercase field names, int
// timestamps, ...) that the Docker SDK refuses to unmarshal into its strict
// struct types. If a client using the docker SDK can deserialize our response,
// the test passes.

func doGET(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)
	return rr
}

func TestDockerCompat_NetworkInspect_ArrayPayload(t *testing.T) {
	srv := NewServer(&stubRuntime{
		networkInspect: func(ctx context.Context, name string) ([]byte, error) {
			return []byte(`[{"id":"net-abc123","name":"proxy","driver":"bridge","scope":"local"}]`), nil
		},
	}, "")

	rr := doGET(t, srv, "/networks/proxy")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out dockertypes.NetworkResource
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("docker SDK cannot unmarshal NetworkResource: %v\nbody=%s", err, rr.Body.String())
	}
	if out.Name != "proxy" {
		t.Errorf("Name: got %q, want %q", out.Name, "proxy")
	}
	if out.ID != "net-abc123" {
		t.Errorf("ID: got %q, want %q", out.ID, "net-abc123")
	}
	if out.Driver != "bridge" {
		t.Errorf("Driver: got %q, want %q", out.Driver, "bridge")
	}
}

func TestDockerCompat_VolumeInspect_ArrayPayload(t *testing.T) {
	srv := NewServer(&stubRuntime{
		volumeInspect: func(ctx context.Context, name string) ([]byte, error) {
			return []byte(`[{"name":"pgdata","driver":"local","source":"/var/lib/gocker/volumes/pgdata"}]`), nil
		},
	}, "")

	rr := doGET(t, srv, "/volumes/pgdata")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out dockervolume.Volume
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("docker SDK cannot unmarshal volume.Volume: %v\nbody=%s", err, rr.Body.String())
	}
	if out.Name != "pgdata" {
		t.Errorf("Name: got %q, want %q", out.Name, "pgdata")
	}
	if out.Driver != "local" {
		t.Errorf("Driver: got %q, want %q", out.Driver, "local")
	}
	if out.Mountpoint != "/var/lib/gocker/volumes/pgdata" {
		t.Errorf("Mountpoint: got %q", out.Mountpoint)
	}
}

func TestDockerCompat_ContainerInspect_PassesRealFieldsThrough(t *testing.T) {
	// nerdctl inspect emits a Docker-shaped object with State nested, full
	// Config (Env/Labels/Hostname/Image), HostConfig, Mounts, and
	// NetworkSettings. The handler should pass all of that through instead
	// of rebuilding a tiny subset — that's what was breaking lazydocker's
	// "env/config/mounts" views.
	payload := `[{
		"Id": "ctr-xyz",
		"Name": "redis",
		"Image": "redis:7",
		"State": {"Status": "running", "Running": true, "Pid": 42},
		"Config": {
			"Image": "redis:7",
			"Env": ["FOO=bar", "REDIS_PORT=6379"],
			"Labels": {"com.example.app": "cache"},
			"Hostname": "abc123",
			"Cmd": ["redis-server"]
		},
		"HostConfig": {"NetworkMode": "bridge"},
		"NetworkSettings": {"IPAddress": "10.4.0.5", "Ports": {}}
	}]`
	srv := NewServer(&stubRuntime{
		containerInspect: func(ctx context.Context, id string) ([]byte, error) {
			return []byte(payload), nil
		},
	}, "")

	rr := doGET(t, srv, "/containers/ctr-xyz/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out dockertypes.ContainerJSON
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("docker SDK cannot unmarshal ContainerJSON: %v\nbody=%s", err, rr.Body.String())
	}
	if out.ID != "ctr-xyz" {
		t.Errorf("ID: got %q, want %q", out.ID, "ctr-xyz")
	}
	if out.Name != "/redis" {
		t.Errorf("Name: got %q, want leading-slash %q", out.Name, "/redis")
	}
	if out.State == nil || out.State.Status != "running" {
		t.Errorf("State.Status: got %+v, want running", out.State)
	}
	if out.Config == nil {
		t.Fatal("Config is nil — env/labels/cmd all lost")
	}
	if len(out.Config.Env) != 2 || out.Config.Env[0] != "FOO=bar" {
		t.Errorf("Config.Env: got %v, want [FOO=bar REDIS_PORT=6379]", out.Config.Env)
	}
	if out.Config.Labels["com.example.app"] != "cache" {
		t.Errorf("Config.Labels: got %v", out.Config.Labels)
	}
	if len(out.Config.Cmd) == 0 || out.Config.Cmd[0] != "redis-server" {
		t.Errorf("Config.Cmd: got %v", out.Config.Cmd)
	}
	if out.Config.Hostname != "abc123" {
		t.Errorf("Config.Hostname: got %q", out.Config.Hostname)
	}
}

func TestDockerCompat_Version(t *testing.T) {
	srv := NewServer(&stubRuntime{}, "")
	rr := doGET(t, srv, "/version")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	var out dockertypes.Version
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("docker SDK cannot unmarshal Version: %v\nbody=%s", err, rr.Body.String())
	}
	if out.APIVersion == "" {
		t.Errorf("APIVersion should be populated")
	}
}

func TestDockerCompat_ContainerList(t *testing.T) {
	srv := NewServer(&stubRuntime{
		containerList: func(ctx context.Context, all bool) ([]engine.ContainerInfo, error) {
			return []engine.ContainerInfo{
				{ID: "ctr-1", Name: "web", Image: "nginx:1.25", State: "running", Status: "Up 2 minutes", IP: "10.0.0.2", Command: "nginx -g 'daemon off;'", Created: time.Now()},
			}, nil
		},
	}, "")

	rr := doGET(t, srv, "/containers/json?all=1")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out []dockertypes.Container
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("docker SDK cannot unmarshal []Container: %v\nbody=%s", err, rr.Body.String())
	}
	if len(out) != 1 || out[0].ID != "ctr-1" {
		t.Errorf("unexpected container list: %+v", out)
	}
}

func TestDockerCompat_ContainerList_Empty(t *testing.T) {
	// Empty list must serialize as "[]", not "null" — SDK handles both but
	// some clients and compose flows are stricter.
	srv := NewServer(&stubRuntime{}, "")
	rr := doGET(t, srv, "/containers/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	if got := strings.TrimSpace(rr.Body.String()); got == "null" {
		t.Errorf("empty list rendered as null; want []")
	}
	var out []dockertypes.Container
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("docker SDK cannot unmarshal empty []Container: %v", err)
	}
}

func TestDockerCompat_ImageList(t *testing.T) {
	srv := NewServer(&stubRuntime{
		imageList: func(ctx context.Context) ([]engine.ImageInfo, error) {
			return []engine.ImageInfo{
				{ID: "sha256:abc", Name: "nginx", Tag: "1.25", Created: time.Now()},
			}, nil
		},
	}, "")

	rr := doGET(t, srv, "/images/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out []dockerimage.Summary
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("docker SDK cannot unmarshal []image.Summary: %v\nbody=%s", err, rr.Body.String())
	}
	if len(out) != 1 || out[0].ID != "sha256:abc" {
		t.Errorf("unexpected image list: %+v", out)
	}
}

func TestDockerCompat_ImageInspect(t *testing.T) {
	srv := NewServer(&stubRuntime{
		imageList: func(ctx context.Context) ([]engine.ImageInfo, error) {
			return []engine.ImageInfo{
				{ID: "sha256:abc", Name: "nginx", Tag: "1.25", Created: time.Now()},
			}, nil
		},
	}, "")

	rr := doGET(t, srv, "/images/nginx:1.25/json")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	// Image inspect uses types.ImageInspect — Created is RFC3339 string here,
	// not a unix int as in list.
	var out dockertypes.ImageInspect
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("docker SDK cannot unmarshal ImageInspect: %v\nbody=%s", err, rr.Body.String())
	}
	if out.ID != "sha256:abc" {
		t.Errorf("ID: got %q, want sha256:abc", out.ID)
	}
}

func TestDockerCompat_NetworkList(t *testing.T) {
	srv := NewServer(&stubRuntime{
		networkList: func(ctx context.Context) ([]engine.NetworkInfo, error) {
			return []engine.NetworkInfo{
				{ID: "net-1", Name: "bridge", Driver: "bridge", Scope: "local"},
			}, nil
		},
	}, "")

	rr := doGET(t, srv, "/networks")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out []dockertypes.NetworkResource
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("docker SDK cannot unmarshal []NetworkResource: %v\nbody=%s", err, rr.Body.String())
	}
	if len(out) != 1 || out[0].Name != "bridge" {
		t.Errorf("unexpected network list: %+v", out)
	}
}

func TestDockerCompat_VolumeList(t *testing.T) {
	srv := NewServer(&stubRuntime{
		volumeList: func(ctx context.Context) ([]engine.VolumeInfo, error) {
			return []engine.VolumeInfo{
				{Name: "pgdata", Driver: "local", Mountpoint: "/var/lib/gocker/volumes/pgdata"},
			}, nil
		},
	}, "")

	rr := doGET(t, srv, "/volumes")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out dockervolume.ListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("docker SDK cannot unmarshal volume.ListResponse: %v\nbody=%s", err, rr.Body.String())
	}
	if len(out.Volumes) != 1 || out.Volumes[0].Name != "pgdata" {
		t.Errorf("unexpected volume list: %+v", out.Volumes)
	}
}

func doPOST(t *testing.T, srv *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)
	return rr
}

func TestDockerCompat_NetworkCreateResponse(t *testing.T) {
	srv := NewServer(&stubRuntime{}, "")
	rr := doPOST(t, srv, "/networks/create", `{"Name":"proxy","Driver":"bridge"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	var out dockertypes.NetworkCreateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("docker SDK cannot unmarshal NetworkCreateResponse: %v\nbody=%s", err, rr.Body.String())
	}
	if out.ID == "" {
		t.Errorf("ID should be populated")
	}
}

func TestDockerCompat_VolumeCreateResponse(t *testing.T) {
	srv := NewServer(&stubRuntime{}, "")
	rr := doPOST(t, srv, "/volumes/create", `{"Name":"pgdata","Driver":"local"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	var out dockervolume.Volume
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("docker SDK cannot unmarshal volume.Volume on create: %v\nbody=%s", err, rr.Body.String())
	}
	if out.Name != "pgdata" {
		t.Errorf("Name: got %q, want pgdata", out.Name)
	}
}

func TestDockerCompat_ContainerCreateResponse(t *testing.T) {
	srv := NewServer(&stubRuntime{}, "")
	rr := doPOST(t, srv, "/containers/create?name=web", `{"Image":"nginx:1.25"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	var out dockercontainer.CreateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("docker SDK cannot unmarshal container.CreateResponse: %v\nbody=%s", err, rr.Body.String())
	}
	if out.ID == "" {
		t.Errorf("ID should be populated")
	}
}

func TestDockerCompat_EventShape(t *testing.T) {
	// Verify that the JSON shape gocker emits for events decodes cleanly into
	// the docker SDK's events.Message type. The /events endpoint writes one
	// Event per line — decode a single line in isolation.
	srv := NewServer(&stubRuntime{}, "")
	srv.publishEvent("container", "start", "abc123", map[string]string{"image": "alpine:3"})

	// Build a synthetic request to trigger a flush.
	// We subscribe directly to the bus and emit an event; verify the shape.
	ch, unsub := srv.events.Subscribe()
	defer unsub()

	srv.publishEvent("container", "die", "abc123", map[string]string{"exitCode": "0"})

	select {
	case evt := <-ch:
		data, err := json.Marshal(evt)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		// Decode into the docker SDK's events.Message.
		var msg dockerevents.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("docker SDK cannot unmarshal events.Message: %v\npayload=%s", err, data)
		}
		if msg.Type != "container" {
			t.Errorf("Type: got %q, want container", msg.Type)
		}
		if msg.Action != "die" {
			t.Errorf("Action: got %q, want die", msg.Action)
		}
		if msg.Actor.ID != "abc123" {
			t.Errorf("Actor.ID: got %q, want abc123", msg.Actor.ID)
		}
		if msg.Actor.Attributes["exitCode"] != "0" {
			t.Errorf("Actor.Attributes[exitCode]: got %q, want 0", msg.Actor.Attributes["exitCode"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestDockerCompat_SystemDf(t *testing.T) {
	srv := NewServer(&stubRuntime{
		imageList: func(ctx context.Context) ([]engine.ImageInfo, error) {
			return []engine.ImageInfo{
				{ID: "sha256:abc", Name: "nginx", Tag: "1.25", Created: time.Now()},
			}, nil
		},
		containerList: func(ctx context.Context, all bool) ([]engine.ContainerInfo, error) {
			return []engine.ContainerInfo{
				{ID: "ctr-1", Name: "web", Image: "nginx:1.25", State: "running", Status: "Up", Created: time.Now()},
			}, nil
		},
		volumeList: func(ctx context.Context) ([]engine.VolumeInfo, error) {
			return []engine.VolumeInfo{
				{Name: "pgdata", Driver: "local", Mountpoint: "/var/lib/gocker/volumes/pgdata"},
			}, nil
		},
	}, "")

	rr := doGET(t, srv, "/system/df")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var out dockertypes.DiskUsage
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("docker SDK cannot unmarshal DiskUsage: %v\nbody=%s", err, rr.Body.String())
	}
	if len(out.Images) != 1 {
		t.Errorf("Images: got %d, want 1", len(out.Images))
	}
	if len(out.Containers) != 1 {
		t.Errorf("Containers: got %d, want 1", len(out.Containers))
	}
	if len(out.Volumes) != 1 {
		t.Errorf("Volumes: got %d, want 1", len(out.Volumes))
	}
}

func TestDockerCompat_ContainerCreate_IgnoresDefaultNetworkMode(t *testing.T) {
	// Docker CLI sends HostConfig.NetworkMode="default" on every `docker run`;
	// gocker must NOT forward that to the backend (which doesn't know "default").
	var capturedArgs []string
	stub := &stubRuntime{
		containerRun: func(ctx context.Context, args []string, interactive bool) error {
			capturedArgs = args
			return nil
		},
	}
	srv := NewServer(stub, "")

	body := `{"Image":"alpine:3","HostConfig":{"NetworkMode":"default"}}`
	rr := doPOST(t, srv, "/containers/create?name=test", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	for i, a := range capturedArgs {
		if a == "--network" && i+1 < len(capturedArgs) && capturedArgs[i+1] == "default" {
			t.Errorf("--network default leaked into backend args: %v", capturedArgs)
		}
	}
}

func TestDockerCompat_ContainerCreate_PassesExplicitNetworkMode(t *testing.T) {
	var capturedArgs []string
	stub := &stubRuntime{
		containerRun: func(ctx context.Context, args []string, interactive bool) error {
			capturedArgs = args
			return nil
		},
	}
	srv := NewServer(stub, "")

	body := `{"Image":"alpine:3","HostConfig":{"NetworkMode":"my-net"}}`
	rr := doPOST(t, srv, "/containers/create?name=test", body)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201", rr.Code)
	}
	found := false
	for i, a := range capturedArgs {
		if a == "--network" && i+1 < len(capturedArgs) && capturedArgs[i+1] == "my-net" {
			found = true
		}
	}
	if !found {
		t.Errorf("explicit network mode 'my-net' not forwarded: %v", capturedArgs)
	}
}

func TestDockerCompat_ExecStart_HijackedStream(t *testing.T) {
	// Docker CLI's `docker exec` sends `POST /exec/{id}/start` with
	// `Upgrade: tcp` and expects a 101 Switching Protocols response
	// followed by the raw stdout stream framed with an 8-byte header.
	// Previously gocker returned 200 with the output inline, so the CLI
	// errored with `unable to upgrade to tcp, received 200` and the exit
	// code was wrong.
	//
	// Test strategy: hijack requires a real net.Conn, so run the full
	// Server on an in-memory net pipe, have a client POST the exec-start
	// request, and assert on the raw response bytes.

	srv := NewServer(&stubRuntime{
		execStream: func(ctx context.Context, args ...string) (io.ReadCloser, error) {
			// Simulate the command "echo hi" by returning hi+newline.
			return io.NopCloser(strings.NewReader("hi\n")), nil
		},
	}, "")

	// Register an exec entry so /exec/{id}/start finds it.
	execStore.Store("exec-regression", execEntry{
		containerID: "ctr",
		config:      ExecConfig{AttachStdout: true, Cmd: []string{"echo", "hi"}},
	})

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()

	httpSrv := &http.Server{Handler: srv.mux, ReadHeaderTimeout: 2 * time.Second}
	go func() { _ = httpSrv.Serve(l) }()
	defer func() { _ = httpSrv.Shutdown(context.Background()) }()

	// Raw TCP request so we can read the exact response bytes.
	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	req := "POST /exec/exec-regression/start HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Upgrade: tcp\r\n" +
		"Connection: Upgrade\r\n" +
		"Content-Length: 0\r\n" +
		"\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}

	// Drain the connection until EOF or deadline. The headers and framed
	// payload can arrive in separate TCP segments — a single Read() often
	// returns only the headers (reproduces as a CI flake but a local pass).
	var all bytes.Buffer
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			all.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	body := all.Bytes()

	// Assert on the status line the CLI checks for.
	if !bytes.HasPrefix(body, []byte("HTTP/1.1 101 ")) {
		t.Errorf("expected 101 Switching Protocols, got:\n%s", body)
	}
	// Content-Type must signal the raw-stream protocol.
	if !bytes.Contains(body, []byte("application/vnd.docker.raw-stream")) {
		t.Errorf("expected application/vnd.docker.raw-stream content type; got:\n%s", body)
	}
	// The 3-byte "hi\n" payload should appear framed: 8-byte header
	// then 3 bytes of data. Pattern: 01 00 00 00 00 00 00 03 'h' 'i' '\n'.
	frame := []byte{1, 0, 0, 0, 0, 0, 0, 3, 'h', 'i', '\n'}
	if !bytes.Contains(body, frame) {
		t.Errorf("expected framed stdout payload for 'hi\\n', got raw bytes:\n%x", body)
	}
}

func TestDockerCompat_NetworkInspect_PassesLabelsThrough(t *testing.T) {
	// Docker Compose reads com.docker.compose.project off network labels
	// to decide "is this mine" on every compose up. If gocker hardcodes
	// Labels: {} in the inspect reshape, compose rejects its own networks
	// with "not created by Docker Compose, use external: true".
	srv := NewServer(&stubRuntime{
		networkInspect: func(ctx context.Context, name string) ([]byte, error) {
			return []byte(`[{"id":"net-abc","name":"myproj_default","labels":{"com.docker.compose.project":"myproj","com.docker.compose.network":"default"}}]`), nil
		},
	}, "")

	rr := doGET(t, srv, "/networks/myproj_default")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	var out dockertypes.NetworkResource
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rr.Body.String())
	}
	if out.Labels["com.docker.compose.project"] != "myproj" {
		t.Errorf("compose project label lost: got Labels=%v", out.Labels)
	}
	if out.Labels["com.docker.compose.network"] != "default" {
		t.Errorf("compose network label lost: got Labels=%v", out.Labels)
	}
}

func TestDockerCompat_NetworkInspect_LabelsUnderConfigAppleShape(t *testing.T) {
	// Apple Container CLI's `container network inspect` nests labels under
	// `config.labels`, not at the top level. Make sure we still find them.
	srv := NewServer(&stubRuntime{
		networkInspect: func(ctx context.Context, name string) ([]byte, error) {
			return []byte(`[{"id":"myproj_default","config":{"id":"myproj_default","labels":{"com.docker.compose.project":"myproj"}},"state":"running"}]`), nil
		},
	}, "")

	rr := doGET(t, srv, "/networks/myproj_default")
	var out dockertypes.NetworkResource
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	if out.Labels["com.docker.compose.project"] != "myproj" {
		t.Errorf("nested config.labels not extracted: got Labels=%v", out.Labels)
	}
}

func TestDockerCompat_VolumeList_PassesLabelsThrough(t *testing.T) {
	// Compose's network/volume ownership check queries the LIST endpoint
	// first. Previously VolumeJSON had no Labels field so compose saw
	// empty labels and refused its own volumes on every `up`.
	srv := NewServer(&stubRuntime{
		volumeList: func(ctx context.Context) ([]engine.VolumeInfo, error) {
			return []engine.VolumeInfo{{
				Name:   "myproj_data",
				Driver: "local",
				Labels: map[string]string{
					"com.docker.compose.project": "myproj",
					"com.docker.compose.volume":  "data",
				},
			}}, nil
		},
	}, "")

	rr := doGET(t, srv, "/volumes")
	var out dockervolume.ListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(out.Volumes))
	}
	if out.Volumes[0].Labels["com.docker.compose.project"] != "myproj" {
		t.Errorf("labels lost in list response: %+v", out.Volumes[0].Labels)
	}
}

func TestDockerCompat_NetworkList_PassesLabelsThrough(t *testing.T) {
	srv := NewServer(&stubRuntime{
		networkList: func(ctx context.Context) ([]engine.NetworkInfo, error) {
			return []engine.NetworkInfo{{
				ID:   "net-abc",
				Name: "myproj_default",
				Labels: map[string]string{
					"com.docker.compose.project": "myproj",
					"com.docker.compose.network": "default",
				},
			}}, nil
		},
	}, "")

	rr := doGET(t, srv, "/networks")
	var out []dockertypes.NetworkResource
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 network, got %d", len(out))
	}
	if out[0].Labels["com.docker.compose.project"] != "myproj" {
		t.Errorf("labels lost in list response: %+v", out[0].Labels)
	}
}

func TestDockerCompat_VolumeInspect_PassesLabelsThrough(t *testing.T) {
	srv := NewServer(&stubRuntime{
		volumeInspect: func(ctx context.Context, name string) ([]byte, error) {
			return []byte(`[{"name":"myproj_data","driver":"local","labels":{"com.docker.compose.project":"myproj","com.docker.compose.volume":"data"}}]`), nil
		},
	}, "")

	rr := doGET(t, srv, "/volumes/myproj_data")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	var out dockervolume.Volume
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rr.Body.String())
	}
	if out.Labels["com.docker.compose.project"] != "myproj" {
		t.Errorf("compose project label lost: got Labels=%v", out.Labels)
	}
	if out.Labels["com.docker.compose.volume"] != "data" {
		t.Errorf("compose volume label lost: got Labels=%v", out.Labels)
	}
}

func TestDockerCompat_Stats_ReturnsValidStatsJSON(t *testing.T) {
	// lazydocker polls /containers/{id}/stats?stream=1 for CPU/mem. A 404
	// or malformed response makes the stats panel dead. We stub zero
	// values but must emit a shape that decodes into types.StatsJSON.
	srv := NewServer(&stubRuntime{
		containerInspect: func(ctx context.Context, id string) ([]byte, error) {
			// Any non-error response is enough to pass the existence check.
			return []byte(`[{"Id":"ctr-stats"}]`), nil
		},
	}, "")

	rr := doGET(t, srv, "/containers/ctr-stats/stats?stream=0")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	// Body is newline-terminated JSON; strip the trailing newline for decode.
	body := bytes.TrimSpace(rr.Body.Bytes())
	var out dockertypes.StatsJSON
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("docker SDK cannot unmarshal StatsJSON: %v\nbody=%s", err, body)
	}
	if out.Name != "/ctr-stats" {
		t.Errorf("Name: got %q, want /ctr-stats", out.Name)
	}
}

func TestDockerCompat_Stats_404OnMissingContainer(t *testing.T) {
	srv := NewServer(&stubRuntime{
		containerInspect: func(ctx context.Context, id string) ([]byte, error) {
			return nil, errors.New("no such container")
		},
	}, "")

	rr := doGET(t, srv, "/containers/nope/stats?stream=0")
	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rr.Code)
	}
}

func TestDockerCompat_Logs_EmitsFramedOutput(t *testing.T) {
	// Docker CLI / lazydocker decode logs using the same 8-byte multiplex
	// frame format as /exec/{id}/start. Previously gocker wrote raw bytes
	// and clients rendered garbage. Assert the frame shape.
	srv := NewServer(&stubRuntime{
		exec: func(ctx context.Context, args ...string) ([]byte, []byte, error) {
			return []byte("hi\n"), nil, nil
		},
	}, "")

	rr := doGET(t, srv, "/containers/ctr/logs")
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	// Expected frame: [01 00 00 00  00 00 00 03]  h i \n
	want := []byte{1, 0, 0, 0, 0, 0, 0, 3, 'h', 'i', '\n'}
	if !bytes.Equal(rr.Body.Bytes(), want) {
		t.Errorf("framed output mismatch:\n  got  %x\n  want %x", rr.Body.Bytes(), want)
	}
}

func TestDockerCompat_VolumeInspect_EmptyArrayReturns404(t *testing.T) {
	srv := NewServer(&stubRuntime{
		volumeInspect: func(ctx context.Context, name string) ([]byte, error) {
			return []byte(`[]`), nil
		},
	}, "")

	rr := doGET(t, srv, "/volumes/missing")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", rr.Code)
	}
}
