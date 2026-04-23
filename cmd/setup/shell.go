package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	shellMarkerStart = "# >>> gocker setup >>>"
	shellMarkerEnd   = "# <<< gocker setup <<<"
)

// DetectShell returns "bash", "zsh", "fish", or "" for unsupported shells.
func DetectShell(shellPath string) string {
	base := filepath.Base(shellPath)
	switch base {
	case "bash", "zsh", "fish":
		return base
	}
	return ""
}

// ShellRCPath returns the rc file to modify for the given shell.
func ShellRCPath(shell, home string) string {
	switch shell {
	case "bash":
		return filepath.Join(home, ".bashrc")
	case "zsh":
		return filepath.Join(home, ".zshrc")
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish")
	}
	return ""
}

// shellExport returns the shell-specific export syntax.
func shellExport(shell, key, value string) string {
	if shell == "fish" {
		return fmt.Sprintf(`set -gx %s "%s"`, key, value)
	}
	return fmt.Sprintf(`export %s="%s"`, key, value)
}

// InstallShellBlock inserts (or updates) a sentinel-wrapped block in the given
// rc file that exports DOCKER_HOST and testcontainers overrides pointing at
// the gocker socket. Returns (changed, err).
//
// Idempotency rules:
//  1. If the rc file already exports DOCKER_HOST pointing at this socket
//     (outside our block), leave everything alone.
//  2. If our block already exists and matches the desired content, no change.
//  3. Otherwise, remove any existing block and append a fresh one.
func InstallShellBlock(rcPath, shell, socket string) (bool, error) {
	existing, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("reading %s: %w", rcPath, err)
	}
	content := string(existing)

	dockerHost := "unix://" + socket
	if rcAlreadyPointsAt(content, dockerHost) && !hasGockerBlock(content) {
		return false, nil
	}

	desired := buildShellBlock(shell, socket)

	// Strip any existing block.
	cleaned := stripGockerBlock(content)

	// If the cleaned content + desired block equals existing content, no-op.
	final := cleaned
	if !strings.HasSuffix(final, "\n") && final != "" {
		final += "\n"
	}
	final += desired + "\n"

	if final == content {
		return false, nil
	}

	if err := os.MkdirAll(filepath.Dir(rcPath), 0o755); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", filepath.Dir(rcPath), err)
	}
	if err := os.WriteFile(rcPath, []byte(final), 0o644); err != nil {
		return false, fmt.Errorf("writing %s: %w", rcPath, err)
	}
	return true, nil
}

func buildShellBlock(shell, socket string) string {
	var b strings.Builder
	b.WriteString(shellMarkerStart + "\n")
	b.WriteString("# Managed by 'gocker setup'. Edit via 'gocker setup' to stay consistent.\n")
	b.WriteString(shellExport(shell, "DOCKER_HOST", "unix://"+socket) + "\n")
	b.WriteString(shellExport(shell, "TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE", socket) + "\n")
	b.WriteString(shellMarkerEnd)
	return b.String()
}

func hasGockerBlock(content string) bool {
	return strings.Contains(content, shellMarkerStart) && strings.Contains(content, shellMarkerEnd)
}

var blockRe = regexp.MustCompile(`(?s)` + regexp.QuoteMeta(shellMarkerStart) + `.*?` + regexp.QuoteMeta(shellMarkerEnd) + `\n?`)

func stripGockerBlock(content string) string {
	return blockRe.ReplaceAllString(content, "")
}

// rcAlreadyPointsAt returns true if an export outside our managed block
// already sets DOCKER_HOST to the given target.
func rcAlreadyPointsAt(content, dockerHost string) bool {
	stripped := stripGockerBlock(content)
	return strings.Contains(stripped, "DOCKER_HOST="+dockerHost) ||
		strings.Contains(stripped, "DOCKER_HOST=\""+dockerHost+"\"") ||
		strings.Contains(stripped, "DOCKER_HOST "+dockerHost) // fish set-style
}
