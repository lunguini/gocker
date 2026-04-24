package setup

import (
	"bufio"
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
		// Terminal-in-raw-mode cases: Enter sends CR, not LF. Previously
		// ReadString('\n') hung forever on these — that's the bug that
		// made users unable to Enter through `gocker setup` prompts.
		{"y\r", false, true},
		{"y\r\n", false, true}, // CRLF, we should swallow both
		{"\r", true, true},
		{"n\r", true, false},
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
		// Raw-mode Enter → CR only.
		{"2\r", "full", "hybrid"},
		{"\r", "shared", "shared"},
		{"shared\r\n", "full", "shared"},
	}
	for _, tc := range cases {
		got := parseChoice(strings.NewReader(tc.input), opts, tc.def)
		if got != tc.want {
			t.Errorf("parseChoice(%q, def=%q) = %q, want %q", tc.input, tc.def, got, tc.want)
		}
	}
}

func TestReadLine_CRLF(t *testing.T) {
	// Two lines separated by CRLF — make sure the CR and LF are consumed
	// as a single line terminator (not as an empty second line). Must
	// share a single *bufio.Reader across calls, same as production's
	// package-level stdinReader.
	br := bufio.NewReader(strings.NewReader("first\r\nsecond\n"))
	if got := readLine(br); got != "first" {
		t.Errorf("first line: got %q, want first", got)
	}
	if got := readLine(br); got != "second" {
		t.Errorf("second line: got %q, want second", got)
	}
}
