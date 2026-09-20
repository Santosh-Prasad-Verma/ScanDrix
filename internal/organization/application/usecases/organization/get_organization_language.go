// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package orgusecases

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// TrackedRepositoryReader provides access to configured repositories.
type TrackedRepositoryReader interface {
	ListTrackedRepositories(ctx context.Context, wsID uuid.UUID) ([]models.TrackedRepository, error)
}

// GetOrganizationLanguageUseCase detects the primary development language across tracked repositories.
type GetOrganizationLanguageUseCase struct {
	repoReader TrackedRepositoryReader
}

func NewGetOrganizationLanguageUseCase(reader TrackedRepositoryReader) *GetOrganizationLanguageUseCase {
	return &GetOrganizationLanguageUseCase{repoReader: reader}
}

// Execute detects language for an organization workspace with optional team, repository, and sample size filters.
func (uc *GetOrganizationLanguageUseCase) Execute(ctx context.Context, wsID uuid.UUID, teamID *uuid.UUID, repoID string, sampleSize int) (string, error) {
	if wsID == uuid.Nil || uc.repoReader == nil {
		return "", nil
	}

	repos, err := uc.repoReader.ListTrackedRepositories(ctx, wsID)
	if err != nil || len(repos) == 0 {
		return "", nil
	}

	if repoID != "" {
		filtered := make([]models.TrackedRepository, 0)
		for _, r := range repos {
			if r.ID.String() == repoID || r.ExternalID == repoID {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) > 0 {
			repos = filtered
		}
	}

	if sampleSize <= 0 {
		sampleSize = 5
	}
	if sampleSize > 10 {
		sampleSize = 10
	}
	if len(repos) > sampleSize {
		repos = repos[:sampleSize]
	}

	// Tally languages from repository metadata
	langCounts := make(map[string]int)
	for _, r := range repos {
		path := strings.ToLower(r.NamespacePath)
		switch {
		case strings.Contains(path, "go") || strings.HasSuffix(path, "-go"):
			langCounts["go"]++
		case strings.Contains(path, "python") || strings.Contains(path, "py"):
			langCounts["python"]++
		case strings.Contains(path, "rust") || strings.HasSuffix(path, "-rs"):
			langCounts["rust"]++
		case strings.Contains(path, "ts") || strings.Contains(path, "typescript") || strings.Contains(path, "node"):
			langCounts["typescript"]++
		case strings.Contains(path, "java") || strings.Contains(path, "spring"):
			langCounts["java"]++
		case strings.Contains(path, "csharp") || strings.Contains(path, "dotnet"):
			langCounts["csharp"]++
		}
	}

	dominant := ""
	maxCount := 0
	for lang, count := range langCounts {
		if count > maxCount {
			dominant = lang
			maxCount = count
		}
	}

	return dominant, nil
}

// ExecuteSimple detects language with default parameters for backward compatibility.
func (uc *GetOrganizationLanguageUseCase) ExecuteSimple(ctx context.Context, wsID uuid.UUID) (string, error) {
	return uc.Execute(ctx, wsID, nil, "", 5)
}
