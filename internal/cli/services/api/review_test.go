// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/pkg/models"
)

func TestSubmitReviewSync(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/reviews" && r.Method == http.MethodPost {
			if r.Header.Get("X-ScanDrix-Async") != "1" {
				t.Errorf("expected X-ScanDrix-Async header")
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(api.ReviewResponse{
				ReviewID:      "rev-123",
				Status:        "COMPLETED",
				Summary:       "All clean",
				FilesAnalyzed: 2,
				Findings:      []models.CodeFinding{},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	client := api.NewClient(ts.URL, "token-abc", "")
	resp, err := client.SubmitReview(context.Background(), api.ReviewRequest{
		Diff: "diff --git a/test.go b/test.go\n",
	})
	if err != nil {
		t.Fatalf("SubmitReview failed: %v", err)
	}
	if resp.ReviewID != "rev-123" || resp.Status != "COMPLETED" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestSubmitReviewAsyncPolling(t *testing.T) {
	pollCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// 1. Enqueue returns 202
		if r.URL.Path == "/api/v1/reviews" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(api.EnqueueJobResponse{
				JobID:  "job-999",
				Status: "PENDING",
			})
			return
		}

		// 2. Poll job status
		if strings.HasPrefix(r.URL.Path, "/api/v1/reviews/jobs/") {
			pollCount++
			if pollCount == 1 {
				_ = json.NewEncoder(w).Encode(api.JobStatusResponse{
					JobID:  "job-999",
					Status: "PROCESSING",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(api.JobStatusResponse{
				JobID:  "job-999",
				Status: "COMPLETED",
				Result: &api.ReviewResponse{
					ReviewID:      "rev-async-999",
					Status:        "COMPLETED",
					Summary:       "Async job finished",
					FilesAnalyzed: 3,
				},
			})
			return
		}

		http.NotFound(w, r)
	}))
	defer ts.Close()

	var statuses []string
	client := api.NewClient(ts.URL, "token-abc", "")
	resp, err := client.SubmitReviewWithProgress(context.Background(), api.ReviewRequest{
		Diff: "diff --git a/file.go b/file.go\n",
	}, func(s string) {
		statuses = append(statuses, s)
	})

	if err != nil {
		t.Fatalf("SubmitReviewWithProgress failed: %v", err)
	}
	if resp.ReviewID != "rev-async-999" {
		t.Errorf("unexpected ReviewID: %s", resp.ReviewID)
	}
	if len(statuses) == 0 {
		t.Errorf("expected progress status callbacks")
	}
}
