package cmd

import (
	"testing"
)

func TestParseMacOSVersion(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"26.0", 26, false},
		{"26.1.1", 26, false},
		{"26", 26, false},
		{"15.4.1", 15, false},
		{"", 0, true},
		{"abc", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseMacOSVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMacOSVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseMacOSVersion(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestFindInstallerAsset(t *testing.T) {
	assets := []ghAsset{
		{Name: "container-0.10.0-installer-signed.pkg", DownloadURL: "https://example.com/pkg"},
		{Name: "container-dSYM.zip", DownloadURL: "https://example.com/dsym"},
		{Name: "Source code (zip)", DownloadURL: "https://example.com/src"},
	}

	asset, err := findInstallerAsset(assets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if asset.Name != "container-0.10.0-installer-signed.pkg" {
		t.Errorf("expected pkg asset, got %q", asset.Name)
	}
}

func TestFindInstallerAsset_NotFound(t *testing.T) {
	assets := []ghAsset{
		{Name: "container-dSYM.zip", DownloadURL: "https://example.com/dsym"},
	}

	_, err := findInstallerAsset(assets)
	if err == nil {
		t.Fatal("expected error when no installer asset found")
	}
}
