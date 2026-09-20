// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package updater_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/scandrix/backend/internal/cli/updater"
)

func TestCheckUpdate_UpdateAvailable(t *testing.T) {
	expectedAsset := fmt.Sprintf("scandrix-cli-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		expectedAsset += ".exe"
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v2.5.0",
			"assets": []map[string]any{
				{
					"name":                 expectedAsset,
					"browser_download_url": "https://downloads.scandrix.io/v2.5.0/" + expectedAsset,
				},
				{
					"name":                 "other-asset.tar.gz",
					"browser_download_url": "https://downloads.scandrix.io/other.tar.gz",
				},
			},
		})
	}))
	defer server.Close()

	info, err := updater.CheckUpdate("v2.4.0", server.URL)
	if err != nil {
		t.Fatalf("CheckUpdate failed: %v", err)
	}

	if !info.UpdateAvailable {
		t.Fatalf("expected update to be available")
	}
	if info.LatestVersion != "v2.5.0" {
		t.Fatalf("expected latest version v2.5.0, got %s", info.LatestVersion)
	}
	if info.DownloadURL != "https://downloads.scandrix.io/v2.5.0/"+expectedAsset {
		t.Fatalf("unexpected download URL: %s", info.DownloadURL)
	}
}

func TestCheckUpdate_UpToDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v2.5.0",
			"assets":   []map[string]any{},
		})
	}))
	defer server.Close()

	info, err := updater.CheckUpdate("v2.5.0", server.URL)
	if err != nil {
		t.Fatalf("CheckUpdate failed: %v", err)
	}

	if info.UpdateAvailable {
		t.Fatalf("expected update NOT to be available")
	}
}

func TestCheckUpdate_DevVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v3.0.0",
			"assets":   []map[string]any{},
		})
	}))
	defer server.Close()

	info, err := updater.CheckUpdate("dev", server.URL)
	if err != nil {
		t.Fatalf("CheckUpdate failed: %v", err)
	}

	// Dev versions ignore update notifications
	if info.UpdateAvailable {
		t.Fatalf("dev build should not flag update available")
	}
}

func TestCheckUpdate_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := updater.CheckUpdate("v1.0.0", server.URL)
	if err == nil {
		t.Fatalf("expected error from 500 response, got nil")
	}
}
