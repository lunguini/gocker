package config

import "testing"

func TestIsDevVersion(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{
		{"", true},
		{"dev", true},
		{"v0.7.7", false},
		{"v1.2.3", false},
		{"v0.7.7-14-g2656d4f", true},       // commits ahead of tag
		{"v0.7.7-dirty", true},             // dirty worktree
		{"v0.7.7-14-g2656d4f-dirty", true}, // both
		{"v0.7.7-rc.1", true},              // pre-release not a clean tag
		{"0.7.7", true},                    // missing leading v
		{"v0.7", true},                     // not three-part
	}
	for _, tc := range cases {
		if got := IsDevVersion(tc.v); got != tc.want {
			t.Errorf("IsDevVersion(%q) = %v, want %v", tc.v, got, tc.want)
		}
	}
}
