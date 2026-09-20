// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/cli/services/api"
)

func TestCentralizedConfigEndpoints(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/config/centralized/status", func(w http.ResponseWriter, r *http.Request) {
		status := api.CentralizedConfigStatus{
			Enabled: true,
			SelectedRepository: &api.TrackedRepository{
				Namespace: "org/central-config",
				Provider:  "github",
			},
			LastSyncedAt: "2026-09-12T00:00:00Z",
		}
		_ = json.NewEncoder(w).Encode(status)
	})

	mux.HandleFunc("/api/v1/config/centralized/init", func(w http.ResponseWriter, r *http.Request) {
		resp := api.CentralizedActionResponse{
			Success: true,
			Message: "Centralized configuration enabled",
			PRURL:   "https://github.com/org/central-config/pull/12",
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/v1/config/centralized/sync", func(w http.ResponseWriter, r *http.Request) {
		resp := api.CentralizedActionResponse{
			Success: true,
			Message: "Config synchronized",
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/v1/config/centralized/disable", func(w http.ResponseWriter, r *http.Request) {
		resp := api.CentralizedActionResponse{
			Success: true,
			Message: "Centralized config disabled",
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := api.NewClient(server.URL, "test-token", "scandrix_team_123")
	ctx := context.Background()

	// 1. Status
	status, err := client.GetCentralizedStatus(ctx)
	if err != nil {
		t.Fatalf("GetCentralizedStatus failed: %v", err)
	}
	if !status.Enabled || status.SelectedRepository.Namespace != "org/central-config" {
		t.Errorf("unexpected status: %+v", status)
	}

	// 2. Init
	initResp, err := client.InitCentralizedConfig(ctx, "org/central-config", "pr")
	if err != nil {
		t.Fatalf("InitCentralizedConfig failed: %v", err)
	}
	if !initResp.Success || initResp.PRURL == "" {
		t.Errorf("unexpected init response: %+v", initResp)
	}

	// 3. Sync
	syncResp, err := client.SyncCentralizedConfig(ctx)
	if err != nil {
		t.Fatalf("SyncCentralizedConfig failed: %v", err)
	}
	if !syncResp.Success {
		t.Errorf("unexpected sync response: %+v", syncResp)
	}

	// 4. Disable
	disableResp, err := client.DisableCentralizedConfig(ctx)
	if err != nil {
		t.Fatalf("DisableCentralizedConfig failed: %v", err)
	}
	if !disableResp.Success {
		t.Errorf("unexpected disable response: %+v", disableResp)
	}
}
