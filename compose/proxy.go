package compose

import (
	"context"
	"os"
	"strings"

	"github.com/lunguini/gocker/engine"
	"github.com/lunguini/gocker/sharedvm"
)

// Proxy forwards compose commands to nerdctl compose inside a VM.
type Proxy struct {
	apple   engine.Runtime
	manager *sharedvm.Manager
}

// NewProxy creates a compose proxy that executes nerdctl compose inside the
// VM managed by mgr.
func NewProxy(apple engine.Runtime, mgr *sharedvm.Manager) *Proxy {
	return &Proxy{apple: apple, manager: mgr}
}

// Exec runs `nerdctl compose <args>` inside the VM.
// Args should be the raw compose arguments (e.g., ["-f", "file.yaml", "build"]).
// Host paths in -f and --project-directory are translated to VM-internal paths.
func (p *Proxy) Exec(ctx context.Context, args []string, interactive bool) error {
	if err := p.manager.EnsureRunning(ctx); err != nil {
		return err
	}

	translated := p.translateArgs(args)

	// If no explicit file or project-directory flag, inject --project-directory
	// so nerdctl finds compose files in the translated host CWD.
	mounts := p.manager.Mounts()
	if !hasFileFlag(translated) {
		if cwd, err := os.Getwd(); err == nil {
			if vmCwd, ok := sharedvm.TranslatePath(cwd, mounts); ok {
				translated = append([]string{"--project-directory", vmCwd}, translated...)
			}
		}
	}

	// Build: container exec -i <vm> nerdctl compose <args>
	// Use -i only (not -t) for the outer exec. Apple's container exec with -t
	// conflicts with nerdctl's -T flag and causes "Operation not supported by
	// device" errors for non-interactive compose commands.
	execArgs := []string{"exec", "-i"}

	// Forward environment variables that compose files may reference.
	// Filter out system vars and translate host paths in values.
	for _, env := range os.Environ() {
		key, val, _ := strings.Cut(env, "=")
		if shouldForwardEnv(key) {
			val, _ = sharedvm.TranslatePath(val, mounts)
			execArgs = append(execArgs, "-e", key+"="+val)
		}
	}

	execArgs = append(execArgs, p.manager.Name(), "nerdctl", "compose")
	execArgs = append(execArgs, translated...)

	return p.apple.ExecInteractive(ctx, execArgs...)
}

// translateArgs rewrites host paths in flags and positional args
// to VM-internal paths.
func (p *Proxy) translateArgs(args []string) []string {
	mounts := p.manager.Mounts()
	result := make([]string, len(args))
	copy(result, args)

	for i := range result {
		// Translate flag values
		if i > 0 {
			switch result[i-1] {
			case "-f", "--file", "--project-directory":
				result[i], _ = sharedvm.TranslatePath(result[i], mounts)
				continue
			}
		}
		// Translate positional args that look like absolute host paths.
		// Skip container:path specs (used by compose cp).
		if strings.HasPrefix(result[i], "/") && !strings.Contains(result[i], ":") {
			result[i], _ = sharedvm.TranslatePath(result[i], mounts)
		}
		// Handle source/. paths (compose cp dir copy syntax)
		if strings.HasPrefix(result[i], "/") && strings.HasSuffix(result[i], "/.") {
			base := strings.TrimSuffix(result[i], "/.")
			translated, _ := sharedvm.TranslatePath(base, mounts)
			result[i] = translated + "/."
		}
	}
	return result
}

// hasFileFlag returns true if args contain -f, --file, or --project-directory.
func hasFileFlag(args []string) bool {
	for _, a := range args {
		switch a {
		case "-f", "--file", "--project-directory":
			return true
		}
		if strings.HasPrefix(a, "-f=") || strings.HasPrefix(a, "--file=") || strings.HasPrefix(a, "--project-directory=") {
			return true
		}
	}
	return false
}

// shouldForwardEnv returns true for environment variables that should be
// forwarded into the VM for compose file variable substitution.
func shouldForwardEnv(key string) bool {
	// Skip system/shell vars that don't belong inside the VM.
	switch key {
	case "HOME", "USER", "LOGNAME", "SHELL", "TERM", "PATH",
		"LANG", "LC_ALL", "LC_CTYPE", "TMPDIR", "DISPLAY",
		"SSH_AUTH_SOCK", "SSH_AGENT_PID",
		"COLORTERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION",
		"XPC_FLAGS", "XPC_SERVICE_NAME",
		"__CF_USER_TEXT_ENCODING", "__CFBundleIdentifier",
		"COMMAND_MODE", "SECURITYSESSIONID",
		"Apple_PubSub_Socket_Render",
		"SHLVL", "OLDPWD", "PWD", "_":
		return false
	}
	// Skip macOS-specific prefixes
	if strings.HasPrefix(key, "DYLD_") || strings.HasPrefix(key, "MallocNano") {
		return false
	}
	return true
}
