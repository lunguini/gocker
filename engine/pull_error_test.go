package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// The interactive pull path must still classify errors: stderr is teed to
// the terminal AND captured so a 401 comes back as ErrUnauthorized instead
// of a bare "exit status 1".
func TestExecInteractiveTee_ClassifiesCapturedStderr(t *testing.T) {
	binary := makeFakeBinary(t, `
echo "Error: HTTP request failed with response: 401 Unauthorized" >&2
exit 1
`)
	eng := New(binary)
	err := eng.execInteractiveTee(context.Background(), "image", "pull", "alpine")
	if err == nil {
		t.Fatal("expected error from failing pull")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized classification, got: %v", err)
	}
	if !strings.Contains(err.Error(), "401 Unauthorized") {
		t.Errorf("error should carry the CLI message, got: %v", err)
	}
}

func TestExecInteractiveTee_SuccessReturnsNil(t *testing.T) {
	binary := makeFakeBinary(t, `exit 0`)
	eng := New(binary)
	if err := eng.execInteractiveTee(context.Background(), "image", "pull", "alpine"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}
