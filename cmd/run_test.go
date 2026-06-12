package cmd

import (
	"os"
	"path/filepath"
	"testing"
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
