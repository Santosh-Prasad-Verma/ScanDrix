// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package bitbucket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBitbucketDataCenterExtended_BuildStatus(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/rest/build-status/1.0/commits/a1b2c3d4", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-bb-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.Method == http.MethodPost {
			var req BuildStatusRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req.State != BuildStatusSuccessful || req.Key != "scandrix-review" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if r.Method == http.MethodGet {
			resp := map[string]any{
				"values": []BuildStatusResponse{
					{
						State:       BuildStatusSuccessful,
						Key:         "scandrix-review",
						Name:        "ScanDrix Review",
						URL:         "https://scandrix.dev/review/999",
						Description: "Passed 10/10 checks",
						DateAdded:   1700000000,
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewBitbucketDataCenterExtended(server.URL, server.Client())
	ctx := context.Background()

	// PostCommitBuildStatus
	err := client.PostCommitBuildStatus(ctx, "test-bb-token", "a1b2c3d4", BuildStatusRequest{
		State:       BuildStatusSuccessful,
		Key:         "scandrix-review",
		Name:        "ScanDrix Review",
		URL:         "https://scandrix.dev/review/999",
		Description: "Passed 10/10 checks",
	})
	if err != nil {
		t.Fatalf("PostCommitBuildStatus failed: %v", err)
	}

	// GetCommitBuildStatuses
	statuses, err := client.GetCommitBuildStatuses(ctx, "test-bb-token", "a1b2c3d4")
	if err != nil {
		t.Fatalf("GetCommitBuildStatuses failed: %v", err)
	}
	if len(statuses) != 1 || statuses[0].Key != "scandrix-review" {
		t.Errorf("Unexpected statuses: %+v", statuses)
	}
}

func TestBitbucketDataCenterExtended_BranchRestrictions(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/rest/branch-permissions/2.0/projects/PRJ/repos/repo-1/restrictions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			resp := map[string]any{
				"values": []ServerBranchRestriction{
					{
						ID:   101,
						Type: "pull-request-only",
						Users: []string{"admin"},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}

		if r.Method == http.MethodPost {
			var rstr ServerBranchRestriction
			json.NewDecoder(r.Body).Decode(&rstr)
			rstr.ID = 102
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rstr)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/rest/branch-permissions/2.0/projects/PRJ/repos/repo-1/restrictions/102", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewBitbucketDataCenterExtended(server.URL, server.Client())
	ctx := context.Background()

	// List
	list, err := client.ListBranchRestrictions(ctx, "tok", "PRJ", "repo-1")
	if err != nil {
		t.Fatalf("ListBranchRestrictions failed: %v", err)
	}
	if len(list) != 1 || list[0].ID != 101 {
		t.Errorf("Unexpected list: %+v", list)
	}

	// Create
	created, err := client.CreateBranchRestriction(ctx, "tok", "PRJ", "repo-1", ServerBranchRestriction{
		Type:  "fast-forward-only",
		Users: []string{"dev1"},
	})
	if err != nil {
		t.Fatalf("CreateBranchRestriction failed: %v", err)
	}
	if created.ID != 102 {
		t.Errorf("Unexpected created ID: %d", created.ID)
	}

	// Delete
	err = client.DeleteBranchRestriction(ctx, "tok", "PRJ", "repo-1", 102)
	if err != nil {
		t.Fatalf("DeleteBranchRestriction failed: %v", err)
	}
}

func TestBitbucketDataCenterExtended_ParticipantsAndMerge(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/rest/api/1.0/projects/PRJ/repos/repo-1/pull-requests/55/participants", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"values": []map[string]any{
				{
					"user": map[string]any{
						"name": "reviewer1",
						"id":   12,
					},
					"role":     "REVIEWER",
					"approved": true,
					"status":   "APPROVED",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/rest/api/1.0/projects/PRJ/repos/repo-1/pull-requests/55/merge", func(w http.ResponseWriter, r *http.Request) {
		resp := MergeCheckResponse{
			CanMerge:   true,
			Conflicted: false,
			Outcome:    "CLEAN",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewBitbucketDataCenterExtended(server.URL, server.Client())
	ctx := context.Background()

	// Participants
	parts, err := client.GetPullRequestParticipants(ctx, "tok", "PRJ", "repo-1", 55)
	if err != nil {
		t.Fatalf("GetPullRequestParticipants failed: %v", err)
	}
	if len(parts) != 1 || parts[0].Role != "REVIEWER" || !parts[0].Approved {
		t.Errorf("Unexpected participants: %+v", parts)
	}

	// Mergeability
	mergeCheck, err := client.CheckPullRequestMergeability(ctx, "tok", "PRJ", "repo-1", 55)
	if err != nil {
		t.Fatalf("CheckPullRequestMergeability failed: %v", err)
	}
	if !mergeCheck.CanMerge || mergeCheck.Conflicted {
		t.Errorf("Unexpected merge check: %+v", mergeCheck)
	}
}
