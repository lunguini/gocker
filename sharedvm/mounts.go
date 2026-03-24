package sharedvm

import (
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
// Returns the original path if no mount covers it.
func TranslatePath(hostPath string, mounts map[string]string) string {
	hostPath = filepath.Clean(hostPath)
	for host, vm := range mounts {
		if hostPath == host || strings.HasPrefix(hostPath, host+"/") {
			return vm + hostPath[len(host):]
		}
	}
	return hostPath
}

// TranslateVolumeSpec rewrites a volume spec's host path.
// e.g., "/Users/me/app/src:/app/src" → "/host/Users/me/app/src:/app/src"
// Bind mounts (starting with / or .) are translated; named volumes are left alone.
func TranslateVolumeSpec(spec string, mounts map[string]string) string {
	parts := strings.SplitN(spec, ":", 2)
	if len(parts) < 2 {
		return spec
	}
	source := parts[0]
	if !strings.HasPrefix(source, "/") {
		return spec // named volume, don't translate
	}
	translated := TranslatePath(source, mounts)
	return translated + ":" + parts[1]
}
