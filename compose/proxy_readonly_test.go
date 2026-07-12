package compose

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lunguini/gocker/config"
	"github.com/lunguini/gocker/engine"
	"github.com/lunguini/gocker/sharedvm"
)

// vmlessRuntime is an engine.Runtime for a machine where the compose VM does
// not exist. It records VM-creation attempts and refuses them; any method a
// test doesn't expect panics via the embedded nil interface.
type vmlessRuntime struct {
	engine.Runtime
	createAttempts int
}

func (s *vmlessRuntime) Exec(ctx context.Context, args ...string) ([]byte, []byte, error) {
	// Covers both the manager's liveness probe (`exec <vm> true`) and the
	// capAddArgs `run --help` probe — neither finds a usable CLI/VM here.
	return nil, nil, errors.New("no such container")
}

func (s *vmlessRuntime) ContainerInspect(ctx context.Context, name string) ([]byte, error) {
	return nil, errors.New("no such container: " + name)
}

func (s *vmlessRuntime) ContainerRemove(ctx context.Context, name string, force bool) error {
	return nil
}

func (s *vmlessRuntime) ContainerRun(ctx context.Context, args []string, interactive bool) error {
	s.createAttempts++
	return errors.New("stub refuses to create VMs")
}

func newVMlessProxy(t *testing.T) (*Proxy, *vmlessRuntime) {
	t.Helper()
	// Isolate ~/.gocker (VM state files, lifecycle locks) from the real host.
	t.Setenv("HOME", t.TempDir())
	rt := &vmlessRuntime{}
	mgr := sharedvm.NewManagerWithName(rt, config.SharedVM{}, "gocker-compose-testproj")
	return NewProxy(rt, mgr), rt
}

// TestComposeReadOnlyDoesNotCreateVM pins the invariant that query-only
// compose subcommands never create a VM: with no VM to ask, the answer is
// empty output and exit 0. Regression test for `compose ps` after `down`
// in full isolation booting — and leaking — a fresh per-project VM.
func TestComposeReadOnlyDoesNotCreateVM(t *testing.T) {
	p, rt := newVMlessProxy(t)

	for _, sub := range []string{"ps", "logs", "images", "top", "port", "events", "ls", "version"} {
		t.Run(sub, func(t *testing.T) {
			err := p.Exec(context.Background(), []string{"-f", "/tmp/x/docker-compose.yml", sub}, false)
			if err != nil {
				t.Fatalf("read-only %q with no VM should be a silent no-op, got error: %v", sub, err)
			}
		})
	}

	if rt.createAttempts != 0 {
		t.Fatalf("read-only compose commands attempted VM creation %d times", rt.createAttempts)
	}
}

// TestComposeMutatingCreatesVM is the counterpart: mutating subcommands must
// still go through EnsureRunning and attempt to create the missing VM.
func TestComposeMutatingCreatesVM(t *testing.T) {
	p, rt := newVMlessProxy(t)

	err := p.Exec(context.Background(), []string{"-f", "/tmp/x/docker-compose.yml", "up", "-d"}, false)
	if err == nil || !strings.Contains(err.Error(), "creating shared VM") {
		t.Fatalf("expected VM-creation failure to propagate, got: %v", err)
	}
	if rt.createAttempts == 0 {
		t.Fatal("mutating compose command never attempted VM creation")
	}
}

func TestIsReadOnly(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"plain ps", []string{"ps"}, true},
		{"ps with file flag", []string{"-f", "x.yml", "ps"}, true},
		{"logs with project flag", []string{"-p", "proj", "logs"}, true},
		{"up is mutating", []string{"-f", "x.yml", "up", "-d"}, false},
		{"down is mutating", []string{"down"}, false},
		{"config still proxies", []string{"config"}, false},
		{"service named ps is not the subcommand", []string{"-f", "x.yml", "logs", "ps"}, true},
		{"exec of service named ps is mutating", []string{"exec", "ps", "sh"}, false},
		{"flag=value does not consume next token", []string{"--ansi=never", "ps"}, true},
		{"no subcommand", []string{"-f", "x.yml"}, false},
		{"empty", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isReadOnly(c.args); got != c.want {
				t.Errorf("isReadOnly(%v) = %v, want %v", c.args, got, c.want)
			}
		})
	}
}
