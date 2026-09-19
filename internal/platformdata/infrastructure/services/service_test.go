// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Platform Data Subsystem
// Package: services_test
// File: service_test.go
// ═══════════════════════════════════════════════════════════════

package services_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/platformdata/domain/enums"
	"github.com/scandrix/backend/internal/platformdata/domain/models"
	"github.com/scandrix/backend/internal/platformdata/infrastructure/repositories"
	"github.com/scandrix/backend/internal/platformdata/infrastructure/services"
)

func TestPullRequestsService_AggregateAndSaveDataStructure(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewMemoryPullRequestsRepository()
	svc := services.NewPullRequestsService(repo)

	orgID := uuid.NewString()
	repoID := uuid.NewString()

	pr := &models.PullRequest{
		Title:          "Feature API Integration",
		Status:         "OPEN",
		Number:         10,
		Provider:       "github",
		OrganizationID: orgID,
		Repository: models.RepositoryInfo{
			ID:       repoID,
			Name:     "backend",
			FullName: "scandrix/backend",
		},
		Files: []models.File{
			{
				Path:     "pkg/api/client.go",
				Filename: "client.go",
				Added:    20,
				Deleted:  5,
				Changes:  25,
			},
		},
	}

	changedFiles := []models.File{
		{
			Path:     "pkg/api/client.go",
			Filename: "client.go",
			Added:    30, // Updated
			Deleted:  8,
			Changes:  38,
			Status:   "modified",
			SHA:      "sha12345",
		},
		{
			Path:     "pkg/api/types.go",
			Filename: "types.go",
			Added:    50,
			Deleted:  0,
			Changes:  50,
			Status:   "added",
			SHA:      "sha67890",
		},
	}

	prioritized := []models.Suggestion{
		{
			ID:                "sug-client-1",
			RelevantFile:      "pkg/api/client.go",
			SuggestionContent: "Set reasonable request timeout on HTTP client",
			Severity:          "HIGH",
			PriorityStatus:    enums.PriorityStatusPrioritized,
			DeliveryStatus:    enums.DeliveryStatusSent,
		},
	}

	unused := []models.Suggestion{
		{
			ID:                "sug-untracked-file",
			RelevantFile:      "pkg/api/untracked.go", // Does not exist in changedFiles yet
			SuggestionContent: "Add package documentation",
			Severity:          "LOW",
			PriorityStatus:    enums.PriorityStatusDiscardedBySafeguard,
			DeliveryStatus:    enums.DeliveryStatusNotSent,
		},
	}

	commits := []models.Commit{
		{
			SHA:     "c1a2b3",
			Message: "feat: add api client",
			Author: models.CommitAuthor{
				Name:  "Drixy Dev",
				Email: "dev@scandrix.dev",
			},
		},
	}

	saved, err := svc.AggregateAndSaveDataStructure(ctx, pr, changedFiles, prioritized, unused, commits)
	if err != nil {
		t.Fatalf("AggregateAndSaveDataStructure failed: %v", err)
	}

	// Verify files merged
	if len(saved.Files) != 3 { // client.go, types.go, untracked.go (stub)
		t.Fatalf("expected 3 files, got %d", len(saved.Files))
	}

	// Verify client.go updated
	var clientFile *models.File
	for i := range saved.Files {
		if saved.Files[i].Path == "pkg/api/client.go" {
			clientFile = &saved.Files[i]
			break
		}
	}
	if clientFile == nil {
		t.Fatalf("client.go file missing")
	}
	if clientFile.Added != 30 || clientFile.Deleted != 8 {
		t.Errorf("expected updated counts (added=30, deleted=8), got (added=%d, deleted=%d)", clientFile.Added, clientFile.Deleted)
	}
	if len(clientFile.Suggestions) != 1 || clientFile.Suggestions[0].ID != "sug-client-1" {
		t.Errorf("expected prioritized suggestion attached to client.go")
	}

	// Verify stub created for untracked.go
	var untrackedFile *models.File
	for i := range saved.Files {
		if saved.Files[i].Path == "pkg/api/untracked.go" {
			untrackedFile = &saved.Files[i]
			break
		}
	}
	if untrackedFile == nil {
		t.Fatalf("stub file for untracked.go was not created")
	}
	if len(untrackedFile.Suggestions) != 1 {
		t.Errorf("expected suggestion attached to untracked stub file")
	}

	// Verify totals recalculated: 30+50+0 = 80 added, 8+0+0 = 8 deleted, 38+50+0 = 88 changes
	if saved.TotalAdded != 80 || saved.TotalDeleted != 8 || saved.TotalChanges != 88 {
		t.Errorf("unexpected totals: added=%d, deleted=%d, changes=%d", saved.TotalAdded, saved.TotalDeleted, saved.TotalChanges)
	}

	// Verify commits attached
	if len(saved.Commits) != 1 || saved.Commits[0].SHA != "c1a2b3" {
		t.Errorf("expected commit attached")
	}
}

func TestPullRequestsService_ExtractUser(t *testing.T) {
	svc := services.NewPullRequestsService(repositories.NewMemoryPullRequestsRepository())

	// 1. GitHub
	ghPayload := map[string]interface{}{
		"user": map[string]interface{}{
			"id":    12345,
			"login": "octocat",
			"name":  "Mona Lisa Octocat",
			"email": "octocat@github.com",
		},
	}
	u, err := svc.ExtractUser(ghPayload, "github")
	if err != nil || u.Username != "octocat" || u.ID != "12345" {
		t.Fatalf("GitHub ExtractUser failed: %+v (err: %v)", u, err)
	}

	// 2. GitLab
	glPayload := map[string]interface{}{
		"user": map[string]interface{}{
			"id":       6789,
			"username": "tanuki",
			"name":     "GitLab Tanuki",
			"email":    "tanuki@gitlab.com",
		},
	}
	u, err = svc.ExtractUser(glPayload, "gitlab")
	if err != nil || u.Username != "tanuki" || u.Email != "tanuki@gitlab.com" {
		t.Fatalf("GitLab ExtractUser failed: %+v (err: %v)", u, err)
	}

	// 3. Bitbucket
	bbPayload := map[string]interface{}{
		"author": map[string]interface{}{
			"uuid":         "{bb-uuid-123}",
			"username":     "atlassian-user",
			"display_name": "Atlassian User",
		},
	}
	u, err = svc.ExtractUser(bbPayload, "bitbucket")
	if err != nil || u.Username != "atlassian-user" || u.ID != "{bb-uuid-123}" {
		t.Fatalf("Bitbucket ExtractUser failed: %+v (err: %v)", u, err)
	}

	// 4. Azure DevOps
	azPayload := map[string]interface{}{
		"createdBy": map[string]interface{}{
			"id":          "az-guid-456",
			"uniqueName":  "dev@azure.com",
			"displayName": "Azure Dev",
		},
	}
	u, err = svc.ExtractUser(azPayload, "azure_repos")
	if err != nil || u.Username != "dev@azure.com" || u.Name != "Azure Dev" {
		t.Fatalf("Azure ExtractUser failed: %+v (err: %v)", u, err)
	}

	// 5. Generic fallback
	genericPayload := map[string]interface{}{
		"foo": "bar",
	}
	u, err = svc.ExtractUser(genericPayload, "custom")
	if err != nil || u.Username != "unknown" {
		t.Fatalf("Generic ExtractUser failed: %+v", u)
	}

	// 6. Nil payload error
	_, err = svc.ExtractUser(nil, "github")
	if err == nil {
		t.Errorf("expected error on nil payload")
	}
}

func TestPullRequestsService_ExtractUsers(t *testing.T) {
	svc := services.NewPullRequestsService(repositories.NewMemoryPullRequestsRepository())

	payload := map[string]interface{}{
		"requested_reviewers": []interface{}{
			map[string]interface{}{
				"id":    101,
				"login": "rev1",
				"name":  "Reviewer One",
			},
			map[string]interface{}{
				"id":    102,
				"login": "rev2",
				"name":  "Reviewer Two",
			},
		},
	}

	users, err := svc.ExtractUsers(payload, "github")
	if err != nil || len(users) != 2 {
		t.Fatalf("ExtractUsers failed: %+v (err: %v)", users, err)
	}
	if users[0].Username != "rev1" || users[1].Username != "rev2" {
		t.Errorf("unexpected reviewers extracted: %+v", users)
	}
}

func TestPullRequestsService_AddPRLevelSuggestions(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewMemoryPullRequestsRepository()
	svc := services.NewPullRequestsService(repo)

	orgID := uuid.NewString()
	repoID := uuid.NewString()

	pr := &models.PullRequest{
		Title:          "Architecture Refactor",
		Status:         "OPEN",
		Number:         55,
		OrganizationID: orgID,
		Repository: models.RepositoryInfo{
			ID: repoID,
		},
	}
	_, _ = repo.Create(ctx, pr)

	prLevelSugs := []models.SuggestionByPR{
		{
			OneSentenceSummary: "Introduce circuit breaker pattern across outbound network calls",
			Severity:           "HIGH",
			DeliveryStatus:     enums.DeliveryStatusSent,
		},
	}

	err := svc.AddPRLevelSuggestions(ctx, orgID, repoID, 55, prLevelSugs)
	if err != nil {
		t.Fatalf("AddPRLevelSuggestions failed: %v", err)
	}

	found, _ := repo.FindOne(ctx, orgID, repoID, 55)
	if len(found.PRLevelSuggestions) != 1 {
		t.Fatalf("expected 1 PR level suggestion, got %d", len(found.PRLevelSuggestions))
	}
	if found.PRLevelSuggestions[0].OneSentenceSummary != prLevelSugs[0].OneSentenceSummary {
		t.Errorf("unexpected summary: %s", found.PRLevelSuggestions[0].OneSentenceSummary)
	}

	// Non-existent PR returns error
	err = svc.AddPRLevelSuggestions(ctx, orgID, repoID, 9999, prLevelSugs)
	if err == nil {
		t.Errorf("expected error when adding PR level suggestion to non-existent PR")
	}
}
