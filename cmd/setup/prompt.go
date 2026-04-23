package setup

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// IsInteractive returns true if stdin is a terminal.
func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// Confirm prompts for y/n and returns the answer. Uses def if input is empty or unparseable.
func Confirm(prompt string, def bool) bool {
	suffix := " [y/N]: "
	if def {
		suffix = " [Y/n]: "
	}
	fmt.Print(prompt + suffix)
	return parseConfirm(os.Stdin, def)
}

func parseConfirm(r io.Reader, def bool) bool {
	line, _ := bufio.NewReader(r).ReadString('\n')
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
	return parseChoice(os.Stdin, options, def)
}

func parseChoice(r io.Reader, options []string, def string) string {
	line, _ := bufio.NewReader(r).ReadString('\n')
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
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	s := strings.TrimSpace(line)
	if s == "" {
		return def
	}
	return s
}
