package setup

import "testing"

func TestParseCurrentContextEndpoint(t *testing.T) {
	output := `[
		{
			"Name": "default",
			"Endpoints": {"docker": {"Host": "unix:///var/run/docker.sock"}},
			"Current": false
		},
		{
			"Name": "gocker",
			"Endpoints": {"docker": {"Host": "unix:///Users/me/.gocker/gocker.sock"}},
			"Current": true
		}
	]`
	name, host, err := parseCurrentContext([]byte(output))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "gocker" {
		t.Errorf("name: got %q, want gocker", name)
	}
	if host != "unix:///Users/me/.gocker/gocker.sock" {
		t.Errorf("host: got %q", host)
	}
}

func TestParseCurrentContextNoneCurrent(t *testing.T) {
	output := `[{"Name": "default", "Endpoints": {"docker": {"Host": "unix:///var/run/docker.sock"}}, "Current": false}]`
	name, _, err := parseCurrentContext([]byte(output))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "" {
		t.Errorf("expected empty name, got %q", name)
	}
}

func TestGockerContextAlreadyPoints(t *testing.T) {
	output := `[{"Name":"gocker","Endpoints":{"docker":{"Host":"unix:///Users/me/.gocker/gocker.sock"}},"Current":true}]`
	if !gockerContextIsCurrent([]byte(output), "/Users/me/.gocker/gocker.sock") {
		t.Errorf("expected current context to match socket")
	}
}
