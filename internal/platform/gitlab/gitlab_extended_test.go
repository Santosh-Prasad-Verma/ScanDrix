package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGitLabExtended_HooksAndApprovals(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/projects/123/hooks" && r.Method == http.MethodGet:
			resp := []map[string]any{
				{
					"id":                      1,
					"url":                     "https://scandrix.dev/webhooks/gitlab",
					"created_at":              now.Format(time.RFC3339),
					"push_events":             true,
					"merge_requests_events":   true,
					"enable_ssl_verification": true,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/projects/123/hooks/1" && r.Method == http.MethodGet:
			resp := map[string]any{
				"id":                      1,
				"url":                     "https://scandrix.dev/webhooks/gitlab",
				"created_at":              now.Format(time.RFC3339),
				"push_events":             true,
				"merge_requests_events":   true,
				"enable_ssl_verification": true,
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/projects/123/hooks" && r.Method == http.MethodPost:
			var req AddProjectHookRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			resp := map[string]any{
				"id":                      2,
				"url":                     req.URL,
				"created_at":              now.Format(time.RFC3339),
				"push_events":             req.PushEvents,
				"merge_requests_events":   req.MergeRequestsEvents,
				"enable_ssl_verification": req.EnableSSLVerification,
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/projects/123/hooks/1" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)

		case r.URL.Path == "/projects/123/merge_requests/42/approval_rules" && r.Method == http.MethodGet:
			resp := []map[string]any{
				{
					"id":                 101,
					"name":               "Security Leads",
					"rule_type":          "regular",
					"approvals_required": 1,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/projects/123/merge_requests/42/approval_rules" && r.Method == http.MethodPost:
			var req CreateMRApprovalRuleRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			resp := map[string]any{
				"id":                 102,
				"name":               req.Name,
				"rule_type":          "regular",
				"approvals_required": req.ApprovalsRequired,
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/projects/123/merge_requests/42/approval_rules/101" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := NewGitLabAdvancedService(nil, server.Client(), server.URL)
	ctx := context.Background()

	// 1. List project hooks
	hooks, err := svc.ListProjectHooks(ctx, "glpat-token", 123)
	if err != nil {
		t.Fatalf("ListProjectHooks error: %v", err)
	}
	if len(hooks) != 1 || hooks[0].URL != "https://scandrix.dev/webhooks/gitlab" {
		t.Fatalf("unexpected hooks: %+v", hooks)
	}

	// 2. Get hook
	hook, err := svc.GetProjectHook(ctx, "glpat-token", 123, 1)
	if err != nil {
		t.Fatalf("GetProjectHook error: %v", err)
	}
	if hook.ID != 1 {
		t.Fatalf("expected hook ID 1, got %d", hook.ID)
	}

	// 3. Add hook
	added, err := svc.AddProjectHook(ctx, "glpat-token", 123, AddProjectHookRequest{
		URL:                 "https://scandrix.dev/webhook-new",
		PushEvents:          true,
		MergeRequestsEvents: true,
	})
	if err != nil {
		t.Fatalf("AddProjectHook error: %v", err)
	}
	if added.ID != 2 {
		t.Fatalf("expected added ID 2, got %d", added.ID)
	}

	// 4. Delete hook
	if err := svc.DeleteProjectHook(ctx, "glpat-token", 123, 1); err != nil {
		t.Fatalf("DeleteProjectHook error: %v", err)
	}

	// 5. List MR approval rules
	rules, err := svc.ListMRApprovalRules(ctx, "glpat-token", 123, 42)
	if err != nil {
		t.Fatalf("ListMRApprovalRules error: %v", err)
	}
	if len(rules) != 1 || rules[0].Name != "Security Leads" {
		t.Fatalf("unexpected approval rules: %+v", rules)
	}

	// 6. Create MR approval rule
	newRule, err := svc.CreateMRApprovalRule(ctx, "glpat-token", 123, 42, CreateMRApprovalRuleRequest{
		Name:              "Architects",
		ApprovalsRequired: 2,
	})
	if err != nil {
		t.Fatalf("CreateMRApprovalRule error: %v", err)
	}
	if newRule.ID != 102 || newRule.ApprovalsRequired != 2 {
		t.Fatalf("unexpected created rule: %+v", newRule)
	}

	// 7. Delete MR approval rule
	if err := svc.DeleteMRApprovalRule(ctx, "glpat-token", 123, 42, 101); err != nil {
		t.Fatalf("DeleteMRApprovalRule error: %v", err)
	}
}

func TestGitLabExtended_TagsReleasesAndDeployments(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/projects/123/protected_tags" && r.Method == http.MethodGet:
			resp := []map[string]any{
				{
					"name": "v*",
					"create_access_levels": []map[string]any{
						{"access_level": 40, "access_level_description": "Maintainers"},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/projects/123/protected_tags" && r.Method == http.MethodPost:
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			resp := map[string]any{
				"name": req["name"],
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/projects/123/protected_tags/v*" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)

		case r.URL.Path == "/projects/123/releases" && r.Method == http.MethodGet:
			resp := []map[string]any{
				{
					"tag_name":    "v1.0.0",
					"name":        "Release 1.0.0",
					"description": "Initial enterprise release",
					"created_at":  now.Format(time.RFC3339),
					"released_at": now.Format(time.RFC3339),
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/projects/123/releases" && r.Method == http.MethodPost:
			var req CreateReleaseRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			resp := map[string]any{
				"tag_name":    req.TagName,
				"name":        req.Name,
				"description": req.Description,
				"created_at":  now.Format(time.RFC3339),
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/projects/123/merge_requests/42/merge" && r.Method == http.MethodPut:
			resp := map[string]any{
				"id":               42,
				"iid":              42,
				"state":            "merged",
				"merge_commit_sha": "sha-merge-999",
				"source_branch":    "feature/auth",
				"target_branch":    "main",
				"merged_at":        now.Format(time.RFC3339),
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/projects/123/deployments" && r.Method == http.MethodGet:
			resp := []map[string]any{
				{
					"id":     501,
					"iid":    1,
					"ref":    "main",
					"sha":    "sha-deploy-1",
					"status": "success",
					"environment": map[string]any{
						"id":   10,
						"name": "production",
					},
					"created_at": now.Format(time.RFC3339),
					"updated_at": now.Format(time.RFC3339),
				},
			}
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	svc := NewGitLabAdvancedService(nil, server.Client(), server.URL)
	ctx := context.Background()

	// 1. Protected tags
	tags, err := svc.ListProtectedTags(ctx, "glpat-token", 123)
	if err != nil {
		t.Fatalf("ListProtectedTags error: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "v*" {
		t.Fatalf("unexpected protected tags: %+v", tags)
	}

	pt, err := svc.ProtectTag(ctx, "glpat-token", 123, "release-*", 40)
	if err != nil {
		t.Fatalf("ProtectTag error: %v", err)
	}
	if pt.Name != "release-*" {
		t.Fatalf("expected protected tag release-*, got %s", pt.Name)
	}

	if err := svc.UnprotectTag(ctx, "glpat-token", 123, "v*"); err != nil {
		t.Fatalf("UnprotectTag error: %v", err)
	}

	// 2. Releases
	releases, err := svc.ListReleases(ctx, "glpat-token", 123)
	if err != nil {
		t.Fatalf("ListReleases error: %v", err)
	}
	if len(releases) != 1 || releases[0].TagName != "v1.0.0" {
		t.Fatalf("unexpected releases: %+v", releases)
	}

	createdRel, err := svc.CreateRelease(ctx, "glpat-token", 123, CreateReleaseRequest{
		TagName:     "v1.1.0",
		Name:        "Release 1.1.0",
		Description: "Security updates",
	})
	if err != nil {
		t.Fatalf("CreateRelease error: %v", err)
	}
	if createdRel.TagName != "v1.1.0" {
		t.Fatalf("unexpected created release: %+v", createdRel)
	}

	// 3. Accept Merge Request
	merged, err := svc.AcceptMergeRequest(ctx, "glpat-token", 123, 42, AcceptMergeRequestRequest{
		MergeCommitMessage: "Merge feature/auth",
		Squash:             true,
	})
	if err != nil {
		t.Fatalf("AcceptMergeRequest error: %v", err)
	}
	if merged.State != "merged" || merged.MergeCommitSHA != "sha-merge-999" {
		t.Fatalf("unexpected merged record: %+v", merged)
	}

	// 4. List deployments
	deployments, err := svc.ListDeployments(ctx, "glpat-token", 123, "production")
	if err != nil {
		t.Fatalf("ListDeployments error: %v", err)
	}
	if len(deployments) != 1 || deployments[0].Environment.Name != "production" {
		t.Fatalf("unexpected deployments: %+v", deployments)
	}
}
