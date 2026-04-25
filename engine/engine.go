package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Engine is the Apple Container CLI backend (macOS).
type Engine struct {
	Binary string
}

func New(binary string) *Engine {
	if binary == "" {
		binary = "/usr/local/bin/container"
	}
	return &Engine{Binary: binary}
}

func (e *Engine) BinaryPath() string {
	return e.Binary
}

func (e *Engine) Validate() error {
	if _, err := os.Stat(e.Binary); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("apple container CLI not found at %s; run 'gocker setup' to install it", e.Binary)
		}
		return fmt.Errorf("cannot access container binary at %s: %w", e.Binary, err)
	}
	return nil
}

// EnsureSystemRunning checks whether the Apple Container system service is
// active and starts it automatically if it isn't. This prevents the confusing
// "XPC connection error: Connection invalid" message users see when the service
// has been stopped (e.g. after a reboot).
func (e *Engine) EnsureSystemRunning(ctx context.Context) error {
	// Probe with `container system status` — exit 0 means it's running.
	stdout, stderr, err := e.Exec(ctx, "system", "status")
	if err == nil {
		return nil
	}

	// Only auto-start if the failure looks like a stopped/disconnected service.
	// The status message may appear on stdout or stderr depending on the version.
	combined := string(stdout) + string(stderr)
	if !strings.Contains(combined, "not running") && !strings.Contains(combined, "XPC") && !strings.Contains(combined, "Connection invalid") {
		return nil // Different error — let it surface naturally later.
	}

	fmt.Fprintln(os.Stderr, "Container system service is not running. Starting it...")
	if startErr := e.ExecInteractive(ctx, "system", "start"); startErr != nil {
		return fmt.Errorf("failed to start container system service: %w (run 'container system start' manually)", startErr)
	}

	// Give the service a moment to become ready and verify.
	for range 10 {
		if _, _, probeErr := e.Exec(ctx, "system", "status"); probeErr == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("container system service started but is not responding — try 'container system start' manually")
}

func (e *Engine) Exec(ctx context.Context, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, e.Binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func (e *Engine) ExecInteractive(ctx context.Context, args ...string) error {
	// Save terminal state so we can restore it if the child process crashes
	oldState := saveTermState()
	if oldState != nil {
		defer restoreTermState(oldState)
	}

	cmd := exec.CommandContext(ctx, e.Binary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Let the container CLI inherit the foreground process group so it
	// can manage the TTY directly (raw mode, signal handling, etc.).
	// Using Setpgid would put it in a background group, causing SIGTTOU
	// freezes when the CLI calls tcsetpgrp() during process changes
	// inside the VM.
	return cmd.Run()
}

func (e *Engine) ExecStream(ctx context.Context, args ...string) (io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, e.Binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	return &streamReader{cmd: cmd, reader: stdout}, nil
}

func (e *Engine) ExecStreamSplit(ctx context.Context, args ...string) (io.ReadCloser, io.ReadCloser, error) {
	return execStreamSplit(ctx, e.Binary, args...)
}

// execStreamSplit starts a command and returns separate stdout/stderr pipes.
// The cmd.Wait runs in the background once both pipes hit EOF.
func execStreamSplit(ctx context.Context, binary string, args ...string) (io.ReadCloser, io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start: %w", err)
	}
	shared := &sharedCmd{cmd: cmd, remaining: 2}
	return &splitReader{shared: shared, reader: stdout}, &splitReader{shared: shared, reader: stderr}, nil
}

type sharedCmd struct {
	cmd       *exec.Cmd
	mu        sync.Mutex
	remaining int
}

type splitReader struct {
	shared *sharedCmd
	reader io.ReadCloser
}

func (s *splitReader) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *splitReader) Close() error {
	_ = s.reader.Close()
	s.shared.mu.Lock()
	s.shared.remaining--
	last := s.shared.remaining == 0
	s.shared.mu.Unlock()
	if last {
		return s.shared.cmd.Wait()
	}
	return nil
}

type streamReader struct {
	cmd    *exec.Cmd
	reader io.ReadCloser
}

func (s *streamReader) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *streamReader) Close() error {
	_ = s.reader.Close()
	return s.cmd.Wait()
}
