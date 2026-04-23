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

func TestRcAlreadyPointsAt_CommentedOutIgnored(t *testing.T) {
	content := "# export DOCKER_HOST=unix:///Users/me/.gocker/gocker.sock\n"
	if rcAlreadyPointsAt(content, "unix:///Users/me/.gocker/gocker.sock") {
		t.Error("commented-out export should not count")
	}
}

func TestRcAlreadyPointsAt_QuotedForms(t *testing.T) {
	target := "unix:///Users/me/.gocker/gocker.sock"
	cases := []struct {
		name    string
		content string
	}{
		{"unquoted", `export DOCKER_HOST=unix:///Users/me/.gocker/gocker.sock`},
		{"double-quoted", `export DOCKER_HOST="unix:///Users/me/.gocker/gocker.sock"`},
		{"single-quoted", `export DOCKER_HOST='unix:///Users/me/.gocker/gocker.sock'`},
		{"fish -gx unquoted", `set -gx DOCKER_HOST unix:///Users/me/.gocker/gocker.sock`},
		{"fish -gx quoted", `set -gx DOCKER_HOST "unix:///Users/me/.gocker/gocker.sock"`},
		{"fish -x quoted", `set -x DOCKER_HOST "unix:///Users/me/.gocker/gocker.sock"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !rcAlreadyPointsAt(tc.content+"\n", target) {
				t.Errorf("should match: %s", tc.content)
			}
		})
	}
}

func TestRcAlreadyPointsAt_DifferentSocketNotMatched(t *testing.T) {
	content := `export DOCKER_HOST="unix:///var/run/docker.sock"` + "\n"
	if rcAlreadyPointsAt(content, "unix:///Users/me/.gocker/gocker.sock") {
		t.Error("different socket should not count as matching")
	}
}

func TestInstallShellBlock_CreatesFileWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	rc := filepath.Join(tmp, "fish-subdir", "config.fish") // parent dir does not exist yet

	changed, err := InstallShellBlock(rc, "fish", "/Users/me/.gocker/gocker.sock")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Error("expected changed=true on fresh install")
	}
	data, err := os.ReadFile(rc)
	if err != nil {
		t.Fatalf("rc file not created: %v", err)
	}
	if !strings.Contains(string(data), "set -gx DOCKER_HOST") {
		t.Errorf("fish syntax missing: %s", data)
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
