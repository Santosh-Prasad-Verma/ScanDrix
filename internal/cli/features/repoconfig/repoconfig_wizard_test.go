// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package repoconfig

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/scandrix/backend/internal/cli/services/api"
)

func TestConfigRepoSetupAction_YesFlag(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"repository_id": "repo-123",
				"namespace": "test-owner/test-repo",
				"review_enabled": true,
				"auto_approve_enabled": false,
				"request_changes_min_severity": "high",
				"ignored_file_patterns": ["*.lock"],
				"base_branch_patterns": ["main"],
				"ignored_title_patterns": ["wip*"]
			}`))
			return
		}
		if r.Method == http.MethodPut || r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"repository_id": "repo-123",
				"namespace": "test-owner/test-repo",
				"review_enabled": true,
				"auto_approve_enabled": true,
				"request_changes_min_severity": "critical"
			}`))
			return
		}
	}))
	defer ts.Close()

	client := api.NewClient(ts.URL, "test_token", "")
	ctx := context.Background()

	opts := ConfigRepoSetupOptions{
		Yes:  true,
		JSON: true,
	}

	err := ConfigRepoSetupAction(ctx, client, "test-owner/test-repo", opts)
	if err != nil {
		t.Fatalf("ConfigRepoSetupAction failed: %v", err)
	}
}
