// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Platform Data Subsystem
// Package: models
// File: models_test.go
// ═══════════════════════════════════════════════════════════════

package models

import (
	"encoding/json"
	"testing"

	"github.com/scandrix/backend/internal/platformdata/domain/enums"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPullRequest_Serialization(t *testing.T) {
	pr := PullRequest{
		UUID:          "test-uuid-1234",
		Title:         "Refactor authentication middleware",
		Status:        "OPEN",
		Number:        42,
		URL:           "https://github.com/org/repo/pull/42",
		BaseBranchRef: "main",
		HeadBranchRef: "feature/auth",
		Repository: RepositoryInfo{
			ID:       "repo-1",
			Name:     "core-backend",
			FullName: "org/core-backend",
		},
		User: PullRequestUser{
			ID:       "user-1",
			Username: "dev-lead",
			Email:    "dev@scandrix.internal",
		},
		Files: []File{
			{
				ID:       "file-1",
				Path:     "internal/auth/middleware.go",
				Filename: "middleware.go",
				Status:   "modified",
				Added:    25,
				Deleted:  10,
				Changes:  35,
				Suggestions: []Suggestion{
					{
						ID:                 "sug-1",
						RelevantFile:       "internal/auth/middleware.go",
						Language:           "go",
						SuggestionContent:  "Use constant time comparison for HMAC verification",
						Severity:           "critical",
						Label:              "security",
						PriorityStatus:     enums.PriorityStatusPrioritized,
						DeliveryStatus:     enums.DeliveryStatusSent,
						ImplementationStatus: enums.ImplementationStatusImplemented,
					},
				},
			},
		},
		TotalAdded:   25,
		TotalDeleted: 10,
		TotalChanges: 35,
	}

	data, err := json.Marshal(pr)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	var decoded PullRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, pr.UUID, decoded.UUID)
	assert.Equal(t, pr.Title, decoded.Title)
	assert.Equal(t, 42, decoded.Number)
	assert.Equal(t, "core-backend", decoded.Repository.Name)
	assert.Len(t, decoded.Files, 1)
	assert.Len(t, decoded.Files[0].Suggestions, 1)
	assert.Equal(t, enums.DeliveryStatusSent, decoded.Files[0].Suggestions[0].DeliveryStatus)
}

func TestEnums_Values(t *testing.T) {
	assert.Equal(t, enums.DeliveryStatus("sent"), enums.DeliveryStatusSent)
	assert.Equal(t, enums.DeliveryStatus("not_sent"), enums.DeliveryStatusNotSent)
	assert.Equal(t, enums.ImplementationStatus("implemented"), enums.ImplementationStatusImplemented)
	assert.Equal(t, enums.PriorityStatus("prioritized"), enums.PriorityStatusPrioritized)
	assert.Equal(t, enums.PriorityStatus("discarded-by-drixy-fine-tuning"), enums.PriorityStatusDiscardedByDrixyFineTuning)
}
