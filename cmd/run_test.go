package cmd

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/lunguini/gocker/engine"
)

func TestReadEnvFile_MissingFileReturnsError(t *testing.T) {
	_, err := readEnvFile(filepath.Join(t.TempDir(), "nope.env"))
	if err == nil {
		t.Fatal("expected error for missing env file, got nil")
	}
}

func TestReadEnvFile_ParsesLinesSkippingCommentsAndBlanks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vars.env")
	content := "FOO=bar\n\n# a comment\nBAZ=qux\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	envs, err := readEnvFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"FOO=bar", "BAZ=qux"}
	if len(envs) != len(want) || envs[0] != want[0] || envs[1] != want[1] {
		t.Errorf("expected %v, got %v", want, envs)
	}
}

func TestReadEnvFile_BareKeyInheritsFromHost(t *testing.T) {
	t.Setenv("GOCKER_TEST_INHERITED", "from-host")
	path := filepath.Join(t.TempDir(), "vars.env")
	// Docker semantics: a bare KEY inherits the host value; an unset bare
	// KEY is silently dropped.
	content := "GOCKER_TEST_INHERITED\nGOCKER_TEST_DEFINITELY_UNSET\nFOO=bar\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	envs, err := readEnvFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Contains(envs, "GOCKER_TEST_INHERITED=from-host") {
		t.Errorf("bare KEY should inherit host value, got %v", envs)
	}
	for _, e := range envs {
		if e == "GOCKER_TEST_DEFINITELY_UNSET" || e == "GOCKER_TEST_DEFINITELY_UNSET=" {
			t.Errorf("unset bare KEY must be dropped, got %v", envs)
		}
	}
	if !slices.Contains(envs, "FOO=bar") {
		t.Errorf("normal KEY=VALUE lines must still work, got %v", envs)
	}
}

func TestRun_EnvFileFlagIsRepeatable(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.env")
	fileB := filepath.Join(dir, "b.env")
	if err := os.WriteFile(fileA, []byte("A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileB, []byte("B=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var gotArgs []string
	mock := &engine.MockRuntime{
		ContainerRunFunc: func(ctx context.Context, args []string, interactive bool) error {
			gotArgs = args
			return nil
		},
	}
	cmd := newRunCmd(mock)
	err := cmd.Run(context.Background(), []string{"run", "--env-file", fileA, "--env-file", fileB, "alpine"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(gotArgs, "A=1") || !slices.Contains(gotArgs, "B=2") {
		t.Errorf("both env files should contribute vars, got %v", gotArgs)
	}
}
