package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
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

func (e *Engine) Exec(ctx context.Context, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, e.Binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func (e *Engine) ExecInteractive(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, e.Binary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
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
