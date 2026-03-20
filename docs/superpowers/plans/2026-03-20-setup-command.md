# `gocker setup` Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `gocker setup` command that validates macOS prerequisites, installs Apple Container from GitHub releases, initializes the system, and creates `~/.gocker/`. Add a pre-flight binary check to all other commands.

**Architecture:** New `cmd/setup.go` command runs sequential prerequisite checks and installation. `engine.Validate()` provides a lightweight binary-exists check reused by all other commands via a shared wrapper in `cmd/root.go`.

**Tech Stack:** Go standard library only (`net/http`, `os/exec`, `encoding/json`, `runtime`)

**Spec:** `docs/superpowers/specs/2026-03-20-setup-command-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `engine/engine.go` | Modify | Add `Validate() error` method |
| `engine/engine_test.go` | Create | Tests for `Validate()` |
| `cmd/setup.go` | Create | The `gocker setup` command — prerequisite checks, install, init |
| `cmd/setup_test.go` | Create | Tests for setup helper functions (version parsing, asset matching) |
| `cmd/root.go` | Modify | Register setup command, add `validateEngine` wrapper |

---

### Task 1: Add `Engine.Validate()` method

**Files:**
- Modify: `engine/engine.go:12-21`
- Create: `engine/engine_test.go`

- [ ] **Step 1: Write the failing test for Validate**

In `engine/engine_test.go`:

```go
package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_BinaryExists(t *testing.T) {
	// Create a temp file to act as the binary
	tmp := filepath.Join(t.TempDir(), "container")
	if err := os.WriteFile(tmp, []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}

	eng := New(tmp)
	if err := eng.Validate(); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidate_BinaryMissing(t *testing.T) {
	eng := New("/nonexistent/path/container")
	err := eng.Validate()
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
	expected := "Apple Container CLI not found at /nonexistent/path/container. Run 'gocker setup' to install it."
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/adrian/Projects/lunguini/gocker && go test ./engine/ -run TestValidate -v`
Expected: FAIL — `eng.Validate` does not exist

- [ ] **Step 3: Implement Validate method**

Add to `engine/engine.go` after the `New` function (after line 21):

```go
func (e *Engine) Validate() error {
	if _, err := os.Stat(e.Binary); os.IsNotExist(err) {
		return fmt.Errorf("Apple Container CLI not found at %s. Run 'gocker setup' to install it.", e.Binary)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/adrian/Projects/lunguini/gocker && go test ./engine/ -run TestValidate -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add engine/engine.go engine/engine_test.go
git commit -m "feat(engine): add Validate method to check container binary exists"
```

---

### Task 2: Create `gocker setup` command

**Files:**
- Create: `cmd/setup.go`
- Create: `cmd/setup_test.go`

This is the largest task. We'll build it incrementally: first the helper functions with tests, then wire them into the command action.

- [ ] **Step 1: Write tests for helper functions**

In `cmd/setup_test.go`:

```go
package cmd

import (
	"testing"
)

func TestParseMacOSVersion(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"26.0", 26, false},
		{"26.1.1", 26, false},
		{"26", 26, false},
		{"15.4.1", 15, false},
		{"", 0, true},
		{"abc", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseMacOSVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMacOSVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseMacOSVersion(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestFindInstallerAsset(t *testing.T) {
	assets := []ghAsset{
		{Name: "container-0.10.0-installer-signed.pkg", DownloadURL: "https://example.com/pkg"},
		{Name: "container-dSYM.zip", DownloadURL: "https://example.com/dsym"},
		{Name: "Source code (zip)", DownloadURL: "https://example.com/src"},
	}

	asset, err := findInstallerAsset(assets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if asset.Name != "container-0.10.0-installer-signed.pkg" {
		t.Errorf("expected pkg asset, got %q", asset.Name)
	}
}

func TestFindInstallerAsset_NotFound(t *testing.T) {
	assets := []ghAsset{
		{Name: "container-dSYM.zip", DownloadURL: "https://example.com/dsym"},
	}

	_, err := findInstallerAsset(assets)
	if err == nil {
		t.Fatal("expected error when no installer asset found")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/adrian/Projects/lunguini/gocker && go test ./cmd/ -run "TestParseMacOSVersion|TestFindInstallerAsset" -v`
Expected: FAIL — functions don't exist yet

- [ ] **Step 3: Implement setup.go**

Create `cmd/setup.go`:

```go
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

const (
	minMacOSVersion = 26
	releasesURL     = "https://api.github.com/repos/apple/container/releases/latest"
	releasesPageURL = "https://github.com/apple/container/releases"
)

type ghRelease struct {
	Assets []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

func newSetupCmd(eng *engine.Engine) *cli.Command {
	return &cli.Command{
		Name:  "setup",
		Usage: "Check prerequisites and install Apple Container",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runSetup(ctx, eng)
		},
	}
}

func runSetup(ctx context.Context, eng *engine.Engine) error {
	// Step 1: Check macOS version
	fmt.Print("Checking macOS version... ")
	verOut, err := exec.CommandContext(ctx, "sw_vers", "-productVersion").Output()
	if err != nil {
		return fmt.Errorf("failed to get macOS version: %w", err)
	}
	major, err := parseMacOSVersion(strings.TrimSpace(string(verOut)))
	if err != nil {
		return fmt.Errorf("failed to parse macOS version: %w", err)
	}
	if major < minMacOSVersion {
		fmt.Println("X")
		return fmt.Errorf("macOS %d+ required, found macOS %d", minMacOSVersion, major)
	}
	fmt.Printf("macOS %d OK\n", major)

	// Step 2: Check architecture
	fmt.Print("Checking architecture... ")
	if runtime.GOARCH != "arm64" {
		fmt.Println("X")
		return fmt.Errorf("Apple Silicon (arm64) required, found %s", runtime.GOARCH)
	}
	fmt.Println("arm64 OK")

	// Step 3: Check/install container binary
	fmt.Print("Checking Apple Container CLI... ")
	if _, err := os.Stat(eng.Binary); os.IsNotExist(err) {
		fmt.Println("not found")
		if err := installContainer(ctx); err != nil {
			return err
		}
	} else {
		fmt.Println("OK")
	}

	// Step 4: Run container system start
	fmt.Print("Initializing container system... ")
	if _, stderr, err := eng.Exec(ctx, "system", "start"); err != nil {
		fmt.Println("X")
		return fmt.Errorf("container system start failed: %s", string(stderr))
	}
	fmt.Println("OK")

	// Step 5: Create ~/.gocker/ directory
	fmt.Print("Creating gocker directory... ")
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	gockerDir := filepath.Join(home, ".gocker")
	if err := os.MkdirAll(gockerDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", gockerDir, err)
	}
	fmt.Println("OK")

	// Step 6: Summary
	fmt.Println("\nGocker is ready! Run 'gocker ps' to get started.")
	return nil
}

func installContainer(ctx context.Context) error {
	fmt.Println("Downloading Apple Container from GitHub releases...")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch releases: %w\nInstall manually from %s", err, releasesPageURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned %s\nInstall manually from %s", resp.Status, releasesPageURL)
	}

	var release ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("failed to parse release JSON: %w", err)
	}

	asset, err := findInstallerAsset(release.Assets)
	if err != nil {
		return err
	}

	fmt.Printf("Downloading %s...\n", asset.Name)
	tmpFile, err := os.CreateTemp("", "container-*.pkg")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}
	dlResp, err := http.DefaultClient.Do(dlReq)
	if err != nil {
		return fmt.Errorf("failed to download installer: %w", err)
	}
	defer dlResp.Body.Close()

	if _, err := io.Copy(tmpFile, dlResp.Body); err != nil {
		return fmt.Errorf("failed to write installer: %w", err)
	}
	tmpFile.Close()

	fmt.Println("Installing Apple Container requires administrator privileges.")
	installCmd := exec.CommandContext(ctx, "sudo", "installer", "-pkg", tmpFile.Name(), "-target", "/")
	installCmd.Stdin = os.Stdin
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("installer failed: %w", err)
	}

	fmt.Println("Apple Container installed successfully.")
	return nil
}

func parseMacOSVersion(version string) (int, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return 0, fmt.Errorf("empty version string")
	}
	parts := strings.Split(version, ".")
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid version %q: %w", version, err)
	}
	return major, nil
}

func findInstallerAsset(assets []ghAsset) (ghAsset, error) {
	for _, a := range assets {
		if strings.HasSuffix(a.Name, "-installer-signed.pkg") {
			return a, nil
		}
	}
	return ghAsset{}, fmt.Errorf("no installer package found in release assets\nInstall manually from %s", releasesPageURL)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/adrian/Projects/lunguini/gocker && go test ./cmd/ -run "TestParseMacOSVersion|TestFindInstallerAsset" -v`
Expected: PASS

- [ ] **Step 5: Verify the full project builds**

Run: `cd /Users/adrian/Projects/lunguini/gocker && go build ./...`
Expected: compiles with no errors

- [ ] **Step 6: Commit**

```bash
git add cmd/setup.go cmd/setup_test.go
git commit -m "feat(cmd): add gocker setup command for Apple Container installation"
```

---

### Task 3: Add pre-flight validation to `root.go`

**Files:**
- Modify: `cmd/root.go`

Now that `cmd/setup.go` exists (Task 2), we can register it and add the `Before` hook. Rather than modifying every command file (18 files), we use urfave/cli v3's `Before` hook on the root command to run validation before any subcommand.

- [ ] **Step 1: Update root.go with Before hook and register setup command**

Replace `cmd/root.go` with:

```go
package cmd

import (
	"context"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

func NewApp() *cli.Command {
	eng := engine.New("")

	return &cli.Command{
		Name:    "gocker",
		Usage:   "Docker-compatible CLI for Apple Container on macOS",
		Version: "0.1.0",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "format",
				Usage: "Output format (table, json)",
				Value: "table",
			},
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "Enable debug output",
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) error {
			// Skip validation for setup (it installs the binary) and
			// when no subcommand is given (help/version output)
			first := cmd.Args().First()
			if first == "setup" || first == "" {
				return nil
			}
			return eng.Validate()
		},
		Commands: []*cli.Command{
			newRunCmd(eng),
			newPsCmd(eng),
			newStopCmd(eng),
			newRmCmd(eng),
			newStartCmd(eng),
			newExecCmd(eng),
			newLogsCmd(eng),
			newInspectCmd(eng),
			newPullCmd(eng),
			newImagesCmd(eng),
			newRmiCmd(eng),
			newBuildCmd(eng),
			newPushCmd(eng),
			newNetworkCmd(eng),
			newVolumeCmd(eng),
			newSystemCmd(eng),
			newDaemonCmd(eng),
			newSandboxCmd(eng),
			newSetupCmd(eng),
		},
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/adrian/Projects/lunguini/gocker && go build ./...`
Expected: compiles successfully

- [ ] **Step 3: Commit**

```bash
git add cmd/root.go
git commit -m "feat(cmd): add pre-flight engine validation with setup command bypass"
```

---

### Task 4: Integration verification

**Files:** None (verification only)

- [ ] **Step 1: Run all tests**

Run: `cd /Users/adrian/Projects/lunguini/gocker && go test ./... -v`
Expected: All tests pass

- [ ] **Step 2: Run linter**

Run: `cd /Users/adrian/Projects/lunguini/gocker && make lint`
Expected: No lint errors (or only pre-existing ones)

- [ ] **Step 3: Build and smoke test help output**

Run: `cd /Users/adrian/Projects/lunguini/gocker && go build -o gocker . && ./gocker setup --help`
Expected: Shows "Check prerequisites and install Apple Container"

- [ ] **Step 4: Test pre-flight error message**

Run: `cd /Users/adrian/Projects/lunguini/gocker && ./gocker ps`
Expected: If `/usr/local/bin/container` doesn't exist, shows: "Apple Container CLI not found at /usr/local/bin/container. Run 'gocker setup' to install it."

- [ ] **Step 5: Final commit (if any lint/build fixes needed)**

```bash
git add -A
git commit -m "fix: address lint and build issues from setup command"
```
