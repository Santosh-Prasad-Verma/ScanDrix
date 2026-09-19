package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGitLabPipelinesService_CommitStatuses(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v4/projects/test-group%2Ftest-repo/statuses/c0ffee123", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("PRIVATE-TOKEN") != "glpat-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req CommitStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.State != CommitStatusSuccess {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		resp := CommitStatusResponse{
			ID:          42,
			SHA:         "c0ffee123",
			Ref:         "main",
			Status:      CommitStatusSuccess,
			Name:        req.Name,
			TargetURL:   req.TargetURL,
			Description: req.Description,
			CreatedAt:   time.Now(),
			Author: GitLabUser{
				ID:       1,
				Username: "scandrix-bot",
				Name:     "ScanDrix Bot",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/v4/projects/test-group%2Ftest-repo/repository/commits/c0ffee123/statuses", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		statuses := []CommitStatusResponse{
			{
				ID:          42,
				SHA:         "c0ffee123",
				Ref:         "main",
				Status:      CommitStatusSuccess,
				Name:        "scandrix/review",
				TargetURL:   "https://scandrix.dev/review/123",
				Description: "ScanDrix code review passed",
				CreatedAt:   time.Now(),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(statuses)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	svc := NewGitLabPipelinesService("glpat-secret", server.Client(), server.URL+"/api/v4")
	ctx := context.Background()

	// Test CreateCommitStatus
	statusResp, err := svc.CreateCommitStatus(ctx, "test-group/test-repo", "c0ffee123", CommitStatusRequest{
		State:       CommitStatusSuccess,
		Name:        "scandrix/review",
		TargetURL:   "https://scandrix.dev/review/123",
		Description: "ScanDrix code review passed",
	})
	if err != nil {
		t.Fatalf("CreateCommitStatus failed: %v", err)
	}
	if statusResp.ID != 42 || statusResp.Status != CommitStatusSuccess {
		t.Errorf("Unexpected status response: %+v", statusResp)
	}

	// Test ListCommitStatuses
	listResp, err := svc.ListCommitStatuses(ctx, "test-group/test-repo", "c0ffee123")
	if err != nil {
		t.Fatalf("ListCommitStatuses failed: %v", err)
	}
	if len(listResp) != 1 || listResp[0].Name != "scandrix/review" {
		t.Errorf("Unexpected list response: %+v", listResp)
	}
}

func TestGitLabPipelinesService_PipelineLifecycle(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v4/projects/42/pipeline", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)

		p := PipelineInfo{
			ID:        1001,
			IID:       12,
			ProjectID: 42,
			SHA:       "feedbeef",
			Ref:       payload["ref"].(string),
			Status:    PipelineStatusPending,
			CreatedAt: time.Now(),
			WebURL:    "https://gitlab.com/test-group/test-repo/-/pipelines/1001",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	})

	mux.HandleFunc("/api/v4/projects/42/pipelines/1001", func(w http.ResponseWriter, r *http.Request) {
		p := PipelineInfo{
			ID:        1001,
			IID:       12,
			ProjectID: 42,
			SHA:       "feedbeef",
			Ref:       "main",
			Status:    PipelineStatusRunning,
			Duration:  125,
			Coverage:  "87.5%",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	})

	mux.HandleFunc("/api/v4/projects/42/pipelines/1001/jobs", func(w http.ResponseWriter, r *http.Request) {
		jobs := []PipelineJob{
			{
				ID:       501,
				Name:     "lint",
				Stage:    "test",
				Status:   PipelineStatusSuccess,
				Duration: 45.2,
			},
			{
				ID:       502,
				Name:     "unit-tests",
				Stage:    "test",
				Status:   PipelineStatusRunning,
				Duration: 80.0,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jobs)
	})

	mux.HandleFunc("/api/v4/projects/42/pipelines/1001/retry", func(w http.ResponseWriter, r *http.Request) {
		p := PipelineInfo{
			ID:     1001,
			Status: PipelineStatusPending,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	})

	mux.HandleFunc("/api/v4/projects/42/pipelines/1001/cancel", func(w http.ResponseWriter, r *http.Request) {
		p := PipelineInfo{
			ID:     1001,
			Status: PipelineStatusCanceled,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	})

	mux.HandleFunc("/api/v4/projects/42/pipelines/1001/bridges", func(w http.ResponseWriter, r *http.Request) {
		bridges := []PipelineBridge{
			{
				ID:     77,
				Name:   "downstream-deploy",
				Stage:  "deploy",
				Status: PipelineStatusRunning,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(bridges)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	svc := NewGitLabPipelinesService("glpat-secret", server.Client(), server.URL+"/api/v4")
	ctx := context.Background()

	// TriggerPipeline
	p, err := svc.TriggerPipeline(ctx, "42", "main", map[string]string{"ENV": "staging"})
	if err != nil {
		t.Fatalf("TriggerPipeline failed: %v", err)
	}
	if p.ID != 1001 || p.Status != PipelineStatusPending {
		t.Errorf("Unexpected pipeline: %+v", p)
	}

	// GetPipeline
	gotP, err := svc.GetPipeline(ctx, "42", 1001)
	if err != nil {
		t.Fatalf("GetPipeline failed: %v", err)
	}
	if gotP.Status != PipelineStatusRunning || gotP.Duration != 125 {
		t.Errorf("Unexpected got pipeline: %+v", gotP)
	}

	// ListPipelineJobs
	jobs, err := svc.ListPipelineJobs(ctx, "42", 1001, "success")
	if err != nil {
		t.Fatalf("ListPipelineJobs failed: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("Expected 2 jobs, got %d", len(jobs))
	}

	// RetryPipeline
	retryP, err := svc.RetryPipeline(ctx, "42", 1001)
	if err != nil {
		t.Fatalf("RetryPipeline failed: %v", err)
	}
	if retryP.Status != PipelineStatusPending {
		t.Errorf("Expected pending status, got %s", retryP.Status)
	}

	// CancelPipeline
	cancelP, err := svc.CancelPipeline(ctx, "42", 1001)
	if err != nil {
		t.Fatalf("CancelPipeline failed: %v", err)
	}
	if cancelP.Status != PipelineStatusCanceled {
		t.Errorf("Expected canceled status, got %s", cancelP.Status)
	}

	// ListPipelineBridges
	bridges, err := svc.ListPipelineBridges(ctx, "42", 1001)
	if err != nil {
		t.Fatalf("ListPipelineBridges failed: %v", err)
	}
	if len(bridges) != 1 || bridges[0].Name != "downstream-deploy" {
		t.Errorf("Unexpected bridges: %+v", bridges)
	}
}

func TestGitLabPipelinesService_ApprovalRules(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v4/projects/99/merge_requests/5/approval_rules", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			rules := []ApprovalRule{
				{
					ID:                1,
					Name:              "Security Team",
					RuleType:          "regular",
					ApprovalsRequired: 2,
					Users: []GitLabUser{
						{ID: 10, Username: "alice"},
						{ID: 20, Username: "bob"},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rules)
			return
		}

		if r.Method == http.MethodPost {
			var payload map[string]any
			json.NewDecoder(r.Body).Decode(&payload)
			rule := ApprovalRule{
				ID:                2,
				Name:              payload["name"].(string),
				RuleType:          "regular",
				ApprovalsRequired: int(payload["approvals_required"].(float64)),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rule)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/v4/projects/99/merge_requests/5/approval_rules/2", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	svc := NewGitLabPipelinesService("glpat-secret", server.Client(), server.URL+"/api/v4")
	ctx := context.Background()

	// List approval rules
	rules, err := svc.ListMergeRequestApprovalRules(ctx, "99", 5)
	if err != nil {
		t.Fatalf("ListMergeRequestApprovalRules failed: %v", err)
	}
	if len(rules) != 1 || rules[0].Name != "Security Team" {
		t.Errorf("Unexpected rules: %+v", rules)
	}

	// Create approval rule
	newRule, err := svc.CreateMergeRequestApprovalRule(ctx, "99", 5, "QA Engineers", 1, []int{30})
	if err != nil {
		t.Fatalf("CreateMergeRequestApprovalRule failed: %v", err)
	}
	if newRule.ID != 2 || newRule.Name != "QA Engineers" {
		t.Errorf("Unexpected new rule: %+v", newRule)
	}

	// Delete approval rule
	if err := svc.DeleteMergeRequestApprovalRule(ctx, "99", 5, 2); err != nil {
		t.Fatalf("DeleteMergeRequestApprovalRule failed: %v", err)
	}
}

func TestGitLabPipelines_Utilities(t *testing.T) {
	tests := []struct {
		seconds int
		want    string
	}{
		{45, "45s"},
		{60, "1m 0s"},
		{125, "2m 5s"},
		{3600, "1h 0m 0s"},
		{3665, "1h 1m 5s"},
	}

	for _, tt := range tests {
		got := FormatPipelineDuration(tt.seconds)
		if got != tt.want {
			t.Errorf("FormatPipelineDuration(%d) = %q, want %q", tt.seconds, got, tt.want)
		}
	}

	cov, err := ParseGitLabCoverage("87.5%")
	if err != nil || cov != 87.5 {
		t.Errorf("ParseGitLabCoverage(\"87.5%%\") = %f, err = %v", cov, err)
	}

	covClean, err := ParseGitLabCoverage(" 92.1  ")
	if err != nil || covClean != 92.1 {
		t.Errorf("ParseGitLabCoverage(\" 92.1  \") = %f, err = %v", covClean, err)
	}
}
