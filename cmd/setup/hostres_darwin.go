//go:build darwin

package setup

import (
	"runtime"

	"golang.org/x/sys/unix"
)

// detectHostResources returns (cpus, memoryGiB) from the host OS. On macOS
// we read hw.memsize via sysctl; if the call fails we fall back to 8 GiB.
func detectHostResources() (int, int) {
	cpus := runtime.NumCPU()
	mem := 8
	if v, err := unix.SysctlUint64("hw.memsize"); err == nil {
		mem = int(v / (1024 * 1024 * 1024))
	}
	return cpus, mem
}
