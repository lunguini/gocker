package cmd

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

// stubRuntime is a minimal engine.Runtime test double, kept local to this
// package (rather than reusing engine.MockRuntime) so this test doesn't churn
// alongside interface additions happening elsewhere in the same session.
// Only ContainerList is implemented; every other method panics via the nil
// embedded Runtime if a test path accidentally calls it.
type stubRuntime struct {
	engine.Runtime
	id string
}

func (s *stubRuntime) ContainerList(ctx context.Context, all bool) ([]engine.ContainerInfo, error) {
	return []engine.ContainerInfo{{ID: s.id}}, nil
}

func TestRuntimeSwitch_StoreLoadSwapsInner(t *testing.T) {
	a := &stubRuntime{id: "aaaaaaaaaaaa"}
	b := &stubRuntime{id: "bbbbbbbbbbbb"}

	sw := newRuntimeSwitch(a)
	if sw.Load() != engine.Runtime(a) {
		t.Fatalf("expected Load() to return a before Store")
	}

	sw.Store(b)
	if sw.Load() != engine.Runtime(b) {
		t.Fatalf("expected Load() to return b after Store")
	}

	// Calls through the switch reach whichever runtime is currently stored.
	containers, err := sw.ContainerList(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 1 || containers[0].ID != b.id {
		t.Fatalf("expected ContainerList to route to b, got %+v", containers)
	}
}

// TestRuntimeSwitch_IsolationFlagRoutesCommand exercises the mechanism
// root.go's Before hook relies on: a global flag parsed before the command
// runs swaps the runtimeSwitch's inner runtime, and a command constructed
// with that switch (here, `ps`) transparently picks up the new runtime on
// its next invocation — without the command constructor being rebuilt.
func TestRuntimeSwitch_IsolationFlagRoutesCommand(t *testing.T) {
	defaultRT := &stubRuntime{id: "default000000"}
	overrideRT := &stubRuntime{id: "override000000"}

	sw := newRuntimeSwitch(engine.Runtime(defaultRT))

	app := &cli.Command{
		Name: "gocker-test",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "isolation"},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			if cmd.IsSet("isolation") {
				sw.Store(engine.Runtime(overrideRT))
			}
			return ctx, nil
		},
		Commands: []*cli.Command{newPsCmd(sw)},
	}

	runAndCapture := func(args []string) string {
		r, w, _ := os.Pipe()
		orig := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = orig }()

		if err := app.Run(context.Background(), args); err != nil {
			t.Fatalf("app.Run(%v) error: %v", args, err)
		}
		_ = w.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		return buf.String()
	}

	out := runAndCapture([]string{"gocker-test", "ps", "-q"})
	if !bytes.Contains([]byte(out), []byte(defaultRT.id[:12])) {
		t.Errorf("expected default runtime's container ID in output, got %q", out)
	}

	out = runAndCapture([]string{"gocker-test", "--isolation", "shared", "ps", "-q"})
	if !bytes.Contains([]byte(out), []byte(overrideRT.id[:12])) {
		t.Errorf("expected override runtime's container ID in output after --isolation, got %q", out)
	}
}
