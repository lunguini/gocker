package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lunguini/gocker/engine"
)

func TestAttach_FallsBackToShWhenBashMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := SaveState(&SandboxState{
		Name:        "dev",
		Agent:       "claude",
		ContainerID: "dev-container",
		Status:      "running",
		Created:     time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	var gotArgs []string
	var gotInteractive bool
	mock := &engine.MockRuntime{
		ContainerExecFunc: func(ctx context.Context, nameOrID string, args []string, interactive bool) error {
			gotArgs = args
			gotInteractive = interactive
			return nil
		},
	}

	if err := NewManager(mock).Attach(context.Background(), "dev"); err != nil {
		t.Fatal(err)
	}
	if !gotInteractive {
		t.Error("attach must exec interactively")
	}
	joined := strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "bash") || !strings.Contains(joined, "sh") {
		t.Errorf("attach should prefer bash but fall back to sh, got args %v", gotArgs)
	}
	if gotArgs[0] != "/bin/sh" {
		t.Errorf("attach must launch via /bin/sh (always present) so bash-less images still work, got %v", gotArgs)
	}
}
