package setup

import (
	"strings"
	"testing"
)

func TestParseConfirm(t *testing.T) {
	cases := []struct {
		input string
		def   bool
		want  bool
	}{
		{"y\n", false, true},
		{"Y\n", false, true},
		{"yes\n", false, true},
		{"n\n", true, false},
		{"no\n", true, false},
		{"\n", true, true},
		{"\n", false, false},
		{"garbage\n", true, true},
	}
	for _, tc := range cases {
		got := parseConfirm(strings.NewReader(tc.input), tc.def)
		if got != tc.want {
			t.Errorf("parseConfirm(%q, def=%v) = %v, want %v", tc.input, tc.def, got, tc.want)
		}
	}
}

func TestParseChoice(t *testing.T) {
	opts := []string{"full", "hybrid", "shared"}
	cases := []struct {
		input string
		def   string
		want  string
	}{
		{"1\n", "full", "full"},
		{"2\n", "full", "hybrid"},
		{"3\n", "full", "shared"},
		{"\n", "shared", "shared"},
		{"shared\n", "full", "shared"},
		{"bogus\n", "full", "full"},
	}
	for _, tc := range cases {
		got := parseChoice(strings.NewReader(tc.input), opts, tc.def)
		if got != tc.want {
			t.Errorf("parseChoice(%q, def=%q) = %q, want %q", tc.input, tc.def, got, tc.want)
		}
	}
}
