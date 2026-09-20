package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubDeploymentsAndStatuses(t *testing.T) {
	mux := http.NewServeMux()

	// Deployments
	mux.HandleFunc("/repos/my-org/my-repo/deployments", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			deployments := []GitHubDeployment{
				{
					ID:          1001,
					SHA:         "sha-abc",
					Ref:         "main",
					Task:        "deploy",
					Environment: "production",
				},
			}
			_ = json.NewEncoder(w).Encode(deployments)
			return
		}
		if r.Method == http.MethodPost {
			var req CreateDeploymentRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			dep := GitHubDeployment{
				ID:          1002,
				SHA:         "sha-def",
				Ref:         req.Ref,
				Task:        req.Task,
				Environment: req.Environment,
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(dep)
			return
		}
	})

	// Deployment Statuses
	mux.HandleFunc("/repos/my-org/my-repo/deployments/1001/statuses", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			statuses := []GitHubDeploymentStatus{
				{
					ID:          5001,
					State:       "success",
					Environment: "production",
					TargetURL:   "https://scandrix.dev/deploy/5001",
				},
			}
			_ = json.NewEncoder(w).Encode(statuses)
			return
		}
		if r.Method == http.MethodPost {
			var req CreateDeploymentStatusRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			st := GitHubDeploymentStatus{
				ID:          5002,
				State:       req.State,
				Environment: req.Environment,
				TargetURL:   req.TargetURL,
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(st)
			return
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewGitHubDeploymentsAndProtectionsService(server.URL, "ghp_secret_token", server.Client())
	ctx := context.Background()

	// 1. List Deployments
	deployments, err := client.ListDeployments(ctx, "my-org", "my-repo", "production")
	if err != nil {
		t.Fatalf("ListDeployments failed: %v", err)
	}
	if len(deployments) != 1 || deployments[0].ID != 1001 {
		t.Fatalf("unexpected deployments: %+v", deployments)
	}

	// 2. Create Deployment
	newDep, err := client.CreateDeployment(ctx, "my-org", "my-repo", CreateDeploymentRequest{
		Ref:         "main",
		Task:        "deploy",
		Environment: "staging",
	})
	if err != nil {
		t.Fatalf("CreateDeployment failed: %v", err)
	}
	if newDep.ID != 1002 || newDep.Environment != "staging" {
		t.Fatalf("unexpected created deployment: %+v", newDep)
	}

	// 3. List Deployment Statuses
	statuses, err := client.ListDeploymentStatuses(ctx, "my-org", "my-repo", 1001)
	if err != nil {
		t.Fatalf("ListDeploymentStatuses failed: %v", err)
	}
	if len(statuses) != 1 || statuses[0].State != "success" {
		t.Fatalf("unexpected statuses: %+v", statuses)
	}

	// 4. Create Deployment Status
	newSt, err := client.CreateDeploymentStatus(ctx, "my-org", "my-repo", 1001, CreateDeploymentStatusRequest{
		State:       "in_progress",
		Environment: "production",
		TargetURL:   "https://scandrix.dev/deploy/5002",
	})
	if err != nil {
		t.Fatalf("CreateDeploymentStatus failed: %v", err)
	}
	if newSt.ID != 5002 || newSt.State != "in_progress" {
		t.Fatalf("unexpected created status: %+v", newSt)
	}
}

func TestGitHubBranchProtectionAndDispatch(t *testing.T) {
	mux := http.NewServeMux()

	// Branch Protection
	mux.HandleFunc("/repos/my-org/my-repo/branches/main/protection", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			bp := GitHubBranchProtection{
				URL: "https://api.github.com/repos/my-org/my-repo/branches/main/protection",
				EnforceAdmins: &GitHubEnforceAdmins{
					Enabled: true,
				},
				RequiredStatusChecks: &GitHubRequiredStatusChecks{
					Strict:   true,
					Contexts: []string{"continuous-integration/travis-ci", "scandrix/review"},
				},
				RequiredPullRequestReviews: &GitHubRequiredPRReviews{
					RequiredApprovingReviewCount: 2,
					RequireCodeOwnerReviews:      true,
				},
			}
			_ = json.NewEncoder(w).Encode(bp)
			return
		}
		if r.Method == http.MethodPut {
			var req BranchProtectionRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			bp := GitHubBranchProtection{
				URL: "https://api.github.com/repos/my-org/my-repo/branches/main/protection",
				EnforceAdmins: &GitHubEnforceAdmins{
					Enabled: req.EnforceAdmins,
				},
				RequiredStatusChecks: req.RequiredStatusChecks,
				RequiredPullRequestReviews: req.RequiredPullRequestReviews,
			}
			_ = json.NewEncoder(w).Encode(bp)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	})

	// Repository Dispatch
	mux.HandleFunc("/repos/my-org/my-repo/dispatches", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload["event_type"] == "custom-review-trigger" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewGitHubDeploymentsAndProtectionsService(server.URL, "ghp_secret_token", server.Client())
	ctx := context.Background()

	// 1. Get Branch Protection
	protection, err := client.GetBranchProtection(ctx, "my-org", "my-repo", "main")
	if err != nil {
		t.Fatalf("GetBranchProtection failed: %v", err)
	}
	if !protection.EnforceAdmins.Enabled || protection.RequiredPullRequestReviews.RequiredApprovingReviewCount != 2 {
		t.Fatalf("unexpected protection: %+v", protection)
	}

	// 2. Update Branch Protection
	updated, err := client.UpdateBranchProtection(ctx, "my-org", "my-repo", "main", BranchProtectionRequest{
		EnforceAdmins: false,
		RequiredStatusChecks: &GitHubRequiredStatusChecks{
			Strict:   false,
			Contexts: []string{"scandrix/review"},
		},
		RequiredPullRequestReviews: &GitHubRequiredPRReviews{
			RequiredApprovingReviewCount: 1,
		},
	})
	if err != nil {
		t.Fatalf("UpdateBranchProtection failed: %v", err)
	}
	if updated.EnforceAdmins.Enabled || updated.RequiredPullRequestReviews.RequiredApprovingReviewCount != 1 {
		t.Fatalf("unexpected updated protection: %+v", updated)
	}

	// 3. Delete Branch Protection
	if err := client.DeleteBranchProtection(ctx, "my-org", "my-repo", "main"); err != nil {
		t.Fatalf("DeleteBranchProtection failed: %v", err)
	}

	// 4. Create Repository Dispatch
	if err := client.CreateRepositoryDispatch(ctx, "my-org", "my-repo", "custom-review-trigger", map[string]any{"source": "cli"}); err != nil {
		t.Fatalf("CreateRepositoryDispatch failed: %v", err)
	}
}
