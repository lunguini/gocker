//go:build integration

// Package placement note: this suite asserts cross-backend behavioral
// contracts for engine.Runtime, including the not-found error contract that
// api.isNotFoundErr enforces at the HTTP layer. It must exercise both
// engine.Runtime (constructing real backends) and isNotFoundErr directly.
// engine cannot import api (api already imports engine), so a shared suite
// can't live in package engine without a cycle. Living in package api (this
// file) lets it call isNotFoundErr and errors_test.go's neighbors directly
// while importing engine like any other api file already does.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lunguini/gocker/engine"
)

// conformanceImage is the image exercised by the suite. Overridable via
// GOCKER_CONFORMANCE_IMAGE for environments where alpine:latest can't be
// pulled anonymously from Docker Hub (rate limits / auth requirements) but
// another image is already cached locally.
func conformanceImage() string {
	if img := os.Getenv("GOCKER_CONFORMANCE_IMAGE"); img != "" {
		return img
	}
	return "alpine:latest"
}

var conformanceCounter int64

// conformanceName returns a unique per-test resource name so parallel/repeat
// runs of this suite never collide, and so a failed run's leftovers are easy
// to spot and hand-clean (`gocker sandbox ls` / `container list -a` /
// `container volume list` / `container network list` all show the prefix).
func conformanceName(kind string) string {
	n := atomic.AddInt64(&conformanceCounter, 1)
	return fmt.Sprintf("conformance-%s-%d-%d", kind, os.Getpid(), n)
}

// setupConformanceRuntime constructs the darwin/linux Runtime under test.
// Named distinctly from engine.setupRuntime (engine/container_integration_test.go)
// to avoid any confusion that this is calling into that unexported helper —
// package api cannot reach it (unexported, different package) so it is
// reimplemented here.
func setupConformanceRuntime(t *testing.T) engine.Runtime {
	t.Helper()
	switch runtime.GOOS {
	case "darwin":
		eng := engine.New("")
		_ = eng.EnsureSystemRunning(context.Background())
		return eng
	case "linux":
		return engine.NewNerdctl("")
	default:
		t.Skipf("unsupported platform: %s", runtime.GOOS)
		return nil
	}
}

// skipIfNoVirtualizationConformance mirrors engine.skipIfNoVirtualization —
// reimplemented locally for the same reason as setupConformanceRuntime.
func skipIfNoVirtualizationConformance(t *testing.T, err error) {
	t.Helper()
	if err != nil && strings.Contains(err.Error(), "Virtualization") {
		t.Skipf("skipping: %v", err)
	}
}

func TestConformance_Engine(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Engine (Apple Container CLI) backend only exists on darwin")
	}
	eng := engine.New("")
	if err := eng.EnsureSystemRunning(context.Background()); err != nil {
		t.Skipf("container system not available: %v", err)
	}
	if err := eng.Validate(); err != nil {
		t.Skipf("container CLI not available: %v", err)
	}
	runConformance(t, eng)
}

func TestConformance_Nerdctl(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("NerdctlRuntime backend only exists on linux")
	}
	rt := engine.NewNerdctl("")
	if err := rt.Validate(); err != nil {
		t.Skipf("nerdctl not available: %v", err)
	}
	runConformance(t, rt)
}

// TestConformance_SharedVM would exercise sharedvm.SharedVMRuntime, but doing
// so from package api would require importing the sharedvm package (which
// imports engine) plus a live shared VM. That's a reasonable no-cycle import
// on paper, but wiring a real Manager here means duplicating VM lifecycle
// setup/gating (GOCKER_DESTRUCTIVE_TESTS, EnsureRunning, Remove-on-cleanup)
// that already exists in sharedvm/sharedvm_integration_test.go, and this
// suite has no safe way to guarantee it isn't churning a developer's in-use
// VM. Left as a follow-up: the cleanest fix is likely to move runConformance
// itself into an internal helper package importable by both api and
// sharedvm's integration tests, so sharedvm can call it against its own
// gated Manager-backed runtime without api needing to depend on sharedvm.
// TODO(follow-up): wire SharedVMRuntime into this suite via that shared helper.

// runConformance is the shared table of behavioral assertions run against
// every engine.Runtime backend. Every subtest cleans up after itself via
// t.Cleanup so a failure doesn't leak containers/networks/volumes.
func runConformance(t *testing.T, rt engine.Runtime) {
	ctx := context.Background()

	if err := rt.ImagePull(ctx, conformanceImage(), engine.ImagePullOpts{}); err != nil {
		skipIfNoVirtualizationConformance(t, err)
		// Anonymous Docker Hub pulls are frequently rate-limited/401'd from
		// shared CI/dev IPs. If the image is already cached locally (a
		// previous successful pull, or pre-seeded in CI), fall back to it
		// rather than failing the whole suite on registry flakiness that has
		// nothing to do with Runtime conformance.
		images, listErr := rt.ImageList(ctx)
		if listErr != nil || !imagePresent(images, conformanceImage()) {
			t.Skipf("ImagePull(%s) failed and image not cached locally: %v", conformanceImage(), err)
		}
		t.Logf("ImagePull(%s) failed (%v); using locally cached image", conformanceImage(), err)
	}

	t.Run("CreateStartSplit", func(t *testing.T) { testCreateStartSplit(t, rt) })
	t.Run("NotFoundContract", func(t *testing.T) { testNotFoundContract(t, rt) })
	t.Run("InspectShape", func(t *testing.T) { testInspectShape(t, rt) })
	t.Run("IDConsistency", func(t *testing.T) { testIDConsistency(t, rt) })
	t.Run("VolumeRoundtrip", func(t *testing.T) { testVolumeRoundtrip(t, rt) })
	t.Run("NetworkRoundtrip", func(t *testing.T) { testNetworkRoundtrip(t, rt) })
	t.Run("ExecStreamStdin", func(t *testing.T) { testExecStreamStdin(t, rt) })
	t.Run("EmptyListsNoError", func(t *testing.T) { testEmptyListsNoError(t, rt) })
}

// imagePresent reports whether image (e.g. "debian:bookworm-slim") matches
// any entry in a list by repo name, tolerating the repo:tag vs. separate
// Name/Tag field split that ImageInfo uses.
func imagePresent(images []engine.ImageInfo, image string) bool {
	repo, tag, _ := strings.Cut(image, ":")
	for _, img := range images {
		if strings.Contains(img.Name, repo) && (tag == "" || img.Tag == tag || strings.Contains(img.Tag, tag)) {
			return true
		}
	}
	return false
}

// findContainer returns the ContainerInfo matching name/ID from a list, or
// (zero, false) if absent.
func findContainer(containers []engine.ContainerInfo, name string) (engine.ContainerInfo, bool) {
	for _, c := range containers {
		if c.Name == name || c.ID == name || strings.Contains(c.Name, name) || strings.Contains(c.ID, name) {
			return c, true
		}
	}
	return engine.ContainerInfo{}, false
}

// isRunning reports whether a ContainerInfo's Status/State fields indicate
// the container is currently running. Backends phrase this differently
// ("running", "Up 3 seconds", "Running") so match loosely.
func isRunning(c engine.ContainerInfo) bool {
	s := strings.ToLower(c.Status + " " + c.State)
	return strings.Contains(s, "run") || strings.HasPrefix(s, "up")
}

// probeVirtualization attempts the cheapest operation that requires
// Virtualization.framework so callers can distinguish "backend broken" from
// "this runner can't boot VMs" (where some operations silently no-op).
// Returns nil where virtualization works.
func probeVirtualization(rt engine.Runtime) error {
	ctx := context.Background()
	name := conformanceName("virtprobe")
	defer func() { _ = rt.ContainerRemove(ctx, name, true) }()
	if _, err := rt.ContainerCreate(ctx, []string{"--name", name, conformanceImage(), "true"}); err != nil {
		return err
	}
	return rt.ContainerStart(ctx, name)
}

func testCreateStartSplit(t *testing.T, rt engine.Runtime) {
	ctx := context.Background()
	name := conformanceName("split")
	_ = rt.ContainerRemove(ctx, name, true)
	t.Cleanup(func() {
		_ = rt.ContainerStop(ctx, name)
		_ = rt.ContainerRemove(ctx, name, true)
	})

	args := []string{"--name", name, conformanceImage(), "sleep", "300"}
	id, err := rt.ContainerCreate(ctx, args)
	if err != nil {
		skipIfNoVirtualizationConformance(t, err)
		t.Fatalf("ContainerCreate failed: %v", err)
	}
	if strings.TrimSpace(id) == "" {
		t.Fatalf("ContainerCreate returned empty ID")
	}

	containers, err := rt.ContainerList(ctx, true)
	if err != nil {
		t.Fatalf("ContainerList(all=true) failed: %v", err)
	}
	c, found := findContainer(containers, name)
	if !found {
		t.Fatalf("created container %q not found in ContainerList(all=true)", name)
	}
	if isRunning(c) {
		t.Errorf("container %q reported running before ContainerStart (status=%q state=%q)", name, c.Status, c.State)
	}

	if err := rt.ContainerStart(ctx, name); err != nil {
		// Create succeeds without booting a VM; Start is where hosted CI
		// runners without Virtualization.framework fail.
		skipIfNoVirtualizationConformance(t, err)
		t.Fatalf("ContainerStart failed: %v", err)
	}

	running, err := rt.ContainerList(ctx, false)
	if err != nil {
		t.Fatalf("ContainerList(all=false) failed: %v", err)
	}
	c2, found := findContainer(running, name)
	if !found {
		t.Fatalf("started container %q not found in ContainerList(all=false)", name)
	}
	if !isRunning(c2) {
		t.Errorf("container %q not reported running after ContainerStart (status=%q state=%q)", name, c2.Status, c2.State)
	}

	if err := rt.ContainerStop(ctx, name); err != nil {
		t.Fatalf("ContainerStop failed: %v", err)
	}
	if err := rt.ContainerRemove(ctx, name, true); err != nil {
		t.Fatalf("ContainerRemove failed: %v", err)
	}
}

// emptyInspectResult reports whether inspect output is the "resource does
// not exist" shape some backends return with a zero exit status: Apple's
// `container inspect` prints an empty JSON array for unknown names. The API
// handlers 404 this via the reshape-failure fallback (see
// handleContainerInspect and reshape*Inspect), so it satisfies the
// not-found contract just like a classified error does.
func emptyInspectResult(data []byte) bool {
	trimmed := strings.TrimSpace(string(data))
	return trimmed == "" || trimmed == "[]" || trimmed == "null"
}

func testNotFoundContract(t *testing.T, rt engine.Runtime) {
	ctx := context.Background()
	missing := conformanceName("missing")

	// The API-observable contract: a request against a nonexistent resource
	// must end up a 404. Backends satisfy it two ways — an error recognized
	// by isNotFoundErr (writeRuntimeError path), or, for inspects only, a
	// successful-but-empty JSON result (reshape-failure path).
	inspects := []struct {
		name string
		call func() ([]byte, error)
	}{
		{"ContainerInspect", func() ([]byte, error) { return rt.ContainerInspect(ctx, missing) }},
		{"NetworkInspect", func() ([]byte, error) { return rt.NetworkInspect(ctx, missing) }},
		{"VolumeInspect", func() ([]byte, error) { return rt.VolumeInspect(ctx, missing) }},
	}
	for _, c := range inspects {
		data, err := c.call()
		switch {
		case err == nil && emptyInspectResult(data):
			// Apple CLI shape: exit 0 + "[]"; API 404s via reshape fallback.
		case err != nil && isNotFoundErr(err):
			// nerdctl shape: "No such ..." error; API 404s via writeRuntimeError.
		case err == nil:
			t.Errorf("%s against nonexistent %q returned data the API would serve as 200: %s", c.name, missing, data)
		default:
			t.Errorf("%s against nonexistent %q returned error not recognized by isNotFoundErr: %v", c.name, missing, err)
		}
	}

	removes := []struct {
		name string
		err  error
	}{
		{"ContainerRemove", rt.ContainerRemove(ctx, missing, false)},
		{"ImageRemove", rt.ImageRemove(ctx, "docker.io/library/definitely-not-a-real-image:"+missing)},
	}
	for _, c := range removes {
		if c.err == nil {
			t.Errorf("%s against nonexistent resource %q returned nil error, expected an error", c.name, missing)
			continue
		}
		if !isNotFoundErr(c.err) {
			t.Errorf("%s against nonexistent resource %q returned error not recognized by isNotFoundErr (API would 500 instead of 404): %v", c.name, missing, c.err)
		}
	}

	// DIVERGENCE: Apple's `container stop` exits 0 for unknown names, so the
	// Engine backend cannot signal not-found and the API returns success
	// where Docker returns 404. nerdctl errors properly. Fixing this would
	// require an existence pre-check on every stop — documented follow-up.
	if err := rt.ContainerStop(ctx, missing); err == nil {
		if _, isApple := rt.(*engine.Engine); isApple {
			t.Logf("DIVERGENCE (documented, not failed): Engine ContainerStop on missing container returns nil; API serves 2xx instead of 404")
		} else {
			t.Errorf("ContainerStop against nonexistent %q returned nil error, expected an error", missing)
		}
	} else if !isNotFoundErr(err) {
		t.Errorf("ContainerStop against nonexistent %q returned error not recognized by isNotFoundErr: %v", missing, err)
	}
}

// caseInsensitiveKey reports whether m contains key under any case variant.
func caseInsensitiveKey(m map[string]any, key string) (any, bool) {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return nil, false
}

// hasIdentifiableField walks a decoded JSON value (map or array-of-maps) and
// reports whether at least one object exposes a name/id-shaped field,
// matching the loose case-insensitive lookup style used by the engine
// parsers (jsonx.GetString).
func hasIdentifiableField(v any) bool {
	candidates := []string{"id", "name", "Id", "ID", "Name"}
	check := func(m map[string]any) bool {
		for _, c := range candidates {
			if val, ok := caseInsensitiveKey(m, c); ok {
				if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
					return true
				}
			}
			// Apple's container inspect nests id under "configuration".
			if cfg, ok := caseInsensitiveKey(m, "configuration"); ok {
				if cm, ok := cfg.(map[string]any); ok {
					if val, ok := caseInsensitiveKey(cm, "id"); ok {
						if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
							return true
						}
					}
				}
			}
		}
		return false
	}
	switch t := v.(type) {
	case map[string]any:
		return check(t)
	case []any:
		for _, item := range t {
			if m, ok := item.(map[string]any); ok && check(m) {
				return true
			}
		}
	}
	return false
}

func testInspectShape(t *testing.T, rt engine.Runtime) {
	ctx := context.Background()
	name := conformanceName("inspect")
	_ = rt.ContainerRemove(ctx, name, true)
	t.Cleanup(func() {
		_ = rt.ContainerStop(ctx, name)
		_ = rt.ContainerRemove(ctx, name, true)
	})

	args := []string{"-d", "--name", name, conformanceImage(), "sleep", "300"}
	if err := rt.ContainerRun(ctx, args, false); err != nil {
		skipIfNoVirtualizationConformance(t, err)
		t.Fatalf("ContainerRun failed: %v", err)
	}

	data, err := rt.ContainerInspect(ctx, name)
	if err != nil {
		t.Fatalf("ContainerInspect failed: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("ContainerInspect returned invalid JSON: %s", data)
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to decode ContainerInspect JSON: %v", err)
	}
	if !hasIdentifiableField(decoded) {
		t.Errorf("ContainerInspect JSON has no identifiable name/id field: %s", data)
	}

	netName := conformanceName("inspectnet")
	if err := rt.NetworkCreate(ctx, netName, nil); err != nil {
		t.Fatalf("NetworkCreate failed: %v", err)
	}
	t.Cleanup(func() { _ = rt.NetworkRemove(ctx, netName) })

	ndata, err := rt.NetworkInspect(ctx, netName)
	if err != nil {
		t.Fatalf("NetworkInspect failed: %v", err)
	}
	if !json.Valid(ndata) {
		t.Fatalf("NetworkInspect returned invalid JSON: %s", ndata)
	}
	var ndecoded any
	if err := json.Unmarshal(ndata, &ndecoded); err != nil {
		t.Fatalf("failed to decode NetworkInspect JSON: %v", err)
	}
	if !hasIdentifiableField(ndecoded) {
		t.Errorf("NetworkInspect JSON has no identifiable name/id field: %s", ndata)
	}

	volName := conformanceName("inspectvol")
	if err := rt.VolumeCreate(ctx, volName); err != nil {
		t.Fatalf("VolumeCreate failed: %v", err)
	}
	t.Cleanup(func() { _ = rt.VolumeRemove(ctx, volName) })

	vdata, err := rt.VolumeInspect(ctx, volName)
	if err != nil {
		t.Fatalf("VolumeInspect failed: %v", err)
	}
	if !json.Valid(vdata) {
		t.Fatalf("VolumeInspect returned invalid JSON: %s", vdata)
	}
	var vdecoded any
	if err := json.Unmarshal(vdata, &vdecoded); err != nil {
		t.Fatalf("failed to decode VolumeInspect JSON: %v", err)
	}
	if !hasIdentifiableField(vdecoded) {
		t.Errorf("VolumeInspect JSON has no identifiable name/id field: %s", vdata)
	}
}

func testIDConsistency(t *testing.T, rt engine.Runtime) {
	ctx := context.Background()
	name := conformanceName("idconsist")
	_ = rt.ContainerRemove(ctx, name, true)
	t.Cleanup(func() {
		_ = rt.ContainerStop(ctx, name)
		_ = rt.ContainerRemove(ctx, name, true)
	})

	args := []string{"--name", name, conformanceImage(), "sleep", "300"}
	id, err := rt.ContainerCreate(ctx, args)
	if err != nil {
		skipIfNoVirtualizationConformance(t, err)
		t.Fatalf("ContainerCreate failed: %v", err)
	}
	id = strings.TrimSpace(id)

	containers, err := rt.ContainerList(ctx, true)
	if err != nil {
		t.Fatalf("ContainerList failed: %v", err)
	}
	c, found := findContainer(containers, name)
	if !found {
		t.Fatalf("container %q not found in list", name)
	}

	// Documented relationship: the ID returned by ContainerCreate and the ID
	// reported by ContainerList must refer to the same container, but are not
	// required to be byte-identical strings — one may be a prefix of the
	// other (long vs. short form), or the list backend may report the name
	// where the create call reported an ID. Accept any of: exact match,
	// prefix relationship, or ID contains/contained-by relationship.
	related := id == c.ID ||
		strings.HasPrefix(id, c.ID) || strings.HasPrefix(c.ID, id) ||
		(id != "" && c.ID != "" && (strings.Contains(id, c.ID) || strings.Contains(c.ID, id)))
	if !related {
		t.Errorf("ContainerCreate ID %q has no documented relationship to ContainerList ID %q for container %q", id, c.ID, name)
	}
}

func testVolumeRoundtrip(t *testing.T, rt engine.Runtime) {
	ctx := context.Background()
	name := conformanceName("vol")

	if err := rt.VolumeCreate(ctx, name); err != nil {
		t.Fatalf("VolumeCreate failed: %v", err)
	}
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			_ = rt.VolumeRemove(ctx, name)
		}
	})

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
		// On hosted CI runners without Virtualization.framework, Apple's
		// `container volume create` reports success but nothing persists.
		// Probe before failing so we distinguish "backend broken" from
		// "this machine can't run VMs at all". The probe only costs a
		// container boot on the failure path.
		skipIfNoVirtualizationConformance(t, probeVirtualization(rt))
		t.Fatalf("volume %q not found in VolumeList after create", name)
	}

	if err := rt.VolumeRemove(ctx, name); err != nil {
		t.Fatalf("VolumeRemove failed: %v", err)
	}
	cleaned = true

	volumes, err = rt.VolumeList(ctx)
	if err != nil {
		t.Fatalf("VolumeList after remove failed: %v", err)
	}
	for _, v := range volumes {
		if v.Name == name {
			t.Errorf("volume %q still present in VolumeList after remove", name)
		}
	}
}

func testNetworkRoundtrip(t *testing.T, rt engine.Runtime) {
	ctx := context.Background()
	name := conformanceName("net")
	labels := map[string]string{"com.docker.compose.project": "conformance", "com.docker.compose.network": name}

	if err := rt.NetworkCreate(ctx, name, labels); err != nil {
		t.Fatalf("NetworkCreate failed: %v", err)
	}
	cleaned := false
	t.Cleanup(func() {
		if !cleaned {
			_ = rt.NetworkRemove(ctx, name)
		}
	})

	networks, err := rt.NetworkList(ctx)
	if err != nil {
		t.Fatalf("NetworkList failed: %v", err)
	}
	var found *engine.NetworkInfo
	for i := range networks {
		if networks[i].Name == name {
			found = &networks[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("network %q not found in NetworkList after create", name)
	}

	// Label survival is backend-dependent, and `network ls` JSON omits
	// labels on both docker and nerdctl — inspect is the path that must
	// carry them (it's what compose reads when deciding whether to adopt a
	// pre-existing network). Assert via inspect where the backend supports
	// labels; treat gaps as a DIVERGENCE rather than a failure.
	switch rt.(type) {
	case *engine.NerdctlRuntime:
		ndata, ierr := rt.NetworkInspect(ctx, name)
		if ierr != nil {
			t.Errorf("NetworkInspect after create failed: %v", ierr)
		} else if !strings.Contains(string(ndata), "com.docker.compose.project") {
			t.Errorf("DIVERGENCE: nerdctl NetworkInspect did not surface labels set at create time: %s", ndata)
		}
	default:
		t.Logf("DIVERGENCE (documented, not failed): backend %T's label survival is unverified/unsupported; skipping label assertion", rt)
	}

	if err := rt.NetworkRemove(ctx, name); err != nil {
		t.Fatalf("NetworkRemove failed: %v", err)
	}
	cleaned = true

	networks, err = rt.NetworkList(ctx)
	if err != nil {
		t.Fatalf("NetworkList after remove failed: %v", err)
	}
	for _, n := range networks {
		if n.Name == name {
			t.Errorf("network %q still present in NetworkList after remove", name)
		}
	}
}

func testExecStreamStdin(t *testing.T, rt engine.Runtime) {
	ctx := context.Background()
	name := conformanceName("execstdin")
	_ = rt.ContainerRemove(ctx, name, true)
	t.Cleanup(func() {
		_ = rt.ContainerStop(ctx, name)
		_ = rt.ContainerRemove(ctx, name, true)
	})

	args := []string{"-d", "--name", name, conformanceImage(), "sleep", "300"}
	if err := rt.ContainerRun(ctx, args, false); err != nil {
		skipIfNoVirtualizationConformance(t, err)
		t.Fatalf("ContainerRun failed: %v", err)
	}

	// Give the container a moment to be exec-able right after run.
	var stdout, stderr io.ReadCloser
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		// "-i" mirrors buildExecArgs (api/containers.go): real API exec
		// callers always pass it, and without it Apple's `container exec`
		// doesn't connect stdin at all.
		stdout, stderr, err = rt.ExecStreamStdin(ctx, strings.NewReader("hello\n"), "exec", "-i", name, "cat")
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		t.Fatalf("ExecStreamStdin failed: %v", err)
	}
	defer stdout.Close()
	defer stderr.Close()

	out, err := io.ReadAll(stdout)
	if err != nil {
		t.Fatalf("reading stdout failed: %v", err)
	}
	if got := strings.TrimRight(string(out), "\n"); got != "hello" {
		errBytes, _ := io.ReadAll(stderr)
		t.Errorf("ExecStreamStdin roundtrip mismatch: got %q, want %q (stderr: %s)", got, "hello", errBytes)
	}
}

func testEmptyListsNoError(t *testing.T, rt engine.Runtime) {
	ctx := context.Background()
	if _, err := rt.ContainerList(ctx, true); err != nil {
		t.Errorf("ContainerList errored: %v", err)
	}
	if _, err := rt.ImageList(ctx); err != nil {
		t.Errorf("ImageList errored: %v", err)
	}
	if _, err := rt.NetworkList(ctx); err != nil {
		t.Errorf("NetworkList errored: %v", err)
	}
	if _, err := rt.VolumeList(ctx); err != nil {
		t.Errorf("VolumeList errored: %v", err)
	}
}
