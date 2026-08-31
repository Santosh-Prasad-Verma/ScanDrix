package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"
)

type UpdateInfo struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	UpdateAvailable bool  `json:"update_available"`
	DownloadURL    string `json:"download_url"`
}

// CheckUpdate queries the latest ScanDrix CLI release version.
func CheckUpdate(currentVersion, serverURL string) (*UpdateInfo, error) {
	if serverURL == "" {
		serverURL = "https://api.github.com/repos/scandrix/backend/releases/latest"
	}

	info := &UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  currentVersion,
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(serverURL)
	if err != nil {
		return info, nil // Non-blocking check
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return info, nil
	}

	var ghRelease struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghRelease); err == nil && ghRelease.TagName != "" {
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

	resp, err := http.Get(downloadURL)
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
