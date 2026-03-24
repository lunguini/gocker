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
		binary := binaryOverride
		if binary == "" {
			binary = "/usr/local/bin/container"
		}
		return New(binary), nil

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
