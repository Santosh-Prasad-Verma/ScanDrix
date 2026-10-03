// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Platform Data Subsystem
// Package: services
// File: service.go
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/platformdata/domain/contracts"
	"github.com/scandrix/backend/internal/platformdata/domain/enums"
	"github.com/scandrix/backend/internal/platformdata/domain/models"
)

// PullRequestsService orchestrates business workflows for pull requests.
type PullRequestsService struct {
	contracts.IPullRequestsRepository
}

// NewPullRequestsService creates a domain service wrapping the repository.
func NewPullRequestsService(repo contracts.IPullRequestsRepository) *PullRequestsService {
	return &PullRequestsService{
		IPullRequestsRepository: repo,
	}
}

// AggregateAndSaveDataStructure groups suggestions by target file and saves the unified PR document.
func (s *PullRequestsService) AggregateAndSaveDataStructure(
	ctx context.Context,
	pr *models.PullRequest,
	changedFiles []models.File,
	prioritizedSuggestions []models.Suggestion,
	unusedSuggestions []models.Suggestion,
	commits []models.Commit,
) (*models.PullRequest, error) {
	if pr == nil {
		return nil, errors.New("pull request cannot be nil")
	}

	// Index existing files by normalized path
	fileIndexMap := make(map[string]int)
	for i, f := range pr.Files {
		norm := strings.TrimPrefix(f.Path, "./")
		fileIndexMap[norm] = i
	}

	// Merge in changed files
	for _, cf := range changedFiles {
		norm := strings.TrimPrefix(cf.Path, "./")
		if idx, ok := fileIndexMap[norm]; ok {
			pr.Files[idx].Added = cf.Added
			pr.Files[idx].Deleted = cf.Deleted
			pr.Files[idx].Changes = cf.Changes
			pr.Files[idx].Status = cf.Status
			pr.Files[idx].SHA = cf.SHA
		} else {
			if cf.ID == "" {
				cf.ID = uuid.NewString()
			}
			pr.Files = append(pr.Files, cf)
			fileIndexMap[norm] = len(pr.Files) - 1
		}
	}

	// Group suggestions by file
	allSuggestions := append(prioritizedSuggestions, unusedSuggestions...)
	for _, sug := range allSuggestions {
		norm := strings.TrimPrefix(sug.RelevantFile, "./")
		idx, ok := fileIndexMap[norm]
		if !ok {
			// The file is absent from the PR, so "modified" claimed a change
			// that did not occur. "added" marks the entry as new to this
			// report (SARIF baselineState "new").
			// AUDIT_REMEDIATION.md F-47.
			stubFile := models.File{
				ID:          uuid.NewString(),
				Path:        sug.RelevantFile,
				Filename:    sug.RelevantFile,
				Status:      "added",
				Suggestions: []models.Suggestion{},
			}
			pr.Files = append(pr.Files, stubFile)
			idx = len(pr.Files) - 1
			fileIndexMap[norm] = idx
		}

		if sug.ID == "" {
			sug.ID = uuid.NewString()
		}
		nowStr := time.Now().UTC().Format(time.RFC3339Nano)
		if sug.CreatedAt == "" {
			sug.CreatedAt = nowStr
		}
		sug.UpdatedAt = nowStr

		// Avoid duplicate suggestion ID
		exists := false
		for i, es := range pr.Files[idx].Suggestions {
			if es.ID == sug.ID {
				pr.Files[idx].Suggestions[i] = sug
				exists = true
				break
			}
		}
		if !exists {
			pr.Files[idx].Suggestions = append(pr.Files[idx].Suggestions, sug)
		}
	}

	// Recalculate totals
	totalAdded, totalDeleted, totalChanges := 0, 0, 0
	for _, f := range pr.Files {
		totalAdded += f.Added
		totalDeleted += f.Deleted
		totalChanges += f.Changes
	}
	pr.TotalAdded = totalAdded
	pr.TotalDeleted = totalDeleted
	pr.TotalChanges = totalChanges

	if len(commits) > 0 {
		pr.Commits = commits
	}

	// Persist to repository
	return s.Create(ctx, pr)
}

// ExtractUser extracts normalized PR author info from a platform webhook payload.
func (s *PullRequestsService) ExtractUser(payload map[string]interface{}, platformType string) (*models.PullRequestUser, error) {
	if payload == nil {
		return nil, errors.New("empty payload")
	}

	switch strings.ToLower(platformType) {
	case "github":
		if u, ok := payload["user"].(map[string]interface{}); ok {
			return &models.PullRequestUser{
				ID:       fmt.Sprintf("%v", u["id"]),
				Username: fmt.Sprintf("%v", u["login"]),
				Name:     fmt.Sprintf("%v", u["name"]),
				Email:    fmt.Sprintf("%v", u["email"]),
			}, nil
		}
	case "gitlab":
		if u, ok := payload["user"].(map[string]interface{}); ok {
			return &models.PullRequestUser{
				ID:       fmt.Sprintf("%v", u["id"]),
				Username: fmt.Sprintf("%v", u["username"]),
				Name:     fmt.Sprintf("%v", u["name"]),
				Email:    fmt.Sprintf("%v", u["email"]),
			}, nil
		}
	case "bitbucket":
		if a, ok := payload["author"].(map[string]interface{}); ok {
			user := &models.PullRequestUser{
				ID:       fmt.Sprintf("%v", a["uuid"]),
				Username: fmt.Sprintf("%v", a["username"]),
				Name:     fmt.Sprintf("%v", a["display_name"]),
			}
			return user, nil
		}
	case "azure_repos", "azuredevops":
		if cb, ok := payload["createdBy"].(map[string]interface{}); ok {
			return &models.PullRequestUser{
				ID:       fmt.Sprintf("%v", cb["id"]),
				Username: fmt.Sprintf("%v", cb["uniqueName"]),
				Name:     fmt.Sprintf("%v", cb["displayName"]),
			}, nil
		}
	}

	// Generic fallback
	return &models.PullRequestUser{
		ID:       "unknown",
		Username: "unknown",
	}, nil
}

// ExtractUsers extracts reviewers and assignees from a webhook payload.
func (s *PullRequestsService) ExtractUsers(payload map[string]interface{}, platformType string) ([]models.PullRequestUser, error) {
	var users []models.PullRequestUser
	if payload == nil {
		return users, nil
	}

	// Check reviewers array
	if rawReviewers, ok := payload["requested_reviewers"].([]interface{}); ok {
		for _, r := range rawReviewers {
			if m, ok := r.(map[string]interface{}); ok {
				users = append(users, models.PullRequestUser{
					ID:       fmt.Sprintf("%v", m["id"]),
					Username: fmt.Sprintf("%v", m["login"]),
					Name:     fmt.Sprintf("%v", m["name"]),
				})
			}
		}
	}

	return users, nil
}

// AddPRLevelSuggestions attaches holistic, PR-level suggestions to the PR document.
func (s *PullRequestsService) AddPRLevelSuggestions(
	ctx context.Context,
	orgID string,
	repoID string,
	prNumber int,
	suggestions []models.SuggestionByPR,
) error {
	pr, err := s.FindByNumberAndRepositoryID(ctx, orgID, repoID, prNumber)
	if err != nil {
		return err
	}
	if pr == nil {
		return errors.New("pull request not found")
	}

	for i := range suggestions {
		if suggestions[i].ID == "" {
			suggestions[i].ID = uuid.NewString()
		}
		if suggestions[i].DeliveryStatus == "" {
			suggestions[i].DeliveryStatus = enums.DeliveryStatusSent
		}
		nowStr := time.Now().UTC().Format(time.RFC3339Nano)
		if suggestions[i].CreatedAt == "" {
			suggestions[i].CreatedAt = nowStr
		}
		suggestions[i].UpdatedAt = nowStr
	}

	pr.PRLevelSuggestions = append(pr.PRLevelSuggestions, suggestions...)
	_, err = s.Update(ctx, pr)
	return err
}
