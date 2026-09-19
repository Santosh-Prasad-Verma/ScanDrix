package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGitLabAnalyticsAndAuditService(t *testing.T) {
	mux := http.NewServeMux()

	// 1. Audit Events
	mux.HandleFunc("/api/v4/projects/42/audit_events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			{
				"id": 1001,
				"author_id": 55,
				"entity_id": 42,
				"entity_type": "Project",
				"details": {"action": "project_visibility_changed"},
				"created_at": "2026-09-19T10:00:00Z"
			}
		]`))
	})

	// 2. Push Rules
	mux.HandleFunc("/api/v4/projects/42/push_rule", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": 1,
			"project_id": 42,
			"commit_message_regex": "^(feat|fix|chore):",
			"branch_name_regex": "^(feature|bugfix)/",
			"prevent_secrets": true,
			"reject_unsigned_commits": false
		}`))
	})

	// 3. Contributors
	mux.HandleFunc("/api/v4/projects/42/repository/contributors", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			{
				"name": "Drixy Engineer",
				"email": "engineer@scandrix.dev",
				"commits": 42,
				"additions": 1500,
				"deletions": 300
			}
		]`))
	})

	// 4. MR Metrics
	mux.HandleFunc("/api/v4/projects/42/merge_requests/7/time_tracking_stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"diff_size": 250,
			"modified_paths_count": 5,
			"commits_count": 3,
			"total_discussions_count": 4,
			"resolved_discussions_count": 4
		}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	svc := NewAnalyticsAndAuditService(client)
	ctx := context.Background()

	// Test GetProjectAuditEvents
	after := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	events, err := svc.GetProjectAuditEvents(ctx, 42, &after, 1, 10)
	if err != nil || len(events) != 1 || events[0].ID != 1001 {
		t.Fatalf("GetProjectAuditEvents failed: events=%+v, err=%v", events, err)
	}

	// Test GetPushRules
	rules, err := svc.GetPushRules(ctx, 42)
	if err != nil || rules == nil || rules.CommitMessageRegex != "^(feat|fix|chore):" || !rules.PreventSecrets {
		t.Fatalf("GetPushRules failed: rules=%+v, err=%v", rules, err)
	}

	// Test GetContributorsStats
	contributors, err := svc.GetContributorsStats(ctx, 42)
	if err != nil || len(contributors) != 1 || contributors[0].Commits != 42 {
		t.Fatalf("GetContributorsStats failed: contributors=%+v, err=%v", contributors, err)
	}

	// Test GetMergeRequestMetrics
	metrics, err := svc.GetMergeRequestMetrics(ctx, 42, 7)
	if err != nil || metrics == nil || metrics.TotalDiscussionsCount != 4 || metrics.DiffSize != 250 {
		t.Fatalf("GetMergeRequestMetrics failed: metrics=%+v, err=%v", metrics, err)
	}
}
