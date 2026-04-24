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

func TestIsQuitInput(t *testing.T) {
	quit := []string{
		"q", "Q", "quit", "QUIT", "exit", "Exit",
		"\x1b",       // bare Esc then Enter
		"\x1b\x1b",   // Meta-Esc variant some terminals send
		" q ",        // with whitespace
		"\t\x1b\n",   // extra whitespace + Esc (trim-space will remove the \n,
		              // actually readLine already strips the \n; this checks robustness)
	}
	for _, s := range quit {
		if !isQuitInput(s) {
			t.Errorf("isQuitInput(%q) = false, want true", s)
		}
	}
	notQuit := []string{
		"", "y", "n", "yes", "shared", "\x1b[A", "\x1b[B", "quitx", "xq",
	}
	for _, s := range notQuit {
		if isQuitInput(s) {
			t.Errorf("isQuitInput(%q) = true, want false", s)
		}
	}
}

func TestParseConfirm_QuitCallsQuitFn(t *testing.T) {
	// Swap quitFn for a test hook so we can verify it's invoked without
	// actually os.Exit'ing the test process.
	called := false
	orig := quitFn
	quitFn = func() { called = true }
	defer func() { quitFn = orig }()

	_ = parseConfirm(strings.NewReader("q\n"), false)
	if !called {
		t.Error("parseConfirm on 'q' input did not call quitFn")
	}
}

func TestParseChoice_QuitCallsQuitFn(t *testing.T) {
	called := false
	orig := quitFn
	quitFn = func() { called = true }
	defer func() { quitFn = orig }()

	_ = parseChoice(strings.NewReader("\x1b\n"), []string{"a", "b"}, "a")
	if !called {
		t.Error("parseChoice on Esc input did not call quitFn")
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
