package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Parser tests ---

func TestLoad_AutoDiscover(t *testing.T) {
	tmpDir := t.TempDir()
	composeContent := `services:
  web:
    image: nginx:latest
`
	if err := os.WriteFile(filepath.Join(tmpDir, "docker-compose.yml"), []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	cf, path, err := Load("")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cf == nil {
		t.Fatal("expected non-nil ComposeFile")
	}
	if _, ok := cf.Services["web"]; !ok {
		t.Error("expected 'web' service")
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
}

func TestLoad_ExplicitFile(t *testing.T) {
	tmpDir := t.TempDir()
	composeContent := `services:
  api:
    image: golang:1.22
`
	filePath := filepath.Join(tmpDir, "compose.yaml")
	if err := os.WriteFile(filePath, []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}

	cf, _, err := Load(filePath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if _, ok := cf.Services["api"]; !ok {
		t.Error("expected 'api' service")
	}
}

func TestLoad_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	_, _, err := Load("")
	if err == nil {
		t.Fatal("expected error for missing compose file")
	}
}

func TestLoad_NoServices(t *testing.T) {
	tmpDir := t.TempDir()
	composeContent := `services:
`
	filePath := filepath.Join(tmpDir, "docker-compose.yml")
	if err := os.WriteFile(filePath, []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, _, err := Load(filePath)
	if err == nil {
		t.Fatal("expected error for empty services")
	}
}

func TestLoad_EnvSubstitution(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TEST_IMAGE_TAG", "3.19")
	composeContent := `services:
  app:
    image: alpine:${TEST_IMAGE_TAG}
`
	filePath := filepath.Join(tmpDir, "docker-compose.yml")
	if err := os.WriteFile(filePath, []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}

	cf, _, err := Load(filePath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cf.Services["app"].Image != "alpine:3.19" {
		t.Errorf("image = %q, want %q", cf.Services["app"].Image, "alpine:3.19")
	}
}

func TestLoad_EnvDefault(t *testing.T) {
	tmpDir := t.TempDir()
	// Ensure MISSING_VAR_FOR_TEST is not set
	os.Unsetenv("MISSING_VAR_FOR_TEST")
	composeContent := `services:
  app:
    image: alpine:${MISSING_VAR_FOR_TEST:-fallback}
`
	filePath := filepath.Join(tmpDir, "docker-compose.yml")
	if err := os.WriteFile(filePath, []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}

	cf, _, err := Load(filePath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cf.Services["app"].Image != "alpine:fallback" {
		t.Errorf("image = %q, want %q", cf.Services["app"].Image, "alpine:fallback")
	}
}

func TestLoad_VersionField(t *testing.T) {
	tmpDir := t.TempDir()
	composeContent := `version: "3.9"
services:
  web:
    image: nginx:latest
`
	filePath := filepath.Join(tmpDir, "docker-compose.yml")
	if err := os.WriteFile(filePath, []byte(composeContent), 0644); err != nil {
		t.Fatal(err)
	}

	cf, _, err := Load(filePath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if _, ok := cf.Services["web"]; !ok {
		t.Error("expected 'web' service")
	}
}

// --- Dependency ordering tests ---

func TestDependencyOrder_NoDeps(t *testing.T) {
	cf := &ComposeFile{
		Services: map[string]Service{
			"a": {Image: "alpine"},
			"b": {Image: "alpine"},
			"c": {Image: "alpine"},
		},
	}
	order, err := DependencyOrder(cf)
	if err != nil {
		t.Fatalf("DependencyOrder error: %v", err)
	}
	if len(order) != 3 {
		t.Fatalf("expected 3 services, got %d", len(order))
	}
	// All three should be present
	seen := map[string]bool{}
	for _, s := range order {
		seen[s] = true
	}
	for _, name := range []string{"a", "b", "c"} {
		if !seen[name] {
			t.Errorf("missing service %q in order", name)
		}
	}
}

func TestDependencyOrder_Linear(t *testing.T) {
	cf := &ComposeFile{
		Services: map[string]Service{
			"a": {Image: "alpine", DependsOn: DependsOn{"b"}},
			"b": {Image: "alpine", DependsOn: DependsOn{"c"}},
			"c": {Image: "alpine"},
		},
	}
	order, err := DependencyOrder(cf)
	if err != nil {
		t.Fatalf("DependencyOrder error: %v", err)
	}
	// c must come before b, b before a
	indexOf := map[string]int{}
	for i, s := range order {
		indexOf[s] = i
	}
	if indexOf["c"] >= indexOf["b"] {
		t.Errorf("c (idx %d) should come before b (idx %d)", indexOf["c"], indexOf["b"])
	}
	if indexOf["b"] >= indexOf["a"] {
		t.Errorf("b (idx %d) should come before a (idx %d)", indexOf["b"], indexOf["a"])
	}
}

func TestDependencyOrder_Diamond(t *testing.T) {
	cf := &ComposeFile{
		Services: map[string]Service{
			"a": {Image: "alpine", DependsOn: DependsOn{"b", "c"}},
			"b": {Image: "alpine", DependsOn: DependsOn{"d"}},
			"c": {Image: "alpine", DependsOn: DependsOn{"d"}},
			"d": {Image: "alpine"},
		},
	}
	order, err := DependencyOrder(cf)
	if err != nil {
		t.Fatalf("DependencyOrder error: %v", err)
	}
	indexOf := map[string]int{}
	for i, s := range order {
		indexOf[s] = i
	}
	if indexOf["d"] >= indexOf["b"] || indexOf["d"] >= indexOf["c"] {
		t.Errorf("d should come before b and c: %v", order)
	}
	if indexOf["a"] != len(order)-1 {
		t.Errorf("a should be last: %v", order)
	}
}

func TestDependencyOrder_Cycle(t *testing.T) {
	cf := &ComposeFile{
		Services: map[string]Service{
			"a": {Image: "alpine", DependsOn: DependsOn{"b"}},
			"b": {Image: "alpine", DependsOn: DependsOn{"a"}},
		},
	}
	_, err := DependencyOrder(cf)
	if err == nil {
		t.Fatal("expected error for circular dependency")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "circular") && !strings.Contains(strings.ToLower(err.Error()), "cycle") {
		t.Errorf("error should mention circular/cycle, got: %v", err)
	}
}

func TestDependencyOrder_MissingDep(t *testing.T) {
	cf := &ComposeFile{
		Services: map[string]Service{
			"a": {Image: "alpine", DependsOn: DependsOn{"nonexistent"}},
		},
	}
	_, err := DependencyOrder(cf)
	if err == nil {
		t.Fatal("expected error for missing dependency")
	}
}

// --- Orchestrator function tests ---

func TestContainerNameForService(t *testing.T) {
	// Default name
	svc := Service{Image: "nginx"}
	name := containerNameForService("myproject", "web", svc)
	if name != "myproject-web-1" {
		t.Errorf("default name = %q, want %q", name, "myproject-web-1")
	}

	// Custom container_name
	svc = Service{Image: "nginx", ContainerName: "custom-web"}
	name = containerNameForService("myproject", "web", svc)
	if name != "custom-web" {
		t.Errorf("custom name = %q, want %q", name, "custom-web")
	}
}

func TestResolveVolume_NamedVolume(t *testing.T) {
	cf := &ComposeFile{
		Volumes: map[string]Volume{
			"dbdata": {},
		},
	}
	result := resolveVolume("myproject", "dbdata:/var/lib/data", cf)
	if result != "myproject_dbdata:/var/lib/data" {
		t.Errorf("named volume = %q, want %q", result, "myproject_dbdata:/var/lib/data")
	}
}

func TestResolveVolume_BindMount(t *testing.T) {
	cf := &ComposeFile{}

	// Relative path
	result := resolveVolume("myproject", "./data:/app/data", cf)
	if result != "./data:/app/data" {
		t.Errorf("relative bind mount = %q, want %q", result, "./data:/app/data")
	}

	// Absolute path
	result = resolveVolume("myproject", "/abs/path:/data", cf)
	if result != "/abs/path:/data" {
		t.Errorf("absolute bind mount = %q, want %q", result, "/abs/path:/data")
	}
}

func TestIsNamedVolume(t *testing.T) {
	tests := []struct {
		spec string
		want bool
	}{
		{"mydata:/data", true},
		{"./data:/data", false},
		{"/abs:/data", false},
		{"~/home:/data", false},
	}
	for _, tt := range tests {
		got := isNamedVolume(tt.spec)
		if got != tt.want {
			t.Errorf("isNamedVolume(%q) = %v, want %v", tt.spec, got, tt.want)
		}
	}
}

func TestVolumeDataDirEnv_Postgres(t *testing.T) {
	svc := Service{
		Image:   "postgres:16",
		Volumes: []string{"pgdata:/var/lib/postgresql/data"},
	}
	env := volumeDataDirEnv(svc)
	if v, ok := env["PGDATA"]; !ok {
		t.Error("expected PGDATA to be set")
	} else if v != "/var/lib/postgresql/data/data" {
		t.Errorf("PGDATA = %q, want %q", v, "/var/lib/postgresql/data/data")
	}
}

func TestVolumeDataDirEnv_MySQL(t *testing.T) {
	svc := Service{
		Image:   "mysql:8",
		Volumes: []string{"mysqldata:/var/lib/mysql"},
	}
	env := volumeDataDirEnv(svc)
	if v, ok := env["MYSQL_DATADIR"]; !ok {
		t.Error("expected MYSQL_DATADIR to be set")
	} else if v != "/var/lib/mysql/data" {
		t.Errorf("MYSQL_DATADIR = %q, want %q", v, "/var/lib/mysql/data")
	}
}

func TestVolumeDataDirEnv_NoMatch(t *testing.T) {
	svc := Service{
		Image:   "nginx:latest",
		Volumes: []string{"html:/usr/share/nginx/html"},
	}
	env := volumeDataDirEnv(svc)
	if len(env) != 0 {
		t.Errorf("expected empty map, got %v", env)
	}
}

func TestVolumeDataDirEnv_UserOverride(t *testing.T) {
	// volumeDataDirEnv should still return the value regardless;
	// the override logic happens in buildRunArgs
	svc := Service{
		Image:   "postgres:16",
		Volumes: []string{"pgdata:/var/lib/postgresql/data"},
	}
	env := volumeDataDirEnv(svc)
	if _, ok := env["PGDATA"]; !ok {
		t.Error("expected PGDATA to be set even when user would override")
	}
}

func TestProjectName(t *testing.T) {
	name := ProjectName("/Users/me/projects/my-app/docker-compose.yml")
	if name != "my-app" {
		t.Errorf("ProjectName = %q, want %q", name, "my-app")
	}
}

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"My App!", "myapp"},
		{"", "default"},
		{"hello-world", "hello-world"},
		{"UPPER_case", "uppercase"},
	}
	for _, tt := range tests {
		got := sanitizeName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAppendUnique(t *testing.T) {
	s := []string{"a", "b"}
	s = appendUnique(s, "c")
	if len(s) != 3 {
		t.Fatalf("expected 3 items, got %d", len(s))
	}
	s = appendUnique(s, "b")
	if len(s) != 3 {
		t.Fatalf("expected 3 items after duplicate, got %d", len(s))
	}
	if s[2] != "c" {
		t.Errorf("last item = %q, want %q", s[2], "c")
	}
}
