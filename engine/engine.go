package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

type Engine struct {
	Binary string
}

func New(binary string) *Engine {
	if binary == "" {
		binary = "/usr/local/bin/container"
	}
	return &Engine{Binary: binary}
}

func (e *Engine) Validate() error {
	if _, err := os.Stat(e.Binary); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("Apple Container CLI not found at %s. Run 'gocker setup' to install it.", e.Binary)
		}
		return fmt.Errorf("cannot access container binary at %s: %w", e.Binary, err)
	}
	return nil
}

func (e *Engine) Exec(ctx context.Context, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, e.Binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// termState holds a saved copy of the terminal settings.
type termState struct {
	termios syscall.Termios
}

func getTermState(fd int) (*termState, error) {
	var t syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCGETA), uintptr(unsafe.Pointer(&t)), 0, 0, 0)
	if errno != 0 {
		return nil, errno
	}
	return &termState{termios: t}, nil
}

func restoreTermState(fd int, state *termState) {
	syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCSETA), uintptr(unsafe.Pointer(&state.termios)), 0, 0, 0)
}

func (e *Engine) ExecInteractive(ctx context.Context, args ...string) error {
	// Save terminal state so we can restore it if the child process crashes
	fd := int(os.Stdin.Fd())
	oldState, err := getTermState(fd)
	if err == nil {
		defer restoreTermState(fd, oldState)
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

type streamReader struct {
	cmd    *exec.Cmd
	reader io.ReadCloser
}

func (s *streamReader) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *streamReader) Close() error {
	s.reader.Close()
	return s.cmd.Wait()
}
