package sharedvm

import (
	"strings"

	"github.com/lunguini/gocker/engine"
)

// wrapExecErr turns a proxied command's stderr + exec error into an error that
// (a) never renders as a bare ": exit status 1" when stderr is empty, and
// (b) carries the same engine sentinels (ErrNotFound, ErrAlreadyExists,
// ErrUnauthorized) so errors.Is keeps working after commands are proxied into
// the shared VM. It mirrors engine/errors.go's cliError; the marker lists are
// replicated here because engine.classifySentinel is unexported.
func wrapExecErr(stderr []byte, err error) error {
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(stderr))
	if msg == "" {
		msg = "command failed"
	}
	return &execErr{msg: msg, cause: err, sentinel: classifyStderr(msg)}
}

// execErr mirrors engine.cliErr: it prints the CLI stderr wrapping the exec
// error and unwraps to both the cause and any matched engine sentinel.
type execErr struct {
	msg      string
	cause    error
	sentinel error
}

func (e *execErr) Error() string { return e.msg + ": " + e.cause.Error() }

func (e *execErr) Unwrap() []error {
	if e.sentinel != nil {
		return []error{e.cause, e.sentinel}
	}
	return []error{e.cause}
}

// classifyStderr maps known Apple/nerdctl error phrasings to engine sentinels.
// Keep the marker lists in sync with engine/errors.go's classifySentinel.
func classifyStderr(msg string) error {
	m := strings.ToLower(msg)
	for _, marker := range []string{"not found", "no such", "does not exist", "unknown image", "unknown container"} {
		if strings.Contains(m, marker) {
			return engine.ErrNotFound
		}
	}
	for _, marker := range []string{"already exists", "already in use"} {
		if strings.Contains(m, marker) {
			return engine.ErrAlreadyExists
		}
	}
	for _, marker := range []string{"401", "unauthorized", "authentication required", "access denied"} {
		if strings.Contains(m, marker) {
			return engine.ErrUnauthorized
		}
	}
	return nil
}
