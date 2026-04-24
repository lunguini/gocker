package setup

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// stdinReader is a package-level shared reader so chained prompts don't lose
// buffered bytes to per-call bufio.Readers (important for scripted stdin).
var stdinReader = bufio.NewReader(os.Stdin)

// IsInteractive returns true if stdin is a terminal.
func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// NormalizeTerminal forces the terminal into the standard line-editing
// (canonical + echo) mode before prompting. Earlier steps in `gocker setup`
// invoke the Apple `container` CLI via ExecInteractive; if it exits without
// restoring termios cleanly, the terminal stays in raw mode and every
// subsequent prompt hangs forever because Enter produces CR (`\r`), not
// LF (`\n`) — and our line-readers wait for LF.
//
// `stty sane` is the cross-shell universal "fix my terminal" command. No-op
// on non-interactive stdin and silent if stty isn't available.
func NormalizeTerminal() {
	if !IsInteractive() {
		return
	}
	cmd := exec.CommandContext(context.Background(), "stty", "sane")
	cmd.Stdin = os.Stdin
	_ = cmd.Run()
}

// quitFn is called when the user requests to abort the wizard. Overridable
// for tests so the process doesn't actually die mid-test.
var quitFn = func() {
	fmt.Fprintln(os.Stderr, "\nSetup cancelled.")
	os.Exit(0)
}

// isQuitInput detects "I want to bail on this wizard" from a line of input.
// Matches: "q", "quit", "exit" (case-insensitive), or a bare Esc keypress
// (represented as one or more ESC bytes before Enter — terminals send
// '\x1b' when Esc is pressed alone, and may send '\x1b\x1b' on some Meta
// bindings). Arrow keys send '\x1b[A' etc., which this check correctly
// ignores because the trimmed string has trailing non-ESC bytes.
func isQuitInput(s string) bool {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "q", "quit", "exit":
		return true
	}
	if s != "" && strings.TrimLeft(s, "\x1b") == "" {
		return true
	}
	return false
}

// readLine reads from r until it sees '\n' or '\r', returns the line with
// the terminator stripped. Works in both canonical-mode terminals (where
// Enter sends '\n') and raw-mode terminals (where Enter sends '\r'). If
// the terminator is '\r' immediately followed by '\n' (CRLF on Windows or
// some telnet paths), consumes both so the next call doesn't see a stray
// empty line.
func readLine(r io.Reader) string {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	var sb strings.Builder
	for {
		b, err := br.ReadByte()
		if err != nil {
			return sb.String()
		}
		if b == '\n' {
			return sb.String()
		}
		if b == '\r' {
			// Swallow a trailing '\n' if present (CRLF pair).
			if next, perr := br.Peek(1); perr == nil && len(next) == 1 && next[0] == '\n' {
				_, _ = br.ReadByte()
			}
			return sb.String()
		}
		sb.WriteByte(b)
	}
}

// Confirm prompts for y/n and returns the answer. Uses def if input is empty or unparseable.
func Confirm(prompt string, def bool) bool {
	suffix := " [y/N]: "
	if def {
		suffix = " [Y/n]: "
	}
	fmt.Print(prompt + suffix)
	return parseConfirm(stdinReader, def)
}

func parseConfirm(r io.Reader, def bool) bool {
	line := readLine(r)
	if isQuitInput(line) {
		quitFn()
		return def
	}
	s := strings.ToLower(strings.TrimSpace(line))
	switch s {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return def
	}
}

// Choose presents a numbered list and returns the selected option. Accepts
// either the 1-based index or the option string itself. Falls back to def on
// empty or unparseable input.
func Choose(prompt string, options []string, def string) string {
	fmt.Println(prompt)
	for i, o := range options {
		marker := "  "
		if o == def {
			marker = "* "
		}
		fmt.Printf("%s%d) %s\n", marker, i+1, o)
	}
	fmt.Printf("Select [default: %s]: ", def)
	return parseChoice(stdinReader, options, def)
}

func parseChoice(r io.Reader, options []string, def string) string {
	line := readLine(r)
	if isQuitInput(line) {
		quitFn()
		return def
	}
	s := strings.TrimSpace(line)
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= len(options) {
		return options[n-1]
	}
	for _, o := range options {
		if strings.EqualFold(o, s) {
			return o
		}
	}
	return def
}

// Input prompts for a free-text string, returning def if empty.
func Input(prompt, def string) string {
	fmt.Printf("%s [default: %s]: ", prompt, def)
	line := readLine(stdinReader)
	if isQuitInput(line) {
		quitFn()
		return def
	}
	s := strings.TrimSpace(line)
	if s == "" {
		return def
	}
	return s
}
