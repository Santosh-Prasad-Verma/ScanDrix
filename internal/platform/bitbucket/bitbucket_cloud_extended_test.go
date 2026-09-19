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
	"time"
)

func TestBitbucketCloudExtended_CommitStatuses(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/repositories/myteam/myrepo/commit/sha123/statuses/build" && r.Method == http.MethodPost:
			var input CloudCommitStatusInput
			_ = json.NewDecoder(r.Body).Decode(&input)
			resp := map[string]any{
				"key":         input.Key,
				"state":       input.State,
				"name":        input.Name,
				"url":         input.URL,
				"description": input.Description,
				"created_on":  now.Format(time.RFC3339),
				"updated_on":  now.Format(time.RFC3339),
				"type":        "commit_status",
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repositories/myteam/myrepo/commit/sha123/statuses" && r.Method == http.MethodGet:
			resp := map[string]any{
				"values": []map[string]any{
					{
						"key":         "scandrix/ci",
						"state":       "SUCCESSFUL",
						"name":        "ScanDrix Review",
						"url":         "https://scandrix.dev",
						"description": "Code review passed",
						"created_on":  now.Format(time.RFC3339),
						"updated_on":  now.Format(time.RFC3339),
						"type":        "commit_status",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repositories/myteam/myrepo/commit/sha123/statuses/build/scandrix/ci" && r.Method == http.MethodGet:
			resp := map[string]any{
				"key":         "scandrix/ci",
				"state":       "SUCCESSFUL",
				"name":        "ScanDrix Review",
				"url":         "https://scandrix.dev",
				"description": "Code review passed",
				"created_on":  now.Format(time.RFC3339),
				"updated_on":  now.Format(time.RFC3339),
				"type":        "commit_status",
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := NewBitbucketAdvancedService(server.Client(), server.URL)
	ctx := context.Background()

	// 1. Create commit status
	status, err := svc.CreateCloudCommitStatus(ctx, "mock-token", "myteam", "myrepo", "sha123", CloudCommitStatusInput{
		Key:         "scandrix/ci",
		State:       CloudStatusSuccessful,
		Name:        "ScanDrix Review",
		URL:         "https://scandrix.dev",
		Description: "Code review passed",
	})
	if err != nil {
		t.Fatalf("CreateCloudCommitStatus error: %v", err)
	}
	if status.Key != "scandrix/ci" || status.State != CloudStatusSuccessful {
		t.Fatalf("unexpected status: %+v", status)
	}

	// 2. Get commit statuses
	statuses, err := svc.GetCloudCommitStatuses(ctx, "mock-token", "myteam", "myrepo", "sha123")
	if err != nil {
		t.Fatalf("GetCloudCommitStatuses error: %v", err)
	}
	if len(statuses) != 1 || statuses[0].Key != "scandrix/ci" {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}

	// 3. Get commit status by key
	single, err := svc.GetCloudCommitStatusByKey(ctx, "mock-token", "myteam", "myrepo", "sha123", "scandrix/ci")
	if err != nil {
		t.Fatalf("GetCloudCommitStatusByKey error: %v", err)
	}
	if single.Name != "ScanDrix Review" {
		t.Fatalf("unexpected single status: %+v", single)
	}
}

func TestBitbucketCloudExtended_PRCommitsDiffstatAndPatch(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repositories/myteam/myrepo/pullrequests/42/commits":
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"values": []map[string]any{
					{
						"hash":    "c0ffee1",
						"message": "feat: add secure auth checks",
						"date":    now.Format(time.RFC3339),
						"author": map[string]any{
							"raw": "Alice Engineer <alice@scandrix.dev>",
							"user": map[string]any{
								"display_name": "Alice Engineer",
								"uuid":         "{user-uuid-1}",
								"nickname":     "alice",
							},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repositories/myteam/myrepo/pullrequests/42/diffstat":
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"values": []map[string]any{
					{
						"status":        "modified",
						"lines_added":   15,
						"lines_removed": 3,
						"old":           map[string]any{"path": "auth/token.go"},
						"new":           map[string]any{"path": "auth/token.go"},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repositories/myteam/myrepo/pullrequests/42/patch":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("From c0ffee1 Mon Sep 17 00:00:00 2001\nSubject: [PATCH] feat: add secure auth checks\n"))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := NewBitbucketAdvancedService(server.Client(), server.URL)
	ctx := context.Background()

	// 1. Get PR commits
	commits, err := svc.GetCloudPRCommits(ctx, "mock-token", "myteam", "myrepo", 42)
	if err != nil {
		t.Fatalf("GetCloudPRCommits error: %v", err)
	}
	if len(commits) != 1 || commits[0].Hash != "c0ffee1" {
		t.Fatalf("unexpected commits: %+v", commits)
	}

	// 2. Get PR diffstat
	diffstat, err := svc.GetCloudPRDiffstat(ctx, "mock-token", "myteam", "myrepo", 42)
	if err != nil {
		t.Fatalf("GetCloudPRDiffstat error: %v", err)
	}
	if len(diffstat) != 1 || diffstat[0].NewPath != "auth/token.go" || diffstat[0].LinesAdded != 15 {
		t.Fatalf("unexpected diffstat: %+v", diffstat)
	}

	// 3. Get PR patch
	patch, err := svc.GetCloudPRPatch(ctx, "mock-token", "myteam", "myrepo", 42)
	if err != nil {
		t.Fatalf("GetCloudPRPatch error: %v", err)
	}
	if len(patch) == 0 {
		t.Fatalf("empty patch returned")
	}
}

func TestBitbucketCloudExtended_EnvironmentsAndMerge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/repositories/myteam/myrepo/environments" && r.Method == http.MethodGet:
			resp := map[string]any{
				"values": []map[string]any{
					{
						"uuid":             "{env-uuid-1}",
						"name":             "Production",
						"environment_type": "Production",
						"rank":             1,
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repositories/myteam/myrepo/environments" && r.Method == http.MethodPost:
			var input CloudRepoEnvironmentInput
			_ = json.NewDecoder(r.Body).Decode(&input)
			resp := map[string]any{
				"uuid":             "{env-uuid-2}",
				"name":             input.Name,
				"environment_type": input.EnvironmentType,
				"rank":             input.Rank,
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repositories/myteam/myrepo/environments/{env-uuid-1}" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)

		case r.URL.Path == "/repositories/myteam/myrepo/pullrequests/42" && r.Method == http.MethodPut:
			w.WriteHeader(http.StatusOK)

		case r.URL.Path == "/repositories/myteam/myrepo/pullrequests/42" && r.Method == http.MethodGet:
			resp := map[string]any{
				"state":   "OPEN",
				"summary": "Everything is ready to merge",
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repositories/myteam/myrepo/pullrequests/42/merge" && r.Method == http.MethodPost:
			resp := map[string]any{
				"state":               "MERGED",
				"close_source_branch": true,
				"merge_commit": map[string]any{
					"hash": "merge1234",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := NewBitbucketAdvancedService(server.Client(), server.URL)
	ctx := context.Background()

	// 1. List environments
	envs, err := svc.ListCloudEnvironments(ctx, "mock-token", "myteam", "myrepo")
	if err != nil {
		t.Fatalf("ListCloudEnvironments error: %v", err)
	}
	if len(envs) != 1 || envs[0].Name != "Production" {
		t.Fatalf("unexpected environments: %+v", envs)
	}

	// 2. Create environment
	createdEnv, err := svc.CreateCloudEnvironment(ctx, "mock-token", "myteam", "myrepo", CloudRepoEnvironmentInput{
		Name:            "Staging",
		EnvironmentType: "Staging",
		Rank:            2,
	})
	if err != nil {
		t.Fatalf("CreateCloudEnvironment error: %v", err)
	}
	if createdEnv.Name != "Staging" {
		t.Fatalf("unexpected created env: %+v", createdEnv)
	}

	// 3. Delete environment
	if err := svc.DeleteCloudEnvironment(ctx, "mock-token", "myteam", "myrepo", "{env-uuid-1}"); err != nil {
		t.Fatalf("DeleteCloudEnvironment error: %v", err)
	}

	// 4. Request reviewers
	if err := svc.RequestCloudReviewers(ctx, "mock-token", "myteam", "myrepo", 42, []string{"{user-uuid-1}"}); err != nil {
		t.Fatalf("RequestCloudReviewers error: %v", err)
	}

	// 5. Check mergeability
	mergeability, err := svc.CheckCloudMergeability(ctx, "mock-token", "myteam", "myrepo", 42)
	if err != nil {
		t.Fatalf("CheckCloudMergeability error: %v", err)
	}
	if !mergeability.CanMerge {
		t.Fatalf("expected can merge to be true")
	}

	// 6. Merge PR
	merged, err := svc.MergeCloudPullRequest(ctx, "mock-token", "myteam", "myrepo", 42, CloudMergeRequest{
		Message:           "Merged by ScanDrix",
		CloseSourceBranch: true,
		MergeStrategy:     "squash",
	})
	if err != nil {
		t.Fatalf("MergeCloudPullRequest error: %v", err)
	}
	if merged.State != "MERGED" || merged.Hash != "merge1234" {
		t.Fatalf("unexpected merged response: %+v", merged)
	}
}
