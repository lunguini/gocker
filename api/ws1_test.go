package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lunguini/gocker/engine"
)

// doGETVersioned drives a request through Server.ServeHTTP (not the raw mux)
// so the version-prefix stripper is exercised.
func doGETVersioned(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	return rr
}

// C3: the version-prefix stripper must only strip a real /vMAJOR.MINOR/
// prefix. Unversioned routes that happen to start with 'v' (/volumes/...)
// must reach their handler untouched.
func TestC3_VersionPrefixStripping(t *testing.T) {
	srv := NewServer(&stubRuntime{
		volumeInspect: func(ctx context.Context, name string) ([]byte, error) {
			return []byte(`{"Name":"` + name + `","Driver":"local","Mountpoint":"/x"}`), nil
		},
	}, "")

	cases := []struct {
		path     string
		wantCode int
		wantName string // for volume inspect responses
	}{
		{"/v1.41/volumes/myvol", http.StatusOK, "myvol"},
		{"/volumes/myvol", http.StatusOK, "myvol"},     // regression: no version prefix
		{"/v1.41/version", http.StatusOK, ""},          // versioned system route
		{"/volumes/version", http.StatusOK, "version"}, // 'version' is a volume name here
	}
	for _, tc := range cases {
		rr := doGETVersioned(t, srv, tc.path)
		if rr.Code != tc.wantCode {
			t.Errorf("%s: status got %d want %d (body: %s)", tc.path, rr.Code, tc.wantCode, rr.Body.String())
			continue
		}
		if tc.wantName != "" {
			var v VolumeJSON
			if err := json.Unmarshal(rr.Body.Bytes(), &v); err != nil {
				t.Errorf("%s: decode: %v", tc.path, err)
				continue
			}
			if v.Name != tc.wantName {
				t.Errorf("%s: volume name got %q want %q", tc.path, v.Name, tc.wantName)
			}
		}
	}
}

// M14: /version and /info must report the version threaded into NewServer,
// not the old hardcoded "gocker-0.1.0".
func TestM14_VersionThreadedThroughNewServer(t *testing.T) {
	srv := NewServer(&stubRuntime{}, "", "v9.9.9-test")
	rr := doGET(t, srv, "/version")
	var v VersionResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v.Version != "v9.9.9-test" {
		t.Errorf("Version got %q want %q", v.Version, "v9.9.9-test")
	}

	// Default when unset falls back to "dev", never the stale constant.
	srv2 := NewServer(&stubRuntime{}, "")
	rr2 := doGET(t, srv2, "/version")
	var v2 VersionResponse
	_ = json.Unmarshal(rr2.Body.Bytes(), &v2)
	if v2.Version != "dev" {
		t.Errorf("default Version got %q want %q", v2.Version, "dev")
	}
	if strings.Contains(v2.Version, "0.1.0") {
		t.Errorf("stale hardcoded version leaked: %q", v2.Version)
	}
}

// M1: within a single filter key, values are ORed (Docker semantics). Two
// name filters must match a container bearing either name, and the status
// filter must actually filter.
func TestM1_FilterSemantics(t *testing.T) {
	list := []engine.ContainerInfo{
		{ID: "a", Name: "alpha", State: "running", Status: "Up 3s", Created: time.Now()},
		{ID: "b", Name: "beta", State: "exited", Status: "Exited (0)", Created: time.Now()},
	}
	srv := NewServer(&stubRuntime{
		containerList: func(ctx context.Context, all bool) ([]engine.ContainerInfo, error) {
			return list, nil
		},
	}, "")

	// OR within name: both alpha and beta should come back.
	rr := doGET(t, srv, `/containers/json?filters={"name":["alpha","beta"]}`)
	var got []ContainerJSON
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("name OR filter: got %d containers, want 2", len(got))
	}

	// status filter: only the exited one.
	rr = doGET(t, srv, `/containers/json?filters={"status":["exited"]}`)
	got = nil
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].ID != "b" {
		t.Errorf("status filter: got %+v, want single container b", got)
	}
}

// M2: a zero creation time must serialize as Created=0, not the negative
// pseudo-timestamp that .Unix() yields for the zero value.
func TestM2_ZeroCreatedTime(t *testing.T) {
	srv := NewServer(&stubRuntime{
		containerList: func(ctx context.Context, all bool) ([]engine.ContainerInfo, error) {
			return []engine.ContainerInfo{{ID: "a", Name: "z", State: "running", Status: "Up"}}, nil
		},
	}, "")
	rr := doGET(t, srv, "/containers/json")
	var got []ContainerJSON
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d containers want 1", len(got))
	}
	if got[0].Created != 0 {
		t.Errorf("Created got %d want 0 (zero time must not become negative)", got[0].Created)
	}
	// And no fabricated bridge network when there's no IP.
	if got[0].NetworkSettings != nil {
		t.Errorf("NetworkSettings should be nil when container has no IP, got %+v", got[0].NetworkSettings)
	}
}

// M4: sub-resources on the /images/{name...} catch-all must not be misrouted
// into inspect, and a short image-ID prefix must resolve.
func TestM4_ImageSubResourceAndShortID(t *testing.T) {
	srv := NewServer(&stubRuntime{
		imageList: func(ctx context.Context) ([]engine.ImageInfo, error) {
			return []engine.ImageInfo{
				{ID: "sha256:abcdef1234567890", Name: "alpine", Tag: "3"},
			}, nil
		},
	}, "")

	// history is unimplemented → 501, not a confusing 404.
	rr := doGET(t, srv, "/images/alpine/history")
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("history: got %d want 501 (body %s)", rr.Code, rr.Body.String())
	}

	// short-ID inspect must resolve via prefix match.
	rr = doGET(t, srv, "/images/abcdef123456/json")
	if rr.Code != http.StatusOK {
		t.Errorf("short-id inspect: got %d want 200 (body %s)", rr.Code, rr.Body.String())
	}

	// full name inspect still works.
	rr = doGET(t, srv, "/images/alpine:3/json")
	if rr.Code != http.StatusOK {
		t.Errorf("name inspect: got %d want 200 (body %s)", rr.Code, rr.Body.String())
	}
}

// M5: a fast-failing pull (before any progress is streamed) maps a
// not-found error to 404 rather than a blanket 500.
func TestM5_PullNotFoundMapsTo404(t *testing.T) {
	srv := NewServer(&stubRuntime{
		imagePull: func(ctx context.Context, image string, opts engine.ImagePullOpts) error {
			return errors.New("no such image: bogus:latest")
		},
	}, "")
	rr := doPOST(t, srv, "/images/create?fromImage=bogus&tag=latest", "")
	if rr.Code != http.StatusNotFound {
		t.Errorf("pull of nonexistent image: got %d want 404 (body %s)", rr.Code, rr.Body.String())
	}
}

// M5: a successful fast pull returns 200 with a final NDJSON status line.
func TestM5_PullSuccessStatusLine(t *testing.T) {
	srv := NewServer(&stubRuntime{
		imagePull: func(ctx context.Context, image string, opts engine.ImagePullOpts) error {
			return nil
		},
	}, "")
	rr := doPOST(t, srv, "/images/create?fromImage=alpine&tag=latest", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("pull: got %d want 200 (body %s)", rr.Code, rr.Body.String())
	}
	var msg map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &msg); err != nil {
		t.Fatalf("final status line not valid JSON: %v (body %s)", err, rr.Body.String())
	}
	if !strings.Contains(msg["status"], "alpine") {
		t.Errorf("status line %q should mention the image", msg["status"])
	}
}
