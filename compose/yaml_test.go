package compose

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCommandOrArgs_String(t *testing.T) {
	input := `command: "echo hello world"`
	var s struct {
		Command CommandOrArgs `yaml:"command"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	expected := []string{"echo", "hello", "world"}
	if len(s.Command) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(s.Command), s.Command)
	}
	for i, v := range expected {
		if s.Command[i] != v {
			t.Errorf("command[%d] = %q, want %q", i, s.Command[i], v)
		}
	}
}

func TestCommandOrArgs_List(t *testing.T) {
	input := "command: [\"echo\", \"hello\"]"
	var s struct {
		Command CommandOrArgs `yaml:"command"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	expected := []string{"echo", "hello"}
	if len(s.Command) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(s.Command), s.Command)
	}
	for i, v := range expected {
		if s.Command[i] != v {
			t.Errorf("command[%d] = %q, want %q", i, s.Command[i], v)
		}
	}
}

func TestEnvironment_Map(t *testing.T) {
	input := "environment:\n  FOO: bar\n  NUM: \"42\""
	var s struct {
		Environment Environment `yaml:"environment"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if s.Environment["FOO"] != "bar" {
		t.Errorf("FOO = %q, want %q", s.Environment["FOO"], "bar")
	}
	if s.Environment["NUM"] != "42" {
		t.Errorf("NUM = %q, want %q", s.Environment["NUM"], "42")
	}
}

func TestEnvironment_List(t *testing.T) {
	input := "environment:\n  - FOO=bar\n  - EMPTY"
	var s struct {
		Environment Environment `yaml:"environment"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if s.Environment["FOO"] != "bar" {
		t.Errorf("FOO = %q, want %q", s.Environment["FOO"], "bar")
	}
	if v, ok := s.Environment["EMPTY"]; !ok {
		t.Error("EMPTY key missing")
	} else if v != "" {
		t.Errorf("EMPTY = %q, want empty string", v)
	}
}

func TestDependsOn_List(t *testing.T) {
	input := "depends_on:\n  - db\n  - redis"
	var s struct {
		DependsOn DependsOn `yaml:"depends_on"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	expected := []string{"db", "redis"}
	if len(s.DependsOn) != len(expected) {
		t.Fatalf("expected %d deps, got %d: %v", len(expected), len(s.DependsOn), s.DependsOn)
	}
	for i, v := range expected {
		if s.DependsOn[i] != v {
			t.Errorf("depends_on[%d] = %q, want %q", i, s.DependsOn[i], v)
		}
	}
}

func TestDependsOn_Map(t *testing.T) {
	input := "depends_on:\n  db:\n    condition: service_started"
	var s struct {
		DependsOn DependsOn `yaml:"depends_on"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	found := false
	for _, d := range s.DependsOn {
		if d == "db" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected depends_on to contain 'db', got %v", s.DependsOn)
	}
}

func TestStringOrSlice_String(t *testing.T) {
	input := "env_file: .env"
	var s struct {
		EnvFile StringOrSlice `yaml:"env_file"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(s.EnvFile) != 1 || s.EnvFile[0] != ".env" {
		t.Errorf("env_file = %v, want [.env]", s.EnvFile)
	}
}

func TestStringOrSlice_List(t *testing.T) {
	input := "env_file:\n  - .env\n  - .env.local"
	var s struct {
		EnvFile StringOrSlice `yaml:"env_file"`
	}
	if err := yaml.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	expected := []string{".env", ".env.local"}
	if len(s.EnvFile) != len(expected) {
		t.Fatalf("expected %d entries, got %d: %v", len(expected), len(s.EnvFile), s.EnvFile)
	}
	for i, v := range expected {
		if s.EnvFile[i] != v {
			t.Errorf("env_file[%d] = %q, want %q", i, s.EnvFile[i], v)
		}
	}
}
