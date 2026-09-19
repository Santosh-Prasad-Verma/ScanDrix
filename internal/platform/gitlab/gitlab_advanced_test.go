package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGitLabAdvancedService_DiscussionsAndVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/projects/123/merge_requests/5/discussions" && r.Method == http.MethodGet:
			resp := []map[string]any{
				{
					"id":              "disc-1",
					"individual_note": false,
					"notes": []map[string]any{
						{
							"id":         501,
							"type":       "DiffNote",
							"body":       "Please ensure thread safety here.",
							"created_at": time.Now().Format(time.RFC3339),
							"resolvable": true,
							"resolved":   false,
							"author":     map[string]any{"username": "drixy-bot"},
							"position": map[string]any{
								"base_sha":      "sha_base",
								"start_sha":     "sha_start",
								"head_sha":      "sha_head",
								"new_path":      "service.go",
								"new_line":      25,
								"position_type": "text",
							},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/projects/123/merge_requests/5/discussions" && r.Method == http.MethodPost:
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			resp := map[string]any{
				"id": "disc-2",
				"notes": []map[string]any{
					{
						"id":         502,
						"type":       "DiffNote",
						"body":       req["body"],
						"created_at": time.Now().Format(time.RFC3339),
						"author":     map[string]any{"username": "drixy-bot"},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/projects/123/merge_requests/5/discussions/disc-1" && r.Method == http.MethodPut:
			w.WriteHeader(http.StatusOK)

		case r.URL.Path == "/projects/123/merge_requests/5/versions":
			resp := []map[string]any{
				{
					"id":               1,
					"head_commit_sha":  "sha_head",
					"base_commit_sha":  "sha_base",
					"start_commit_sha": "sha_start",
					"created_at":       time.Now().Format(time.RFC3339),
					"state":            "collected",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/projects/123/merge_requests/5/approvals":
			resp := map[string]any{
				"id":                 123,
				"iid":                5,
				"approvals_required": 2,
				"approvals_left":     1,
				"approved_by": []map[string]any{
					{"user": map[string]any{"username": "lead-dev"}},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/projects/123/protected_branches":
			resp := []map[string]any{
				{
					"name":                         "main",
					"allow_force_push":             false,
					"code_owner_approval_required": true,
					"push_access_levels":           []map[string]any{{"access_level": 40}},
					"merge_access_levels":          []map[string]any{{"access_level": 30}},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := NewGitLabAdvancedService(nil, server.Client(), server.URL)
	ctx := context.Background()

	// 1. Get discussions
	discussions, err := svc.GetMergeRequestDiscussions(ctx, "mock-token", 123, 5)
	if err != nil {
		t.Fatalf("GetMergeRequestDiscussions failed: %v", err)
	}
	if len(discussions) != 1 || discussions[0].ID != "disc-1" {
		t.Errorf("unexpected discussions: %+v", discussions)
	}

	// 2. Create diff discussion
	pos := NotePosition{
		BaseSHA:      "sha_base",
		StartSHA:     "sha_start",
		HeadSHA:      "sha_head",
		NewPath:      "service.go",
		NewLine:      25,
		PositionType: "text",
	}
	newDisc, err := svc.CreateDiffDiscussion(ctx, "mock-token", 123, 5, "Check concurrency", pos)
	if err != nil {
		t.Fatalf("CreateDiffDiscussion failed: %v", err)
	}
	if newDisc.ID != "disc-2" {
		t.Errorf("unexpected new discussion: %+v", newDisc)
	}

	// 3. Resolve discussion
	err = svc.ResolveDiscussion(ctx, "mock-token", 123, 5, "disc-1", true)
	if err != nil {
		t.Fatalf("ResolveDiscussion failed: %v", err)
	}

	// 4. Get diff versions
	versions, err := svc.GetMergeRequestDiffVersions(ctx, "mock-token", 123, 5)
	if err != nil {
		t.Fatalf("GetMergeRequestDiffVersions failed: %v", err)
	}
	if len(versions) != 1 || versions[0].HeadCommitSHA != "sha_head" {
		t.Errorf("unexpected versions: %+v", versions)
	}

	// 5. Get approvals
	approvals, err := svc.GetMergeRequestApprovals(ctx, "mock-token", 123, 5)
	if err != nil {
		t.Fatalf("GetMergeRequestApprovals failed: %v", err)
	}
	if approvals.ApprovalsRequired != 2 || len(approvals.ApprovedBy) != 1 || approvals.ApprovedBy[0] != "lead-dev" {
		t.Errorf("unexpected approvals: %+v", approvals)
	}

	// 6. Get protected branches
	rules, err := svc.GetProtectedBranches(ctx, "mock-token", 123)
	if err != nil {
		t.Fatalf("GetProtectedBranches failed: %v", err)
	}
	if len(rules) != 1 || rules[0].Name != "main" || rules[0].PushAccessLevel != 40 {
		t.Errorf("unexpected protected branches: %+v", rules)
	}
}
