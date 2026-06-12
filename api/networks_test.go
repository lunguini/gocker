package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lunguini/gocker/engine"
)

// stubRuntime is a no-op implementation of engine.Runtime used for API tests.
// Individual tests override specific methods via function fields.
type stubRuntime struct {
	networkInspect   func(ctx context.Context, name string) ([]byte, error)
	volumeInspect    func(ctx context.Context, name string) ([]byte, error)
	containerInspect func(ctx context.Context, nameOrID string) ([]byte, error)
	containerList    func(ctx context.Context, all bool) ([]engine.ContainerInfo, error)
	containerRun     func(ctx context.Context, args []string, interactive bool) error
	exec             func(ctx context.Context, args ...string) ([]byte, []byte, error)
	execStream       func(ctx context.Context, args ...string) (io.ReadCloser, error)
	execStreamSplit  func(ctx context.Context, args ...string) (io.ReadCloser, io.ReadCloser, error)
	imageList        func(ctx context.Context) ([]engine.ImageInfo, error)
	networkList      func(ctx context.Context) ([]engine.NetworkInfo, error)
	volumeList       func(ctx context.Context) ([]engine.VolumeInfo, error)
	networkCreate    func(ctx context.Context, name string, labels map[string]string) error
	volumeCreate     func(ctx context.Context, name string) error
	containerStart   func(ctx context.Context, nameOrID string) error
	containerStop    func(ctx context.Context, nameOrID string) error
	containerRemove  func(ctx context.Context, nameOrID string, force bool) error
	networkRemove    func(ctx context.Context, name string) error
	volumeRemove     func(ctx context.Context, name string) error
}

func (s *stubRuntime) Validate() error    { return nil }
func (s *stubRuntime) BinaryPath() string { return "" }
func (s *stubRuntime) Exec(ctx context.Context, args ...string) ([]byte, []byte, error) {
	if s.exec != nil {
		return s.exec(ctx, args...)
	}
	return nil, nil, nil
}
func (s *stubRuntime) ExecInteractive(ctx context.Context, args ...string) error { return nil }
func (s *stubRuntime) ExecStream(ctx context.Context, args ...string) (io.ReadCloser, error) {
	if s.execStream != nil {
		return s.execStream(ctx, args...)
	}
	return nil, nil
}
func (s *stubRuntime) ExecStreamSplit(ctx context.Context, args ...string) (io.ReadCloser, io.ReadCloser, error) {
	if s.execStreamSplit != nil {
		return s.execStreamSplit(ctx, args...)
	}
	return nil, nil, nil
}
func (s *stubRuntime) ContainerRun(ctx context.Context, args []string, interactive bool) error {
	if s.containerRun != nil {
		return s.containerRun(ctx, args, interactive)
	}
	return nil
}
func (s *stubRuntime) ContainerList(ctx context.Context, all bool) ([]engine.ContainerInfo, error) {
	if s.containerList != nil {
		return s.containerList(ctx, all)
	}
	return nil, nil
}
func (s *stubRuntime) ContainerStop(ctx context.Context, nameOrID string) error {
	if s.containerStop != nil {
		return s.containerStop(ctx, nameOrID)
	}
	return nil
}
func (s *stubRuntime) ContainerStart(ctx context.Context, nameOrID string) error {
	if s.containerStart != nil {
		return s.containerStart(ctx, nameOrID)
	}
	return nil
}
func (s *stubRuntime) ContainerRemove(ctx context.Context, nameOrID string, force bool) error {
	if s.containerRemove != nil {
		return s.containerRemove(ctx, nameOrID, force)
	}
	return nil
}
func (s *stubRuntime) ContainerExec(ctx context.Context, nameOrID string, args []string, interactive bool) error {
	return nil
}
func (s *stubRuntime) ContainerLogs(ctx context.Context, nameOrID string, opts engine.LogsOptions) error {
	return nil
}
func (s *stubRuntime) ContainerInspect(ctx context.Context, nameOrID string) ([]byte, error) {
	if s.containerInspect != nil {
		return s.containerInspect(ctx, nameOrID)
	}
	return nil, nil
}
func (s *stubRuntime) ImagePull(ctx context.Context, image string, opts engine.ImagePullOpts) error {
	return nil
}
func (s *stubRuntime) ImagePush(ctx context.Context, image string) error         { return nil }
func (s *stubRuntime) ImageList(ctx context.Context) ([]engine.ImageInfo, error) {
	if s.imageList != nil {
		return s.imageList(ctx)
	}
	return nil, nil
}
func (s *stubRuntime) ImageRemove(ctx context.Context, image string) error       { return nil }
func (s *stubRuntime) ImageBuild(ctx context.Context, args []string) error       { return nil }
func (s *stubRuntime) NetworkCreate(ctx context.Context, name string, labels map[string]string) error {
	if s.networkCreate != nil {
		return s.networkCreate(ctx, name, labels)
	}
	return nil
}
func (s *stubRuntime) NetworkList(ctx context.Context) ([]engine.NetworkInfo, error) {
	if s.networkList != nil {
		return s.networkList(ctx)
	}
	return nil, nil
}
func (s *stubRuntime) NetworkRemove(ctx context.Context, name string) error {
	if s.networkRemove != nil {
		return s.networkRemove(ctx, name)
	}
	return nil
}
func (s *stubRuntime) NetworkConnect(ctx context.Context, network, container string) error {
	return nil
}
func (s *stubRuntime) NetworkDisconnect(ctx context.Context, network, container string) error {
	return nil
}
func (s *stubRuntime) NetworkInspect(ctx context.Context, name string) ([]byte, error) {
	if s.networkInspect != nil {
		return s.networkInspect(ctx, name)
	}
	return nil, nil
}
func (s *stubRuntime) VolumeCreate(ctx context.Context, name string) error {
	if s.volumeCreate != nil {
		return s.volumeCreate(ctx, name)
	}
	return nil
}
func (s *stubRuntime) VolumeList(ctx context.Context) ([]engine.VolumeInfo, error) {
	if s.volumeList != nil {
		return s.volumeList(ctx)
	}
	return nil, nil
}
func (s *stubRuntime) VolumeRemove(ctx context.Context, name string) error {
	if s.volumeRemove != nil {
		return s.volumeRemove(ctx, name)
	}
	return nil
}
func (s *stubRuntime) VolumeInspect(ctx context.Context, name string) ([]byte, error) {
	if s.volumeInspect != nil {
		return s.volumeInspect(ctx, name)
	}
	return nil, nil
}

// TestNetworkInspectAppleCLIArrayResponse verifies that when the Apple
// container CLI returns its inspect payload as a JSON array with lowercase
// field names, the API handler unwraps it into a Docker-compatible object.
// Previously the raw array was written to the response, causing Docker SDK
// clients to fail with: "json: cannot unmarshal array into Go value of type
// network.Inspect".
func TestNetworkInspectAppleCLIArrayResponse(t *testing.T) {
	applePayload := []byte(`[{"id":"net-abc123","name":"mynet","driver":"bridge","scope":"local"}]`)
	srv := NewServer(&stubRuntime{
		networkInspect: func(ctx context.Context, name string) ([]byte, error) {
			return applePayload, nil
		},
	}, "")

	req := httptest.NewRequest(http.MethodGet, "/networks/mynet", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	// Must decode as a single object (not an array) — this is what Docker SDK does.
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not a JSON object: %v; body=%s", err, rr.Body.String())
	}

	assertString := func(key, want string) {
		t.Helper()
		if v, _ := got[key].(string); v != want {
			t.Errorf("%s: got %q, want %q", key, v, want)
		}
	}
	assertString("Id", "net-abc123")
	assertString("Name", "mynet")
	assertString("Driver", "bridge")
	assertString("Scope", "local")

	// Docker SDK expects these fields to be present as objects/maps, not missing.
	for _, key := range []string{"IPAM", "Containers", "Options", "Labels"} {
		if _, ok := got[key]; !ok {
			t.Errorf("expected response to contain %q field", key)
		}
	}
}

func TestNetworkInspectAppleCLIObjectResponse(t *testing.T) {
	// Defensive: if Apple ever switches to returning an object directly, the
	// handler should still reshape it correctly.
	applePayload := []byte(`{"id":"net-xyz","name":"bridge","driver":"bridge","scope":"local"}`)
	srv := NewServer(&stubRuntime{
		networkInspect: func(ctx context.Context, name string) ([]byte, error) {
			return applePayload, nil
		},
	}, "")

	req := httptest.NewRequest(http.MethodGet, "/networks/bridge", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not a JSON object: %v", err)
	}
	if id, _ := got["Id"].(string); id != "net-xyz" {
		t.Errorf("Id: got %q, want %q", id, "net-xyz")
	}
}

func TestNetworkInspectEmptyArrayReturns404(t *testing.T) {
	srv := NewServer(&stubRuntime{
		networkInspect: func(ctx context.Context, name string) ([]byte, error) {
			return []byte(`[]`), nil
		},
	}, "")

	req := httptest.NewRequest(http.MethodGet, "/networks/missing", nil)
	rr := httptest.NewRecorder()
	srv.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404; body=%s", rr.Code, rr.Body.String())
	}
}
