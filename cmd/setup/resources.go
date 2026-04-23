package setup

import (
	"fmt"
	"runtime"

	"golang.org/x/sys/unix"
)

// defaultResources returns (cpu, memory) defaults for the given isolation mode,
// scaled to the host. Shared/hybrid give the VM more headroom than full.
func defaultResources(isolation string, hostCPUs, hostMemGB int) (int, string) {
	cpuCap, memCap := 8, 16
	cpuFrac, memFrac := 2, 2

	switch isolation {
	case "shared", "hybrid":
		// more generous — this VM runs everything
	default: // full
		cpuCap, memCap = 4, 8
		cpuFrac, memFrac = 4, 4
	}

	cpu := hostCPUs / cpuFrac
	if cpu < 2 {
		cpu = 2
	}
	if cpu > cpuCap {
		cpu = cpuCap
	}

	mem := hostMemGB / memFrac
	if mem < 2 {
		mem = 2
	}
	if mem > memCap {
		mem = memCap
	}
	return cpu, fmt.Sprintf("%dG", mem)
}

// detectHostResources returns (cpus, memoryGiB) from the host OS.
func detectHostResources() (int, int) {
	cpus := runtime.NumCPU()
	mem := 8 // fallback
	if v, err := unix.SysctlUint64("hw.memsize"); err == nil {
		mem = int(v / (1024 * 1024 * 1024))
	}
	return cpus, mem
}
