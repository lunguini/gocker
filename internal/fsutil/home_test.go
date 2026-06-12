package fsutil

import "testing"

func TestHomeDir_ReturnsHome(t *testing.T) {
	t.Setenv("HOME", "/tmp/fakehome")
	if got := HomeDir(); got != "/tmp/fakehome" {
		t.Errorf("expected /tmp/fakehome, got %q", got)
	}
}

func TestHomeDir_ExitsWhenUnresolvable(t *testing.T) {
	t.Setenv("HOME", "")

	exited := false
	origExit := osExit
	osExit = func(code int) { exited = true }
	defer func() { osExit = origExit }()

	got := HomeDir()
	if !exited {
		t.Errorf("expected exit when home dir is unresolvable, got %q", got)
	}
}
