package cmd

import (
	"context"
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
}

func newSetupCmd(eng *engine.Engine) *cli.Command {
	return &cli.Command{
		Name:  "setup",
		Usage: "Check prerequisites and install Apple Container",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runSetup(ctx, eng)
		},
	}
}

func runSetup(ctx context.Context, eng *engine.Engine) error {
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
		return fmt.Errorf("Apple Silicon (arm64) required, found %s", runtime.GOARCH)
	}
	fmt.Println("arm64 OK")

	// Step 3: Check/install container binary
	fmt.Print("Checking Apple Container CLI... ")
	if _, err := os.Stat(eng.Binary); err != nil {
		if !os.IsNotExist(err) {
			fmt.Println("X")
			return fmt.Errorf("cannot access container binary at %s: %w", eng.Binary, err)
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

	// Step 6: Summary
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
	defer resp.Body.Close()

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
	defer os.Remove(tmpFile.Name())

	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}
	dlResp, err := http.DefaultClient.Do(dlReq)
	if err != nil {
		return fmt.Errorf("failed to download installer: %w", err)
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download installer: HTTP %s", dlResp.Status)
	}

	if _, err := io.Copy(tmpFile, dlResp.Body); err != nil {
		return fmt.Errorf("failed to write installer: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close installer file: %w", err)
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

func findInstallerAsset(assets []ghAsset) (ghAsset, error) {
	for _, a := range assets {
		if strings.HasSuffix(a.Name, "-installer-signed.pkg") {
			return a, nil
		}
	}
	return ghAsset{}, fmt.Errorf("no installer package found in release assets\nInstall manually from %s", releasesPageURL)
}
