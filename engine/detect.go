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
	path, _ := ResolveBinaryInfo(override)
	return path
}

// ResolveBinaryInfo reports the resolved Apple container CLI path along with
// how it was found — "config" (explicit override), "PATH" (found on $PATH),
// or "fallback" (the hardcoded /usr/local/bin/container). It powers the
// `gocker doctor` diagnostics so users can see why a given binary was chosen.
func ResolveBinaryInfo(override string) (path, source string) {
	if override != "" {
		return override, "config"
	}
	if p, err := exec.LookPath("container"); err == nil {
		return p, "PATH"
	}
	return "/usr/local/bin/container", "fallback"
}
