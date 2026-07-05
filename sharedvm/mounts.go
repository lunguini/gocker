package sharedvm

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultMounts returns mount mappings from workspace dirs.
// Each host dir is mounted at /host<dir> inside the VM.
// e.g., /Users/adrian → /host/Users/adrian
func DefaultMounts(workspaceDirs []string) map[string]string {
	mounts := make(map[string]string, len(workspaceDirs))
	for _, dir := range workspaceDirs {
		dir = filepath.Clean(dir)
		mounts[dir] = "/host" + dir
	}
	return mounts
}

// MountFlags returns -v flags for VM creation from the mount mapping.
// Hosts are sorted so the generated create args are deterministic across runs
// (a map iteration would otherwise reorder -v flags, adding noise when
// comparing or debugging VM create commands).
func MountFlags(mounts map[string]string) []string {
	hosts := make([]string, 0, len(mounts))
	for host := range mounts {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	flags := make([]string, 0, len(mounts)*2)
	for _, host := range hosts {
		flags = append(flags, "-v", host+":"+mounts[host])
	}
	return flags
}

// TranslatePath converts a host-absolute path to its VM-internal path.
// Returns (translated, true) if a mount covers the path, or (original, false) if not.
func TranslatePath(hostPath string, mounts map[string]string) (string, bool) {
	hostPath = filepath.Clean(hostPath)
	for host, vm := range mounts {
		if hostPath == host || strings.HasPrefix(hostPath, host+"/") {
			return vm + hostPath[len(host):], true
		}
	}
	return hostPath, false
}

// blockedMountRoots are directories too broad to auto-mount into the VM.
// The map is keyed by both the literal path and its symlink-resolved form so
// that e.g. /etc and /private/etc (macOS) are both rejected.
var blockedMountRoots = buildBlockedMountRoots([]string{
	"/",
	"/tmp",
	"/var",
	"/etc",
	"/private",
})

func buildBlockedMountRoots(roots []string) map[string]bool {
	out := make(map[string]bool, len(roots)*2)
	for _, r := range roots {
		out[r] = true
		if resolved, err := filepath.EvalSymlinks(r); err == nil {
			out[filepath.Clean(resolved)] = true
		}
	}
	return out
}

// ResolveMountParent determines which directory to mount for a given path.
// If the path is a directory, it returns that directory.
// If the path is a file (or doesn't exist), it walks up to find the nearest
// existing parent directory.
// Symlinks in the input are resolved before the blocklist check, so a
// symlink pointing at a blocked system root is rejected.
// Returns an error if the resolved directory is a blocked broad system root
// or if the path is not absolute.
func ResolveMountParent(path string) (string, error) {
	path = filepath.Clean(path)

	// Only absolute paths are meaningful for bind mounts into the VM.
	// Rejecting relative/empty paths up front removes the ambiguity that
	// CodeQL flags as tainted path-expression input.
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("mount path must be absolute, got %q", path)
	}

	// Walk up until we find an existing directory (following symlinks).
	dir := path
	for {
		info, err := os.Stat(dir)
		if err == nil {
			if info.IsDir() {
				break
			}
			// It's a file — use its parent
			dir = filepath.Dir(dir)
			continue
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent
	}

	// Resolve symlinks so the blocklist check can't be bypassed by a link
	// pointing into a blocked root. EvalSymlinks requires the path to exist;
	// if it fails, fall back to the lexical path (which we already walked up
	// to an existing directory above).
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = filepath.Clean(resolved)
	}

	if blockedMountRoots[dir] {
		return "", fmt.Errorf("cannot auto-mount %q — too broad. Use a more specific path (e.g., a subdirectory of %s)", dir, dir)
	}

	return dir, nil
}

// TranslateVolumeSpec rewrites a volume spec's host path.
// Returns an error if the host path is absolute but not covered by any mount.
func TranslateVolumeSpec(spec string, mounts map[string]string) (string, error) {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) < 2 {
		return spec, nil
	}
	source := parts[0]
	if !strings.HasPrefix(source, "/") {
		return spec, nil // named volume, don't translate
	}
	translated, ok := TranslatePath(source, mounts)
	if !ok {
		return "", fmt.Errorf("bind mount path %q is not accessible in the shared VM (not covered by any workspace mount)", source)
	}
	return translated + ":" + parts[1], nil
}
