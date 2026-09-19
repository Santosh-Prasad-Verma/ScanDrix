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

func TestBitbucketAdvancedService_ActivitiesAndTasks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/repositories/myworkspace/myrepo/pullrequests/7/activity":
			resp := map[string]any{
				"values": []map[string]any{
					{
						"approval": map[string]any{
							"user": map[string]any{"display_name": "Bob Senior"},
							"date": time.Now().Format(time.RFC3339),
						},
					},
					{
						"comment": map[string]any{
							"id":         float64(901),
							"user":       map[string]any{"display_name": "Drixy AI"},
							"content":    map[string]any{"raw": "Consider reducing memory allocations."},
							"created_on": time.Now().Format(time.RFC3339),
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repositories/myworkspace/myrepo/pullrequests/7/comments/901/tasks" && r.Method == http.MethodPost:
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			raw := req["content"].(map[string]any)["raw"].(string)
			resp := map[string]any{
				"id":         float64(55),
				"state":      "UNRESOLVED",
				"content":    map[string]any{"raw": raw},
				"created_on": time.Now().Format(time.RFC3339),
				"creator":    map[string]any{"display_name": "Drixy AI"},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/repositories/myworkspace/myrepo/branch-restrictions":
			val := 2
			resp := map[string]any{
				"values": []map[string]any{
					{
						"id":    float64(12),
						"kind":  "require_approvals_to_merge",
						"value": val,
						"users": []map[string]any{
							{"display_name": "Tech Leads"},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := NewBitbucketAdvancedService(server.Client(), server.URL)
	ctx := context.Background()

	// 1. Get activities
	activities, err := svc.GetPullRequestActivities(ctx, "mock-token", "myworkspace", "myrepo", 7)
	if err != nil {
		t.Fatalf("GetPullRequestActivities failed: %v", err)
	}
	if len(activities) != 2 || activities[0].Action != "approval" || activities[1].Action != "comment" {
		t.Errorf("unexpected activities: %+v", activities)
	}

	// 2. Create comment task
	task, err := svc.CreateCommentTask(ctx, "mock-token", "myworkspace", "myrepo", 7, 901, "Benchmark memory usage")
	if err != nil {
		t.Fatalf("CreateCommentTask failed: %v", err)
	}
	if task.ID != 55 || task.Content != "Benchmark memory usage" {
		t.Errorf("unexpected task: %+v", task)
	}

	// 3. Get branch restrictions
	restrictions, err := svc.GetBranchRestrictions(ctx, "mock-token", "myworkspace", "myrepo")
	if err != nil {
		t.Fatalf("GetBranchRestrictions failed: %v", err)
	}
	if len(restrictions) != 1 || restrictions[0].Kind != "require_approvals_to_merge" {
		t.Errorf("unexpected restrictions: %+v", restrictions)
	}
}
