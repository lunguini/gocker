# Design: `gocker setup` Command

## Overview

Add a `gocker setup` command that validates system prerequisites, installs Apple's `container` CLI from GitHub releases if missing, initializes the container system, and creates the `~/.gocker/` directory. Also add a lightweight pre-flight check to all other commands so users get a clear error if setup hasn't been run.

## Motivation

Gocker depends on Apple's `container` CLI at `/usr/local/bin/container`, but currently has zero validation. If the binary is missing, users get a raw "no such file or directory" OS error from whichever command they happen to run first. A setup command provides proper onboarding, and a pre-flight check catches the missing-binary case everywhere else.

## Design

### 1. `gocker setup` command (`cmd/setup.go`)

A new command registered in `cmd/root.go` via `newSetupCmd(eng)`. Runs these steps sequentially, skipping any already satisfied:

**Step 1: Check macOS version**
- Run `sw_vers -productVersion` and parse the major version as an integer (e.g., `"26.0"` or `"26.1.1"` → `26`). Use `strings.Split` on `"."` and `strconv.Atoi` on the first element.
- Require `major >= 26`. Fail with a clear message if not met

**Step 2: Check architecture**
- Check `runtime.GOARCH == "arm64"`
- Fail with a clear message if running on Intel

**Step 3: Check/install `container` binary**
- Check if `/usr/local/bin/container` exists via `os.Stat`
- If missing:
  - Query `https://api.github.com/repos/apple/container/releases/latest` for the latest release
  - Parse the JSON response: iterate the `assets` array, match an asset whose `name` matches the pattern `container-*-installer-signed.pkg` (e.g., `container-0.10.0-installer-signed.pkg`). If no matching asset is found, fail with a message linking to the releases page for manual install.
  - Download the matched asset's `browser_download_url` to a temp file (e.g., `os.CreateTemp("", "container-*.pkg")`)
  - Print a message explaining why elevated privileges are needed ("Installing Apple Container requires administrator privileges.")
  - Run `sudo installer -pkg <tempfile> -target /` via `os/exec`. The macOS `installer` command handles placing the binary and setting correct permissions.
  - Clean up the temp file
- If already present, skip with a checkmark

**Step 4: Run `container system start`**
- Execute using the full path `eng.Binary` (i.e., `/usr/local/bin/container system start`) rather than relying on PATH, consistent with how the rest of the engine operates
- This is idempotent — Apple's CLI handles re-runs gracefully

**Step 5: Create `~/.gocker/` directory**
- Use `os.UserHomeDir()` to get the home directory path, then `filepath.Join(home, ".gocker")` to construct the full path. Call `os.MkdirAll` on that path with `0755` permissions.
- Idempotent — no-op if it already exists

**Step 6: Print summary**
- Print status for each step: checkmark if already done or successfully completed, X if failed
- Final message confirming gocker is ready to use

### 2. Pre-flight check (`engine/engine.go`)

Add a `Validate() error` method to `Engine`:

```go
func (e *Engine) Validate() error {
    if _, err := os.Stat(e.Binary); os.IsNotExist(err) {
        return fmt.Errorf("Apple Container CLI not found at %s. Run 'gocker setup' to install it.", e.Binary)
    }
    return nil
}
```

This method is called at the start of command actions that use the engine. The `setup` command explicitly does **not** call `Validate()` since it is the command responsible for installing the binary. The check is intentionally minimal — just "does the binary exist?" — to keep it fast and avoid network calls on every invocation.

### 3. File changes

**New files:**
- `cmd/setup.go` — the setup command

**Modified files:**
- `cmd/root.go` — register `newSetupCmd(eng)` in Commands slice
- `engine/engine.go` — add `Validate() error` method
- Command action functions — add a shared `validateEngine` wrapper function in `cmd/root.go` that calls `eng.Validate()` and returns early with the error. Each command action (except `setup`) calls this wrapper at the top. This avoids repetition and ensures consistent behavior.

### 4. Dependencies

No new external dependencies. Uses standard library only:
- `net/http` — GitHub API and binary download
- `os` — file operations
- `os/exec` — running `sw_vers` and `container system start`
- `runtime` — architecture check
- `encoding/json` — parsing GitHub API response

### 5. Idempotency

Every step checks current state before acting. Running `gocker setup` when everything is already configured produces only checkmarks and a "ready" message. No re-downloads, no re-initialization.

### 6. Error handling

- macOS version or architecture mismatch: fail immediately with explanation, don't attempt partial setup
- GitHub API failure (rate limit, network): fail with the HTTP error and suggest manual install from the releases page
- `sudo installer` failure: show the error output. If permission denied, explain that the installer requires administrator privileges
- `container system start` failure: show the stderr output from the command

## Out of scope

- Homebrew installation (no formula exists for Apple Container)
- Updating an already-installed `container` binary (`gocker setup` only installs if missing)
- Config file persistence (planned `koanf` config is not yet implemented)
- Version checking of the installed `container` binary
