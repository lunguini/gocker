package engine

import (
	"errors"
	"fmt"
	"testing"
)

func TestCliError_ClassifiesSentinels(t *testing.T) {
	base := fmt.Errorf("exit status 1")
	cases := []struct {
		stderr   string
		sentinel error
	}{
		{`Error: failed to delete one or more containers: ["web"]: not found`, ErrNotFound},
		{"No such container: web", ErrNotFound},
		{"image does not exist", ErrNotFound},
		{`time="2026-07-07T09:07:40Z" level=error msg="no network found matching: web"`, ErrNotFound},
		{`Error: container with name "web" already exists`, ErrAlreadyExists},
		{"network gocker-net already exists", ErrAlreadyExists},
		{"401 Unauthorized. Reason: access denied or wrong credentials", ErrUnauthorized},
		{"authentication required", ErrUnauthorized},
	}
	for _, c := range cases {
		err := cliError([]byte(c.stderr), base)
		if !errors.Is(err, c.sentinel) {
			t.Errorf("cliError(%q) should match sentinel %v", c.stderr, c.sentinel)
		}
		if !errors.Is(err, base) {
			t.Errorf("cliError(%q) must still wrap the original error", c.stderr)
		}
	}
}

func TestCliError_UnclassifiedHasNoSentinel(t *testing.T) {
	err := cliError([]byte("XPC connection error: Connection invalid"), fmt.Errorf("exit status 1"))
	for _, sentinel := range []error{ErrNotFound, ErrAlreadyExists, ErrUnauthorized} {
		if errors.Is(err, sentinel) {
			t.Errorf("unclassified error must not match %v", sentinel)
		}
	}
}

func TestCliError_MessagePreservesStderrAndCause(t *testing.T) {
	err := cliError([]byte("  boom  \n"), fmt.Errorf("exit status 7"))
	want := "boom: exit status 7"
	if err.Error() != want {
		t.Errorf("expected %q, got %q", want, err.Error())
	}
}

func TestCliError_EmptyStderrStillInformative(t *testing.T) {
	err := cliError(nil, fmt.Errorf("exit status 1"))
	if err.Error() != "command failed: exit status 1" {
		t.Errorf("unexpected message for empty stderr: %q", err.Error())
	}
}
