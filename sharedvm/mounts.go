package sharedvm

import (
	"fmt"
	"path/filepath"
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
func MountFlags(mounts map[string]string) []string {
	var flags []string
	for host, vm := range mounts {
		flags = append(flags, "-v", host+":"+vm)
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
