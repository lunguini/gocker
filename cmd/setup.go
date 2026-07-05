package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/lunguini/gocker/cmd/setup"
	"github.com/lunguini/gocker/engine"
	"github.com/urfave/cli/v3"
)

const (
	minMacOSVersion = 26
	releasesURL     = "https://api.github.com/repos/apple/container/releases/latest"
	releasesPageURL = "https://github.com/apple/container/releases"
)

type ghRelease struct {
	Assets []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	// Digest is a recent GitHub Releases API addition: "sha256:<hex>" of the
	// asset content. Older API responses (or older releases) omit it — treat
	// as optional and skip verification rather than failing when absent.
	Digest string `json:"digest"`
}

func newSetupCmd(eng engine.Runtime) *cli.Command {
	return &cli.Command{
		Name:  "setup",
		Usage: "Check prerequisites, install Apple Container, configure gocker",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "yes",
				Usage: "Non-interactive mode: use CI-friendly defaults (shared isolation), skip shell/docker-context prompts",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runSetup(ctx, eng, cmd.Bool("yes"))
		},
	}
}

func runSetup(ctx context.Context, eng engine.Runtime, nonInteractive bool) error {
	// Step 1: Check macOS version
	fmt.Print("Checking macOS version... ")
	verOut, err := exec.CommandContext(ctx, "sw_vers", "-productVersion").Output()
	if err != nil {
		return fmt.Errorf("failed to get macOS version: %w", err)
	}
	major, err := parseMacOSVersion(strings.TrimSpace(string(verOut)))
	if err != nil {
		return fmt.Errorf("failed to parse macOS version: %w", err)
	}
	if major < minMacOSVersion {
		fmt.Println("X")
		return fmt.Errorf("macOS %d+ required, found macOS %d", minMacOSVersion, major)
	}
	fmt.Printf("macOS %d OK\n", major)

	// Step 2: Check architecture
	fmt.Print("Checking architecture... ")
	if runtime.GOARCH != "arm64" {
		fmt.Println("X")
		return fmt.Errorf("apple silicon (arm64) required, found %s", runtime.GOARCH)
	}
	fmt.Println("arm64 OK")

	// Step 3: Check/install container binary
	fmt.Print("Checking Apple Container CLI... ")
	if _, err := os.Stat(eng.BinaryPath()); err != nil {
		if !os.IsNotExist(err) {
			fmt.Println("X")
			return fmt.Errorf("cannot access container binary at %s: %w", eng.BinaryPath(), err)
		}
		fmt.Println("not found")
		if err := installContainer(ctx); err != nil {
			return err
		}
	} else {
		fmt.Println("OK")
	}

	// Step 4: Run container system start
	fmt.Println("Initializing container system...")
	if err := eng.ExecInteractive(ctx, "system", "start"); err != nil {
		return fmt.Errorf("container system start failed: %w", err)
	}
	fmt.Println("Container system OK")

	// Step 5: Create ~/.gocker/ directory
	fmt.Print("Creating gocker directory... ")
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	gockerDir := filepath.Join(home, ".gocker")
	if err := os.MkdirAll(gockerDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", gockerDir, err)
	}
	fmt.Println("OK")

	// Step 6: Interactive configuration wizard.
	if err := setup.RunWizard(ctx, setup.Options{NonInteractive: nonInteractive}); err != nil {
		return fmt.Errorf("configuration wizard: %w", err)
	}

	// Step 7: Summary
	fmt.Println("\nGocker is ready! Run 'gocker ps' to get started.")
	return nil
}

func installContainer(ctx context.Context) error {
	fmt.Println("Downloading Apple Container from GitHub releases...")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch releases: %w\nInstall manually from %s", err, releasesPageURL)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned %s\nInstall manually from %s", resp.Status, releasesPageURL)
	}

	var release ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("failed to parse release JSON: %w", err)
	}

	asset, err := findInstallerAsset(release.Assets)
	if err != nil {
		return err
	}

	fmt.Printf("Downloading %s...\n", asset.Name)
	tmpFile, err := os.CreateTemp("", "container-*.pkg")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}
	dlResp, err := http.DefaultClient.Do(dlReq)
	if err != nil {
		return fmt.Errorf("failed to download installer: %w", err)
	}
	defer func() { _ = dlResp.Body.Close() }()

	if dlResp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download installer: HTTP %s", dlResp.Status)
	}

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmpFile, hasher), dlResp.Body); err != nil {
		return fmt.Errorf("failed to write installer: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close installer file: %w", err)
	}

	if err := verifyAssetDigest(asset, hasher.Sum(nil)); err != nil {
		return err
	}

	fmt.Println("Installing Apple Container requires administrator privileges.")
	installCmd := exec.CommandContext(ctx, "sudo", "installer", "-pkg", tmpFile.Name(), "-target", "/")
	installCmd.Stdin = os.Stdin
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("installer failed: %w", err)
	}

	fmt.Println("Apple Container installed successfully.")
	return nil
}

func parseMacOSVersion(version string) (int, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return 0, fmt.Errorf("empty version string")
	}
	parts := strings.Split(version, ".")
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid version %q: %w", version, err)
	}
	return major, nil
}

// verifyAssetDigest checks sum (a raw sha256 digest, as from hasher.Sum(nil))
// against asset.Digest ("sha256:<hex>"), when the release API provided one.
// A missing digest is not an error — it's logged and skipped, since not all
// GitHub API versions/releases populate the field, and macOS's own installer
// signature check still runs afterward.
func verifyAssetDigest(asset ghAsset, sum []byte) error {
	if asset.Digest == "" {
		fmt.Println("Note: release metadata has no digest for this asset; skipping checksum verification (installer signature is still checked).")
		return nil
	}
	const prefix = "sha256:"
	if !strings.HasPrefix(asset.Digest, prefix) {
		fmt.Printf("Note: unrecognized digest format %q; skipping checksum verification.\n", asset.Digest)
		return nil
	}
	want := strings.ToLower(strings.TrimPrefix(asset.Digest, prefix))
	got := hex.EncodeToString(sum)
	if got != want {
		return fmt.Errorf("downloaded installer %s failed checksum verification: expected sha256:%s, got sha256:%s", asset.Name, want, got)
	}
	fmt.Println("Checksum verified OK.")
	return nil
}

func findInstallerAsset(assets []ghAsset) (ghAsset, error) {
	for _, a := range assets {
		if strings.HasSuffix(a.Name, "-installer-signed.pkg") {
			return a, nil
		}
	}
	return ghAsset{}, fmt.Errorf("no installer package found in release assets\nInstall manually from %s", releasesPageURL)
}
