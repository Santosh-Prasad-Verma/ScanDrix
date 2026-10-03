package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"

	"github.com/scandrix/backend/internal/netguard"
)

type UpdateInfo struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	DownloadURL     string `json:"download_url"`
}

// CheckUpdate queries the latest ScanDrix CLI release version.
func CheckUpdate(currentVersion, serverURL string) (*UpdateInfo, error) {
	if serverURL == "" {
		repo := os.Getenv("SCANDRIX_UPDATE_REPO")
		if repo == "" {
			repo = "scandrix/backend"
		}
		serverURL = fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	}

	info := &UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  currentVersion,
	}

	// The release URL is either built from SCANDRIX_UPDATE_REPO or passed in by
	// a caller, so it is validated before the CLI reaches out to it. Without
	// this, a value aimed at an internal address turns the update check into an
	// SSRF probe and its response into attacker-chosen "latest version" text.
	// Loopback http stays permitted so an httptest server works in tests.
	releaseOpts := netguard.Options{AllowHTTPLoopback: true}
	parsed, err := netguard.Validate(serverURL, releaseOpts)
	if err != nil {
		return info, fmt.Errorf("refusing update server URL: %w", err)
	}

	client := netguard.NewClient(5, releaseOpts)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, parsed.String(), nil) // #nosec G704 -- serverURL is validated by netguard.Validate before the request is built
	if err != nil {
		return info, fmt.Errorf("failed creating request: %w", err)
	}
	req.Header.Set("User-Agent", "scandrix-cli/"+currentVersion)

	resp, err := client.Do(req) // #nosec G704 -- client is netguard.NewClient, which revalidates every redirect hop
	if err != nil {
		return info, fmt.Errorf("failed reaching update server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return info, fmt.Errorf("update server returned HTTP %d", resp.StatusCode)
	}

	var ghRelease struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghRelease); err != nil {
		return info, fmt.Errorf("failed decoding release data: %w", err)
	}
	if ghRelease.TagName == "" {
		return info, fmt.Errorf("no tag found in latest release")
	}

	info.LatestVersion = ghRelease.TagName
	if info.LatestVersion != currentVersion && currentVersion != "dev" {
		info.UpdateAvailable = true
		expectedAsset := fmt.Sprintf("scandrix-cli-%s-%s", runtime.GOOS, runtime.GOARCH)
		if runtime.GOOS == "windows" {
			expectedAsset += ".exe"
		}
		for _, asset := range ghRelease.Assets {
			if asset.Name == expectedAsset {
				info.DownloadURL = asset.BrowserDownloadURL
				break
			}
		}
	}

	return info, nil
}

// ApplyUpdate downloads the binary and replaces the running executable atomically.
func ApplyUpdate(downloadURL string) error {
	if downloadURL == "" {
		return fmt.Errorf("no download URL provided for update")
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed locating current executable: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed creating download request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed downloading update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with HTTP %d", resp.StatusCode)
	}

	tmpFile := execPath + ".tmp"
	out, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed creating temporary binary: %w", err)
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed writing update payload: %w", err)
	}
	out.Close()

	if err := os.Rename(tmpFile, execPath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed replacing binary: %w", err)
	}

	return nil
}
