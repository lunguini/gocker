package engine

import (
	"os"
	"testing"
)

func requireFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read testdata file %s: %v", path, err)
	}
	return data
}

func TestParseContainerListJSON(t *testing.T) {
	data := requireFile(t, "testdata/container_list.json")
	containers, err := parseContainerListJSON(data)
	if err != nil {
		t.Fatalf("parseContainerListJSON returned error: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}

	t.Run("running container", func(t *testing.T) {
		c := containers[0]
		if c.ID != "abc123def456" {
			t.Errorf("ID = %q, want %q", c.ID, "abc123def456")
		}
		// containerInfoFromNested sets Name = getString(config, "id"), same as ID
		if c.Name != "abc123def456" {
			t.Errorf("Name = %q, want %q", c.Name, "abc123def456")
		}
		// Image is the full reference from Apple CLI
		if c.Image != "docker.io/library/nginx:latest" {
			t.Errorf("Image = %q, want %q", c.Image, "docker.io/library/nginx:latest")
		}
		if c.State != "running" {
			t.Errorf("State = %q, want %q", c.State, "running")
		}
		if c.IP != "192.168.64.3" {
			t.Errorf("IP = %q, want %q", c.IP, "192.168.64.3")
		}
		if c.Command != "/docker-entrypoint.sh" {
			t.Errorf("Command = %q, want %q", c.Command, "/docker-entrypoint.sh")
		}
		// Core Data epoch: 764188800 seconds from 2001-01-01 = 2025-03-20
		if c.Created.Year() != 2025 {
			t.Errorf("Created year = %d, want 2025", c.Created.Year())
		}
	})

	t.Run("stopped container", func(t *testing.T) {
		c := containers[1]
		if c.ID != "789xyz012abc" {
			t.Errorf("ID = %q, want %q", c.ID, "789xyz012abc")
		}
		if c.Name != "789xyz012abc" {
			t.Errorf("Name = %q, want %q", c.Name, "789xyz012abc")
		}
		if c.Image != "docker.io/library/postgres:16" {
			t.Errorf("Image = %q, want %q", c.Image, "docker.io/library/postgres:16")
		}
		if c.State != "stopped" {
			t.Errorf("State = %q, want %q", c.State, "stopped")
		}
		if c.IP != "192.168.64.5" {
			t.Errorf("IP = %q, want %q", c.IP, "192.168.64.5")
		}
		if c.Command != "/usr/local/bin/docker-entrypoint.sh" {
			t.Errorf("Command = %q, want %q", c.Command, "/usr/local/bin/docker-entrypoint.sh")
		}
	})
}

func TestParseContainerListNDJSON(t *testing.T) {
	data := requireFile(t, "testdata/container_list_ndjson.jsonl")
	containers, err := parseContainerListJSON(data)
	if err != nil {
		t.Fatalf("parseContainerListJSON (NDJSON) returned error: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}

	if containers[0].ID != "abc123def456" {
		t.Errorf("container[0].ID = %q, want %q", containers[0].ID, "abc123def456")
	}
	if containers[0].Name != "abc123def456" {
		t.Errorf("container[0].Name = %q, want %q", containers[0].Name, "abc123def456")
	}
	if containers[0].Image != "docker.io/library/nginx:latest" {
		t.Errorf("container[0].Image = %q, want %q", containers[0].Image, "docker.io/library/nginx:latest")
	}
	if containers[0].State != "running" {
		t.Errorf("container[0].State = %q, want %q", containers[0].State, "running")
	}
	if containers[1].ID != "789xyz012abc" {
		t.Errorf("container[1].ID = %q, want %q", containers[1].ID, "789xyz012abc")
	}
	if containers[1].Name != "789xyz012abc" {
		t.Errorf("container[1].Name = %q, want %q", containers[1].Name, "789xyz012abc")
	}
	if containers[1].State != "stopped" {
		t.Errorf("container[1].State = %q, want %q", containers[1].State, "stopped")
	}
}

func TestParseContainerListEmpty(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		result, err := parseContainerListJSON([]byte(""))
		if err != nil {
			t.Errorf("unexpected error for empty string: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil result for empty string, got %v", result)
		}
	})

	t.Run("empty array", func(t *testing.T) {
		result, err := parseContainerListJSON([]byte("[]"))
		if err != nil {
			t.Errorf("unexpected error for empty array: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil result for empty array, got %v", result)
		}
	})
}

func TestParseImageListJSON(t *testing.T) {
	data := requireFile(t, "testdata/image_list.json")
	images, err := parseImageListJSON(data)
	if err != nil {
		t.Fatalf("parseImageListJSON returned error: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}

	t.Run("nginx image", func(t *testing.T) {
		img := images[0]
		if img.Name != "nginx" {
			t.Errorf("Name = %q, want %q", img.Name, "nginx")
		}
		if img.Tag != "latest" {
			t.Errorf("Tag = %q, want %q", img.Tag, "latest")
		}
		if img.Digest != "sha256:a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2" {
			t.Errorf("Digest = %q, want sha256:a1b2...", img.Digest)
		}
		if img.Size != "28.9 MB" {
			t.Errorf("Size = %q, want %q", img.Size, "28.9 MB")
		}
		if img.Created.Year() != 2025 || img.Created.Month() != 1 || img.Created.Day() != 15 {
			t.Errorf("Created = %v, want 2025-01-15", img.Created)
		}
	})

	t.Run("postgres image", func(t *testing.T) {
		img := images[1]
		if img.Name != "postgres" {
			t.Errorf("Name = %q, want %q", img.Name, "postgres")
		}
		if img.Tag != "16" {
			t.Errorf("Tag = %q, want %q", img.Tag, "16")
		}
		if img.Size != "112.4 MB" {
			t.Errorf("Size = %q, want %q", img.Size, "112.4 MB")
		}
	})
}

func TestParseImageListNDJSON(t *testing.T) {
	data := requireFile(t, "testdata/image_list_ndjson.jsonl")
	images, err := parseImageListJSON(data)
	if err != nil {
		t.Fatalf("parseImageListJSON (NDJSON) returned error: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(images))
	}
	if images[0].Name != "nginx" {
		t.Errorf("images[0].Name = %q, want %q", images[0].Name, "nginx")
	}
	if images[1].Name != "postgres" {
		t.Errorf("images[1].Name = %q, want %q", images[1].Name, "postgres")
	}
}

func TestParseImageListEmpty(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		result, err := parseImageListJSON([]byte(""))
		if err != nil {
			t.Errorf("unexpected error for empty string: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil result for empty string, got %v", result)
		}
	})

	t.Run("empty array", func(t *testing.T) {
		result, err := parseImageListJSON([]byte("[]"))
		if err != nil {
			t.Errorf("unexpected error for empty array: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil result for empty array, got %v", result)
		}
	})
}

func TestParseReference(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantTag  string
	}{
		{"docker.io/library/ubuntu:24.04", "ubuntu", "24.04"},
		{"docker.io/myuser/myapp:v1", "myuser/myapp", "v1"},
		{"nginx:latest", "nginx", "latest"},
		{"nginx", "nginx", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, tag := parseReference(tt.input)
			if name != tt.wantName {
				t.Errorf("parseReference(%q) name = %q, want %q", tt.input, name, tt.wantName)
			}
			if tag != tt.wantTag {
				t.Errorf("parseReference(%q) tag = %q, want %q", tt.input, tag, tt.wantTag)
			}
		})
	}
}

func TestParseNetworkListJSON(t *testing.T) {
	data := requireFile(t, "testdata/network_list.json")
	networks, err := parseNetworkListJSON(data)
	if err != nil {
		t.Fatalf("parseNetworkListJSON returned error: %v", err)
	}
	if len(networks) != 2 {
		t.Fatalf("expected 2 networks, got %d", len(networks))
	}

	t.Run("bridge network", func(t *testing.T) {
		n := networks[0]
		if n.ID != "net-abc123" {
			t.Errorf("ID = %q, want %q", n.ID, "net-abc123")
		}
		if n.Name != "bridge" {
			t.Errorf("Name = %q, want %q", n.Name, "bridge")
		}
		if n.Driver != "bridge" {
			t.Errorf("Driver = %q, want %q", n.Driver, "bridge")
		}
		if n.Scope != "local" {
			t.Errorf("Scope = %q, want %q", n.Scope, "local")
		}
	})

	t.Run("custom network", func(t *testing.T) {
		n := networks[1]
		if n.ID != "net-def456" {
			t.Errorf("ID = %q, want %q", n.ID, "net-def456")
		}
		if n.Name != "my-custom-net" {
			t.Errorf("Name = %q, want %q", n.Name, "my-custom-net")
		}
	})
}

func TestParseVolumeListJSON(t *testing.T) {
	data := requireFile(t, "testdata/volume_list.json")
	volumes, err := parseVolumeListJSON(data)
	if err != nil {
		t.Fatalf("parseVolumeListJSON returned error: %v", err)
	}
	if len(volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(volumes))
	}

	t.Run("my-data volume", func(t *testing.T) {
		v := volumes[0]
		if v.Name != "my-data" {
			t.Errorf("Name = %q, want %q", v.Name, "my-data")
		}
		if v.Driver != "local" {
			t.Errorf("Driver = %q, want %q", v.Driver, "local")
		}
		if v.Mountpoint != "/var/lib/volumes/my-data" {
			t.Errorf("Mountpoint = %q, want %q", v.Mountpoint, "/var/lib/volumes/my-data")
		}
		if v.Created.Year() != 2025 || v.Created.Month() != 6 || v.Created.Day() != 1 {
			t.Errorf("Created = %v, want 2025-06-01", v.Created)
		}
	})

	t.Run("postgres-vol volume", func(t *testing.T) {
		v := volumes[1]
		if v.Name != "postgres-vol" {
			t.Errorf("Name = %q, want %q", v.Name, "postgres-vol")
		}
		if v.Mountpoint != "/var/lib/volumes/postgres-vol" {
			t.Errorf("Mountpoint = %q, want %q", v.Mountpoint, "/var/lib/volumes/postgres-vol")
		}
	})
}

func TestGetString(t *testing.T) {
	m := map[string]any{
		"ID":   "abc123",
		"name": "test-container",
	}

	t.Run("first key matches", func(t *testing.T) {
		result := getString(m, "ID")
		if result != "abc123" {
			t.Errorf("getString(m, \"ID\") = %q, want %q", result, "abc123")
		}
	})

	t.Run("second key matches", func(t *testing.T) {
		result := getString(m, "id", "ID")
		if result != "abc123" {
			t.Errorf("getString(m, \"id\", \"ID\") = %q, want %q", result, "abc123")
		}
	})

	t.Run("case sensitive lookup", func(t *testing.T) {
		result := getString(m, "name")
		if result != "test-container" {
			t.Errorf("getString(m, \"name\") = %q, want %q", result, "test-container")
		}
	})

	t.Run("missing key returns empty", func(t *testing.T) {
		result := getString(m, "nonexistent", "alsoMissing")
		if result != "" {
			t.Errorf("getString(m, \"nonexistent\") = %q, want empty string", result)
		}
	})
}
