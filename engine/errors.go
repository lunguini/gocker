package engine

import (
	"errors"
	"strings"
)

// Sentinel errors classified from container CLI output. Backends are pure
// shell-outs, so error text is the only signal — classification happens
// once here, where the stderr is captured, instead of being re-matched by
// every caller. Check with errors.Is.
var (
	ErrNotFound      = errors.New("resource not found")
	ErrAlreadyExists = errors.New("resource already exists")
	ErrUnauthorized  = errors.New("registry authentication required")
)

// cliErr carries the CLI's stderr text, the original exec error, and the
// sentinel its text classified to (if any).
type cliErr struct {
	msg      string
	cause    error
	sentinel error
}

func (e *cliErr) Error() string { return e.msg + ": " + e.cause.Error() }

func (e *cliErr) Unwrap() []error {
	if e.sentinel != nil {
		return []error{e.cause, e.sentinel}
	}
	return []error{e.cause}
}

// classifySentinel maps known CLI error phrasings (Apple container CLI and
// nerdctl) to sentinels. Add new phrasings here, not at call sites.
func classifySentinel(msg string) error {
	m := strings.ToLower(msg)
	// "no network found matching" is nerdctl's network-inspect phrasing.
	for _, marker := range []string{"not found", "no such", "does not exist", "unknown image", "unknown container", "no network found"} {
		if strings.Contains(m, marker) {
			return ErrNotFound
		}
	}
	for _, marker := range []string{"already exists", "already in use"} {
		if strings.Contains(m, marker) {
			return ErrAlreadyExists
		}
	}
	for _, marker := range []string{"401", "unauthorized", "authentication required", "access denied"} {
		if strings.Contains(m, marker) {
			return ErrUnauthorized
		}
	}
	return nil
}

// cliError builds the standard engine error: trimmed stderr wrapping the
// exec error, tagged with a sentinel when the text matches a known class.
func cliError(stderr []byte, err error) error {
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		msg = "command failed"
	}
	return &cliErr{msg: msg, cause: err, sentinel: classifySentinel(msg)}
}
