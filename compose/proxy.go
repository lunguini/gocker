package compose

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
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

	// Ensure the host CWD is reachable inside the VM. Without this, nerdctl
	// compose either fails with "no configuration file provided" (no -f flag)
	// or hits "file not found" translating a relative -f path. Same behavior
	// as ContainerRun/ImageBuild for -v bind mounts.
	cwd := resolvedCwd()
	if cwd != "" {
		if _, ok := sharedvm.TranslatePath(cwd, p.manager.Mounts()); !ok {
			if parent, perr := sharedvm.ResolveMountParent(cwd); perr == nil {
				if err := p.manager.ExpandMounts(ctx, []string{parent}); err != nil {
					return err
				}
			}
		}
	}

	translated := p.translateArgs(args)

	// If no explicit file or project-directory flag, inject --project-directory
	// so nerdctl finds compose files in the translated host CWD.
	mounts := p.manager.Mounts()
	if !hasFileFlag(translated) && cwd != "" {
		if vmCwd, ok := sharedvm.TranslatePath(cwd, mounts); ok {
			translated = append([]string{"--project-directory", vmCwd}, translated...)
		}
	}

	// Build: container exec -i <vm> nerdctl compose <args>
	// Use -i only (not -t) for the outer exec. Apple's container exec with -t
	// conflicts with nerdctl's -T flag and causes "Operation not supported by
	// device" errors for non-interactive compose commands.
	execArgs := []string{"exec", "-i"}

	// Forward only environment variables the compose file(s) actually
	// reference (plus COMPOSE_*/DOCKER_* controls), instead of dumping the
	// whole host environment onto argv. This keeps unrelated secrets
	// (AWS_SECRET_ACCESS_KEY, GITHUB_TOKEN, ...) out of the VM's ps-visible
	// process table. Host paths in forwarded values are still translated.
	allow := referencedEnvVars(args, cwd)
	for _, env := range os.Environ() {
		key, val, _ := strings.Cut(env, "=")
		if shouldForwardEnv(key, allow) {
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

// resolvedCwd returns the current working directory with symlinks resolved
// (e.g. /tmp -> /private/tmp on macOS). VM mounts are always stored as
// symlink-resolved paths, so TranslatePath needs the same form to match.
func resolvedCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		return resolved
	}
	return cwd
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
// forwarded into the VM for compose file variable substitution. It is an
// allowlist: a var is forwarded only if the compose file(s) reference it, or
// it is a COMPOSE_*/DOCKER_* control variable that compose itself consumes.
func shouldForwardEnv(key string, referenced map[string]bool) bool {
	if referenced[key] {
		return true
	}
	return strings.HasPrefix(key, "COMPOSE_") || strings.HasPrefix(key, "DOCKER_")
}

// envVarRef matches ${VAR}, ${VAR:-default} and $VAR references in a compose
// file. The capture group is the bare variable name.
var envVarRef = regexp.MustCompile(`\$\{?([A-Za-z_][A-Za-z0-9_]*)`)

// referencedEnvVars scans the compose file(s) that will be used and returns
// the set of environment variable names referenced via ${VAR}/$VAR. When no
// -f/--file flag is given, it falls back to the default compose file names in
// cwd. Files that can't be read are skipped — over-scanning is harmless, the
// allowlist just won't gain entries it can't find.
func referencedEnvVars(args []string, cwd string) map[string]bool {
	set := map[string]bool{}
	for _, f := range composeFilesToScan(args, cwd) {
		if !filepath.IsAbs(f) && cwd != "" {
			f = filepath.Join(cwd, f)
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, m := range envVarRef.FindAllSubmatch(data, -1) {
			set[string(m[1])] = true
		}
	}
	return set
}

// composeFilesToScan resolves which compose files to scan for env references:
// explicit -f/--file values when present, otherwise the standard candidate
// names in cwd.
func composeFilesToScan(args []string, cwd string) []string {
	var files []string
	for i, a := range args {
		switch {
		case a == "-f" || a == "--file":
			if i+1 < len(args) {
				files = append(files, args[i+1])
			}
		case strings.HasPrefix(a, "-f="):
			files = append(files, strings.TrimPrefix(a, "-f="))
		case strings.HasPrefix(a, "--file="):
			files = append(files, strings.TrimPrefix(a, "--file="))
		}
	}
	if len(files) == 0 {
		files = []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"}
	}
	return files
}
