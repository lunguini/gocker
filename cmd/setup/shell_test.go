package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectShellFromEnv(t *testing.T) {
	cases := map[string]string{
		"/bin/bash":              "bash",
		"/usr/local/bin/zsh":     "zsh",
		"/opt/homebrew/bin/fish": "fish",
		"/usr/bin/tcsh":          "",
	}
	for shell, want := range cases {
		if got := DetectShell(shell); got != want {
			t.Errorf("DetectShell(%q) = %q, want %q", shell, got, want)
		}
	}
}

func TestShellRCPath(t *testing.T) {
	home := "/tmp/home"
	cases := map[string]string{
		"bash": filepath.Join(home, ".bashrc"),
		"zsh":  filepath.Join(home, ".zshrc"),
		"fish": filepath.Join(home, ".config/fish/config.fish"),
	}
	for shell, want := range cases {
		if got := ShellRCPath(shell, home); got != want {
			t.Errorf("ShellRCPath(%q) = %q, want %q", shell, got, want)
		}
	}
}

func TestInstallShellBlockIdempotent(t *testing.T) {
	tmp := t.TempDir()
	rc := filepath.Join(tmp, ".zshrc")
	if err := os.WriteFile(rc, []byte("# existing content\nexport FOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	socket := "/Users/me/.gocker/gocker.sock"

	// First install.
	changed, err := InstallShellBlock(rc, "zsh", socket)
	if err != nil {
		t.Fatalf("first install: %v", err)
	}
	if !changed {
		t.Errorf("first install should report changed=true")
	}
	content, _ := os.ReadFile(rc)
	if !strings.Contains(string(content), ">>> gocker setup >>>") {
		t.Errorf("block marker missing: %s", content)
	}
	if !strings.Contains(string(content), `DOCKER_HOST="unix://`+socket+`"`) {
		t.Errorf("DOCKER_HOST not set correctly: %s", content)
	}

	// Second install with same socket — no change.
	changed, err = InstallShellBlock(rc, "zsh", socket)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if changed {
		t.Errorf("re-install with same socket should be a no-op")
	}
}

func TestInstallShellBlockUpdatesSocket(t *testing.T) {
	tmp := t.TempDir()
	rc := filepath.Join(tmp, ".bashrc")
	if err := os.WriteFile(rc, []byte("# pre-existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _ = InstallShellBlock(rc, "bash", "/old/sock")
	changed, _ := InstallShellBlock(rc, "bash", "/new/sock")
	if !changed {
		t.Errorf("socket change should report changed=true")
	}
	content, _ := os.ReadFile(rc)
	if strings.Contains(string(content), "/old/sock") {
		t.Errorf("old socket path not replaced: %s", content)
	}
	if !strings.Contains(string(content), "/new/sock") {
		t.Errorf("new socket path not written: %s", content)
	}
	if strings.Count(string(content), ">>> gocker setup >>>") != 1 {
		t.Errorf("expected exactly one block, got:\n%s", content)
	}
}

func TestInstallShellBlockRespectsExistingDOCKER_HOST(t *testing.T) {
	tmp := t.TempDir()
	rc := filepath.Join(tmp, ".zshrc")
	if err := os.WriteFile(rc, []byte(`export DOCKER_HOST=unix:///Users/me/.gocker/gocker.sock`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := InstallShellBlock(rc, "zsh", "/Users/me/.gocker/gocker.sock")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Errorf("should not write block if DOCKER_HOST already points at this socket")
	}
}

func TestShellExportSyntax(t *testing.T) {
	if got := shellExport("zsh", "DOCKER_HOST", "unix:///foo"); got != `export DOCKER_HOST="unix:///foo"` {
		t.Errorf("zsh: %q", got)
	}
	if got := shellExport("bash", "DOCKER_HOST", "unix:///foo"); got != `export DOCKER_HOST="unix:///foo"` {
		t.Errorf("bash: %q", got)
	}
	if got := shellExport("fish", "DOCKER_HOST", "unix:///foo"); got != `set -gx DOCKER_HOST "unix:///foo"` {
		t.Errorf("fish: %q", got)
	}
}
