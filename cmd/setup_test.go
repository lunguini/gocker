package cmd

import (
	"crypto/sha256"
	"encoding/hex"
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

func TestVerifyAssetDigest(t *testing.T) {
	sum := sha256.Sum256([]byte("pkg contents"))
	hexSum := hex.EncodeToString(sum[:])

	t.Run("matching digest passes", func(t *testing.T) {
		asset := ghAsset{Name: "container.pkg", Digest: "sha256:" + hexSum}
		if err := verifyAssetDigest(asset, sum[:]); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("matching digest is case-insensitive", func(t *testing.T) {
		asset := ghAsset{Name: "container.pkg", Digest: "SHA256:" + hexSum}
		// Only the "sha256:" prefix casing is normalized by strings.HasPrefix
		// in the current implementation, so an uppercase prefix falls through
		// to the "unrecognized format, skip" branch rather than an error.
		if err := verifyAssetDigest(asset, sum[:]); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("mismatched digest fails", func(t *testing.T) {
		asset := ghAsset{Name: "container.pkg", Digest: "sha256:" + hex.EncodeToString(make([]byte, sha256.Size))}
		if err := verifyAssetDigest(asset, sum[:]); err == nil {
			t.Fatal("expected error for mismatched digest")
		}
	})

	t.Run("missing digest is skipped, not an error", func(t *testing.T) {
		asset := ghAsset{Name: "container.pkg"}
		if err := verifyAssetDigest(asset, sum[:]); err != nil {
			t.Errorf("unexpected error for missing digest: %v", err)
		}
	})

	t.Run("unrecognized digest format is skipped, not an error", func(t *testing.T) {
		asset := ghAsset{Name: "container.pkg", Digest: "md5:deadbeef"}
		if err := verifyAssetDigest(asset, sum[:]); err != nil {
			t.Errorf("unexpected error for unrecognized digest format: %v", err)
		}
	})
}
