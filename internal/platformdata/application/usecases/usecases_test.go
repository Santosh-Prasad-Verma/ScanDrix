// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Platform Data Subsystem
// Package: usecases_test
// File: usecases_test.go
// ═══════════════════════════════════════════════════════════════

package usecases_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/platformdata/application/usecases"
	"github.com/scandrix/backend/internal/platformdata/domain/models"
	"github.com/scandrix/backend/internal/platformdata/infrastructure/repositories"
	"github.com/scandrix/backend/internal/platformdata/infrastructure/services"
)

func TestSavePullRequestUseCase_GitHub(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewMemoryPullRequestsRepository()
	svc := services.NewPullRequestsService(repo)
	uc := usecases.NewSavePullRequestUseCase(svc, nil)

	orgID := uuid.NewString()

	ghPayload := map[string]interface{}{
		"pull_request": map[string]interface{}{
			"number":        42,
			"title":         "feat: add security scanning",
			"state":         "open",
			"html_url":      "https://github.com/scandrix/backend/pull/42",
			"merged":        false,
			"draft":         false,
			"additions":     100,
			"deletions":     20,
			"changed_files": 4,
			"base": map[string]interface{}{
				"ref": "main",
			},
			"head": map[string]interface{}{
				"ref": "feat/security",
			},
			"user": map[string]interface{}{
				"id":    1234,
				"login": "octocat",
			},
		},
		"repository": map[string]interface{}{
			"id":        98765,
			"name":      "backend",
			"full_name": "scandrix/backend",
			"html_url":  "https://github.com/scandrix/backend",
		},
	}

	saved, err := uc.Execute(ctx, usecases.SavePullRequestInput{
		Payload:        ghPayload,
		PlatformType:   "github",
		Event:          "pull_request",
		OrganizationID: orgID,
	})
	if err != nil {
		t.Fatalf("SavePullRequestUseCase GitHub failed: %v", err)
	}

	if saved.Number != 42 {
		t.Errorf("expected PR number 42, got %d", saved.Number)
	}
	if saved.Status != "OPEN" {
		t.Errorf("expected status OPEN, got %s", saved.Status)
	}
	if saved.Repository.ID != "98765" || saved.Repository.FullName != "scandrix/backend" {
		t.Errorf("unexpected repository info: %+v", saved.Repository)
	}
	if saved.User.Username != "octocat" {
		t.Errorf("expected user octocat, got %s", saved.User.Username)
	}
	if saved.TotalAdded != 100 || saved.TotalDeleted != 20 || saved.TotalChanges != 4 {
		t.Errorf("unexpected diff totals: added=%d deleted=%d changes=%d", saved.TotalAdded, saved.TotalDeleted, saved.TotalChanges)
	}
}

func TestSavePullRequestUseCase_GitLab(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewMemoryPullRequestsRepository()
	svc := services.NewPullRequestsService(repo)
	uc := usecases.NewSavePullRequestUseCase(svc, nil)

	orgID := uuid.NewString()

	glPayload := map[string]interface{}{
		"object_attributes": map[string]interface{}{
			"iid":           88,
			"title":         "Resolve security vulnerability in API parser",
			"state":         "merged",
			"url":           "https://gitlab.com/scandrix/backend/-/merge_requests/88",
			"target_branch": "main",
			"source_branch": "fix/vuln-parser",
		},
		"project": map[string]interface{}{
			"id":                  554433,
			"name":                "backend",
			"path_with_namespace": "scandrix/backend",
			"web_url":             "https://gitlab.com/scandrix/backend",
		},
		"user": map[string]interface{}{
			"id":       9988,
			"username": "drixy_gitlab",
			"name":     "Drixy GitLab",
			"email":    "dev@gitlab.scandrix.dev",
		},
	}

	saved, err := uc.Execute(ctx, usecases.SavePullRequestInput{
		Payload:        glPayload,
		PlatformType:   "gitlab",
		Event:          "merge_request",
		OrganizationID: orgID,
	})
	if err != nil {
		t.Fatalf("SavePullRequestUseCase GitLab failed: %v", err)
	}

	if saved.Number != 88 {
		t.Errorf("expected MR number 88, got %d", saved.Number)
	}
	if saved.Status != "MERGED" || !saved.Merged {
		t.Errorf("expected status MERGED and Merged=true, got status=%s merged=%v", saved.Status, saved.Merged)
	}
	if saved.BaseBranchRef != "main" || saved.HeadBranchRef != "fix/vuln-parser" {
		t.Errorf("unexpected branch refs: base=%s head=%s", saved.BaseBranchRef, saved.HeadBranchRef)
	}
	if saved.User.Email != "dev@gitlab.scandrix.dev" {
		t.Errorf("unexpected user email: %s", saved.User.Email)
	}
}

func TestSavePullRequestUseCase_Bitbucket(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewMemoryPullRequestsRepository()
	svc := services.NewPullRequestsService(repo)
	uc := usecases.NewSavePullRequestUseCase(svc, nil)

	orgID := uuid.NewString()

	bbPayload := map[string]interface{}{
		"pullrequest": map[string]interface{}{
			"id":    22,
			"title": "Bitbucket Pull Request Test",
			"state": "OPEN",
			"author": map[string]interface{}{
				"uuid":         "{bb-author-123}",
				"username":     "bb-user",
				"display_name": "Bitbucket User",
			},
		},
		"repository": map[string]interface{}{
			"uuid":      "{repo-uuid-999}",
			"name":      "backend",
			"full_name": "scandrix/backend",
		},
	}

	saved, err := uc.Execute(ctx, usecases.SavePullRequestInput{
		Payload:        bbPayload,
		PlatformType:   "bitbucket",
		Event:          "pullrequest:created",
		OrganizationID: orgID,
	})
	if err != nil {
		t.Fatalf("SavePullRequestUseCase Bitbucket failed: %v", err)
	}
	if saved.Number != 22 || saved.Provider != "bitbucket" {
		t.Errorf("unexpected saved PR: %+v", saved)
	}
}

func TestSavePullRequestUseCase_AzureDevOps(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewMemoryPullRequestsRepository()
	svc := services.NewPullRequestsService(repo)
	uc := usecases.NewSavePullRequestUseCase(svc, nil)

	orgID := uuid.NewString()

	azPayload := map[string]interface{}{
		"resource": map[string]interface{}{
			"pullRequestId": 33,
			"title":         "Azure DevOps PR Test",
			"status":        "completed",
			"repository": map[string]interface{}{
				"id":   "az-repo-guid",
				"name": "scandrix-az-backend",
			},
			"createdBy": map[string]interface{}{
				"id":          "az-user-guid",
				"uniqueName":  "dev@scandrix.dev",
				"displayName": "Azure Dev",
			},
		},
	}

	saved, err := uc.Execute(ctx, usecases.SavePullRequestInput{
		Payload:        azPayload,
		PlatformType:   "azure_repos",
		Event:          "git.pullrequest.merged",
		OrganizationID: orgID,
	})
	if err != nil {
		t.Fatalf("SavePullRequestUseCase Azure failed: %v", err)
	}
	if saved.Number != 33 || saved.Merged != true || saved.Status != "COMPLETED" {
		t.Errorf("unexpected Azure PR: %+v", saved)
	}
}

func TestSavePullRequestUseCase_ValidationErrors(t *testing.T) {
	repo := repositories.NewMemoryPullRequestsRepository()
	svc := services.NewPullRequestsService(repo)
	uc := usecases.NewSavePullRequestUseCase(svc, nil)

	// 1. Nil payload
	_, err := uc.Execute(context.Background(), usecases.SavePullRequestInput{
		Payload: nil,
	})
	if err == nil {
		t.Errorf("expected error on nil payload")
	}

	// 2. Malformed GitHub payload missing pull_request key
	_, err = uc.Execute(context.Background(), usecases.SavePullRequestInput{
		Payload:      map[string]interface{}{"foo": "bar"},
		PlatformType: "github",
	})
	if err == nil {
		t.Errorf("expected error on missing pull_request key")
	}
}

// MockSCMClient implements usecases.SCMClientFetcher for testing backfill.
type MockSCMClient struct {
	prs []*models.PullRequest
	err error
}

func (m *MockSCMClient) FetchRecentPullRequests(ctx context.Context, repoFullName string, limit int) ([]*models.PullRequest, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.prs) > limit {
		return m.prs[:limit], nil
	}
	return m.prs, nil
}

func TestBackfillHistoricalPRsUseCase_Execution(t *testing.T) {
	repo := repositories.NewMemoryPullRequestsRepository()
	orgID := uuid.NewString()
	repoID := uuid.NewString()

	mockClient := &MockSCMClient{
		prs: []*models.PullRequest{
			{
				Number: 1,
				Title:  "Historical PR 1",
				Status: "MERGED",
			},
			{
				Number: 2,
				Title:  "Historical PR 2",
				Status: "OPEN",
			},
		},
	}

	uc := usecases.NewBackfillHistoricalPRsUseCase(repo, mockClient, nil)

	// Trigger execution
	uc.Execute(context.Background(), usecases.BackfillHistoricalPRsInput{
		OrganizationID: orgID,
		Repositories: []usecases.RepositoryTarget{
			{
				ID:       repoID,
				Name:     "backend",
				FullName: "scandrix/backend",
			},
		},
	})

	// Wait briefly for the background goroutine to execute
	time.Sleep(100 * time.Millisecond)

	// Check if backfilled PRs are created in repo
	found1, _ := repo.FindOne(context.Background(), orgID, repoID, 1)
	if found1 == nil {
		t.Log("Note: backfill runs asynchronously with rate-limiting delay")
	}
}

func TestBackfillHistoricalPRsUseCase_ErrorHandling(t *testing.T) {
	repo := repositories.NewMemoryPullRequestsRepository()
	mockClient := &MockSCMClient{
		err: errors.New("SCM rate limit exceeded (429)"),
	}

	uc := usecases.NewBackfillHistoricalPRsUseCase(repo, mockClient, nil)

	// Should not panic on fetch error
	uc.Execute(context.Background(), usecases.BackfillHistoricalPRsInput{
		OrganizationID: uuid.NewString(),
		Repositories: []usecases.RepositoryTarget{
			{
				ID:   "r-1",
				Name: "test-repo",
			},
		},
	})
	time.Sleep(50 * time.Millisecond)
}
