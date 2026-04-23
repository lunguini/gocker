package setup

import (
	"fmt"
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

	cpu := min(max(hostCPUs/cpuFrac, 2), cpuCap)
	mem := min(max(hostMemGB/memFrac, 2), memCap)
	return cpu, fmt.Sprintf("%dG", mem)
}
