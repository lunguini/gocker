package compose

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasFileFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"no flags", []string{"up", "-d"}, false},
		{"short -f separate", []string{"-f", "docker-compose.yml", "up"}, true},
		{"short -f attached", []string{"-f=docker-compose.yml", "up"}, true},
		{"long --file separate", []string{"--file", "docker-compose.yml", "up"}, true},
		{"long --file attached", []string{"--file=docker-compose.yml", "up"}, true},
		{"--project-directory separate", []string{"--project-directory", "/tmp/proj", "up"}, true},
		{"--project-directory attached", []string{"--project-directory=/tmp/proj", "up"}, true},
		{"other flags only", []string{"-p", "myproj", "--profile", "dev", "up"}, false},
		{"empty", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasFileFlag(tc.args); got != tc.want {
				t.Errorf("hasFileFlag(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestReferencedEnvVars_ScansExplicitFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "custom.yml")
	body := "services:\n  web:\n    image: ${IMAGE}\n    environment:\n      - TOKEN=$SECRET_TOKEN\n      - HOST=${DB_HOST:-localhost}\n"
	if err := os.WriteFile(f, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := referencedEnvVars([]string{"-f", f, "up"}, dir)
	for _, want := range []string{"IMAGE", "SECRET_TOKEN", "DB_HOST"} {
		if !got[want] {
			t.Errorf("expected %q to be referenced, set=%v", want, got)
		}
	}
	if got["AWS_SECRET_ACCESS_KEY"] {
		t.Error("unreferenced var must not be in the allowlist")
	}
}

func TestReferencedEnvVars_DefaultCandidatesInCwd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  db:\n    image: ${DB_IMAGE}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := referencedEnvVars([]string{"up", "-d"}, dir)
	if !got["DB_IMAGE"] {
		t.Errorf("expected DB_IMAGE from default compose file, set=%v", got)
	}
}

func TestShouldForwardEnv_AllowlistAndControlPrefixes(t *testing.T) {
	ref := map[string]bool{"IMAGE": true}
	cases := map[string]bool{
		"IMAGE":                 true,  // referenced
		"COMPOSE_PROJECT_NAME":  true,  // control prefix
		"DOCKER_HOST":           true,  // control prefix
		"AWS_SECRET_ACCESS_KEY": false, // unrelated secret
		"HOME":                  false, // not referenced
	}
	for key, want := range cases {
		if got := shouldForwardEnv(key, ref); got != want {
			t.Errorf("shouldForwardEnv(%q) = %v, want %v", key, got, want)
		}
	}
}
