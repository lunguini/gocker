package setup

import "testing"

func TestDefaultResourcesForIsolation(t *testing.T) {
	cases := []struct {
		name      string
		isolation string
		hostCPUs  int
		hostMemGB int
		wantCPU   int
		wantMem   string
	}{
		{"shared gets more", "shared", 8, 16, 4, "8G"},
		{"hybrid same as shared", "hybrid", 8, 16, 4, "8G"},
		{"full gets modest defaults", "full", 8, 16, 2, "4G"},
		{"caps CPU at 8", "shared", 32, 64, 8, "16G"},
		{"caps memory at 16G", "shared", 8, 128, 4, "16G"},
		{"tiny host still usable", "full", 2, 4, 2, "2G"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cpu, mem := defaultResources(tc.isolation, tc.hostCPUs, tc.hostMemGB)
			if cpu != tc.wantCPU {
				t.Errorf("CPU: got %d, want %d", cpu, tc.wantCPU)
			}
			if mem != tc.wantMem {
				t.Errorf("Mem: got %q, want %q", mem, tc.wantMem)
			}
		})
	}
}
