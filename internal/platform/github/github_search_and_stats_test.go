// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubSearchAndStatsService(t *testing.T) {
	mux := http.NewServeMux()

	// 1. Search Code
	mux.HandleFunc("/search/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"total_count": 1,
			"items": [
				{
					"name": "main.go",
					"path": "cmd/main.go",
					"sha": "abc1234",
					"html_url": "https://github.com/scandrix/backend/blob/main/cmd/main.go",
					"repository": {
						"id": 100,
						"full_name": "scandrix/backend",
						"private": false
					}
				}
			]
		}`))
	})

	// 2. Commit Activity
	mux.HandleFunc("/repos/scandrix/backend/stats/commit_activity", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			{
				"days": [0, 1, 2, 3, 4, 5, 0],
				"total": 15,
				"week": 1700000000
			}
		]`))
	})

	// 3. Code Frequency
	mux.HandleFunc("/repos/scandrix/backend/stats/code_frequency", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			[1700000000, 500, -120]
		]`))
	})

	// 4. Traffic Views
	mux.HandleFunc("/repos/scandrix/backend/traffic/views", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"count": 1200,
			"uniques": 350,
			"views": [
				{
					"timestamp": "2026-09-19T00:00:00Z",
					"count": 100,
					"uniques": 30
				}
			]
		}`))
	})

	// 5. Traffic Clones
	mux.HandleFunc("/repos/scandrix/backend/traffic/clones", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"count": 50,
			"uniques": 20,
			"clones": []
		}`))
	})

	// 6. Punch Card
	mux.HandleFunc("/repos/scandrix/backend/stats/punch_card", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			[1, 14, 25]
		]`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	svc := NewSearchAndStatsService(client)
	ctx := context.Background()

	// Test SearchCode
	searchRes, err := svc.SearchCode(ctx, "ScanDrix in:file", 1, 10)
	if err != nil || searchRes.TotalCount != 1 || searchRes.Items[0].Name != "main.go" {
		t.Fatalf("SearchCode failed: res=%+v, err=%v", searchRes, err)
	}

	// Test GetCommitActivity
	activity, err := svc.GetCommitActivity(ctx, "scandrix", "backend")
	if err != nil || len(activity) != 1 || activity[0].Total != 15 {
		t.Fatalf("GetCommitActivity failed: act=%+v, err=%v", activity, err)
	}

	// Test GetCodeFrequency
	freq, err := svc.GetCodeFrequency(ctx, "scandrix", "backend")
	if err != nil || len(freq) != 1 || freq[0].Additions != 500 || freq[0].Deletions != -120 {
		t.Fatalf("GetCodeFrequency failed: freq=%+v, err=%v", freq, err)
	}

	// Test GetTrafficViews
	views, err := svc.GetTrafficViews(ctx, "scandrix", "backend")
	if err != nil || views.Count != 1200 || views.Uniques != 350 {
		t.Fatalf("GetTrafficViews failed: views=%+v, err=%v", views, err)
	}

	// Test GetTrafficClones
	clones, err := svc.GetTrafficClones(ctx, "scandrix", "backend")
	if err != nil || clones.Count != 50 || clones.Uniques != 20 {
		t.Fatalf("GetTrafficClones failed: clones=%+v, err=%v", clones, err)
	}

	// Test GetPunchCard
	card, err := svc.GetPunchCard(ctx, "scandrix", "backend")
	if err != nil || len(card) != 1 || card[0].Commits != 25 {
		t.Fatalf("GetPunchCard failed: card=%+v, err=%v", card, err)
	}
}
