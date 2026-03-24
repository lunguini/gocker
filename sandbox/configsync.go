package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ConfigMount struct {
	HostPath      string
	ContainerPath string
	ReadOnly      bool
	Optional      bool
}

func ClaudeConfigMounts(syncConfig, syncState, managedSettings bool) []ConfigMount {
	home, _ := os.UserHomeDir()
	var mounts []ConfigMount

	sandboxHome := "/home/sandbox"

	if syncConfig {
		// Mount host settings as host-settings.json; entrypoint merges
		// them with the baked-in sandbox settings at startup
		mounts = append(mounts,
			ConfigMount{filepath.Join(home, ".claude", "settings.json"), sandboxHome + "/.claude/host-settings.json", true, true},
			ConfigMount{filepath.Join(home, ".claude.md"), sandboxHome + "/.claude.md", true, true},
		)
	}

	if syncState {
		mounts = append(mounts,
			ConfigMount{filepath.Join(home, ".claude.json"), sandboxHome + "/.claude.json", false, true},
		)
	}

	if managedSettings {
		mounts = append(mounts,
			ConfigMount{"/Library/Application Support/ClaudeCode/managed-settings.json", "/etc/claude-code/managed-settings.json", true, true},
			ConfigMount{"/Library/Application Support/ClaudeCode/managed-mcp.json", "/etc/claude-code/managed-mcp.json", true, true},
		)
	}

	return mounts
}

func CodexConfigMounts(syncConfig, syncState, managedSettings bool) []ConfigMount {
	home, _ := os.UserHomeDir()
	var mounts []ConfigMount

	if syncConfig {
		mounts = append(mounts,
			ConfigMount{filepath.Join(home, ".codex"), "/root/.codex", true, true},
		)
	}

	return mounts
}

var agentConfigFuncs = map[string]func(syncConfig, syncState, managedSettings bool) []ConfigMount{
	"claude": ClaudeConfigMounts,
	"codex":  CodexConfigMounts,
}

// SessionSyncMounts returns mounts that sync the host's Claude Code session
// into the VM so /resume works across host and sandbox.
//
// Claude Code stores sessions at ~/.claude/projects/<path-hash>/ where
// path-hash is the workspace path with "/" replaced by "-".
// Host: ~/.claude/projects/-Users-adrian-Projects-myapp/
// VM:   ~/.claude/projects/-workspace/
//
// We mount the host session dir at the VM's expected session path.
func SessionSyncMounts(hostWorkspace, containerWorkspace string) []ConfigMount {
	home, _ := os.UserHomeDir()
	sandboxHome := "/home/sandbox"

	hostHash := pathToHash(hostWorkspace)
	containerHash := pathToHash(containerWorkspace)

	hostSessionDir := filepath.Join(home, ".claude", "projects", hostHash)
	containerSessionDir := sandboxHome + "/.claude/projects/" + containerHash

	// Create host session dir if it doesn't exist so sandbox sessions
	// persist back to the host (enables /resume outside the sandbox)
	os.MkdirAll(hostSessionDir, 0755)

	return []ConfigMount{
		{hostSessionDir, containerSessionDir, false, false},
	}
}

// pathToHash converts a workspace path to Claude Code's session directory name.
// /Users/adrian/Projects/myapp → -Users-adrian-Projects-myapp
func pathToHash(path string) string {
	path = filepath.Clean(path)
	return strings.ReplaceAll(path, "/", "-")
}

func GetConfigMounts(agent string, syncConfig, syncState, managedSettings bool) []ConfigMount {
	if fn, ok := agentConfigFuncs[agent]; ok {
		return fn(syncConfig, syncState, managedSettings)
	}
	return nil
}

func GenerateMountFlags(mounts []ConfigMount) []string {
	var flags []string
	for _, m := range mounts {
		if m.Optional {
			if _, err := os.Stat(m.HostPath); os.IsNotExist(err) {
				continue
			}
		}
		flag := fmt.Sprintf("%s:%s", m.HostPath, m.ContainerPath)
		if m.ReadOnly {
			flag += ":ro"
		}
		flags = append(flags, "-v", flag)
	}
	return flags
}
