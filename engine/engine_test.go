package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_BinaryExists(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "container")
	if err := os.WriteFile(tmp, []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}
	eng := New(tmp)
	if err := eng.Validate(); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidate_BinaryMissing(t *testing.T) {
	eng := New("/nonexistent/path/container")
	err := eng.Validate()
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
	expected := "Apple Container CLI not found at /nonexistent/path/container. Run 'gocker setup' to install it."
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}
