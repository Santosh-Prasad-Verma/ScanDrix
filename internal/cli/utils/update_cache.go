// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// UpdateCacheRecord stores timestamp and latest known version to throttle remote checks.
type UpdateCacheRecord struct {
	LastCheckedAt time.Time `json:"last_checked_at"`
	LatestVersion string    `json:"latest_version"`
	UpdateURL     string    `json:"update_url,omitempty"`
}

// UpdateCachePath returns ~/.scandrix/update_cache.json.
func UpdateCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".scandrix", "update_cache.json")
}

// CheckForUpdates asynchronously checks if a newer version of ScanDrix is available.
func CheckForUpdates(currentVersion string) {
	if os.Getenv("SCANDRIX_DISABLE_UPDATE_CHECK") == "1" || os.Getenv("CI") != "" {
		return
	}

	go func() {
		cachePath := UpdateCachePath()
		var cache UpdateCacheRecord

		if data, err := os.ReadFile(cachePath); err == nil {
			_ = json.Unmarshal(data, &cache)
		}

		// Check at most once every 24 hours
		if time.Since(cache.LastCheckedAt) < 24*time.Hour && cache.LatestVersion != "" {
			notifyIfNewer(currentVersion, cache.LatestVersion, cache.UpdateURL)
			return
		}

		// Remote check with short timeout to never degrade CLI latency
		client := &http.Client{Timeout: 2 * time.Second}
		req, err := http.NewRequest("GET", "https://api.github.com/repos/scandrix/scandrix/releases/latest", nil)
		if err != nil {
			return
		}
		req.Header.Set("User-Agent", "scandrix-cli/"+currentVersion)

		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil {
				_ = resp.Body.Close()
			}
			return
		}
		defer resp.Body.Close()

		var ghRelease struct {
			TagName string `json:"tag_name"`
			HTMLURL string `json:"html_url"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&ghRelease); err != nil {
			return
		}

		latest := strings.TrimPrefix(strings.TrimSpace(ghRelease.TagName), "v")
		if latest == "" {
			return
		}

		cache.LastCheckedAt = time.Now()
		cache.LatestVersion = latest
		cache.UpdateURL = ghRelease.HTMLURL

		_ = os.MkdirAll(filepath.Dir(cachePath), 0700)
		if cacheBytes, err := json.Marshal(cache); err == nil {
			_ = os.WriteFile(cachePath, cacheBytes, 0600)
		}

		notifyIfNewer(currentVersion, latest, cache.UpdateURL)
	}()
}

func notifyIfNewer(current, latest, updateURL string) {
	cur := strings.TrimPrefix(strings.TrimSpace(current), "v")
	lat := strings.TrimPrefix(strings.TrimSpace(latest), "v")
	if cur == "" || lat == "" || cur == "dev" || cur == "test" || cur >= lat {
		return
	}

	fmt.Fprintf(os.Stderr, "\n\033[33m╭──────────────────────────────────────────────────────────╮\033[0m\n")
	fmt.Fprintf(os.Stderr, "\033[33m│\033[0m  Update available: \033[90mv%s\033[0m -> \033[32mv%s\033[0m                           \033[33m│\033[0m\n", cur, lat)
	if updateURL != "" {
		fmt.Fprintf(os.Stderr, "\033[33m│\033[0m  Run: \033[36mscandrix upgrade\033[0m or download at:                   \033[33m│\033[0m\n")
		fmt.Fprintf(os.Stderr, "\033[33m│\033[0m  \033[4;34m%s\033[0m\033[33m│\033[0m\n", padRight(updateURL, 56))
	} else {
		fmt.Fprintf(os.Stderr, "\033[33m│\033[0m  Run: \033[36mscandrix upgrade\033[0m to update to the latest version    \033[33m│\033[0m\n")
	}
	fmt.Fprintf(os.Stderr, "\033[33m╰──────────────────────────────────────────────────────────╯\033[0m\n\n")
}

func padRight(s string, length int) string {
	if len(s) >= length {
		return s[:length]
	}
	return s + strings.Repeat(" ", length-len(s))
}
