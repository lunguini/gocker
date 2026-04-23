//go:build !darwin

package setup

import "runtime"

// detectHostResources returns (cpus, memoryGiB) from the host OS. The setup
// wizard primarily targets macOS, where we read actual host memory via an
// hw.memsize sysctl (see hostres_darwin.go). On other platforms we return a
// conservative default for memory so the wizard still produces sensible
// VM defaults; users can override interactively or edit ~/.gocker/config.yaml.
func detectHostResources() (int, int) {
	return runtime.NumCPU(), 8
}
