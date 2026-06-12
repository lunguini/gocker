package engine

import (
	"fmt"
	"os/exec"
	"runtime"
)

// DetectRuntime auto-detects the appropriate container runtime for the
// current platform. On macOS it uses Apple's container CLI, on Linux
// it uses nerdctl (containerd).
func DetectRuntime(binaryOverride string) (Runtime, error) {
	switch runtime.GOOS {
	case "darwin":
		return New(resolveContainerBinary(binaryOverride)), nil

	case "linux":
		binary := binaryOverride
		if binary == "" {
			binary = "nerdctl"
			if _, err := exec.LookPath(binary); err != nil {
				return nil, fmt.Errorf("nerdctl not found — install containerd and nerdctl to use gocker on Linux")
			}
		}
		return NewNerdctl(binary), nil

	default:
		return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// resolveContainerBinary picks the Apple container CLI binary path.
// Priority: explicit override (config runtimeBinary / flag) > $PATH >
// /usr/local/bin/container. The hardcoded fallback stays last because
// GUI-launched processes often run with a minimal PATH.
func resolveContainerBinary(override string) string {
	if override != "" {
		return override
	}
	if path, err := exec.LookPath("container"); err == nil {
		return path
	}
	return "/usr/local/bin/container"
}
