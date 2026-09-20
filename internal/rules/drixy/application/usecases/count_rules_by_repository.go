// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: count_rules_by_repository.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// DrixyRuleRepositoryCount aggregates active and paused rules for a repository scope.
type DrixyRuleRepositoryCount struct {
	RepositoryID string  `json:"repositoryId"`
	DirectoryID  *string `json:"directoryId"`
	Count        int     `json:"count"`
}

// CountRulesByRepositoryUseCase calculates rule counts per repository and directory in a single operation.
type CountRulesByRepositoryUseCase struct {
	rulesService contracts.IDrixyRulesService
}

// NewCountRulesByRepositoryUseCase constructs the count use case.
func NewCountRulesByRepositoryUseCase(
	rulesService contracts.IDrixyRulesService,
) *CountRulesByRepositoryUseCase {
	return &CountRulesByRepositoryUseCase{
		rulesService: rulesService,
	}
}

// Execute counts all active and paused rules grouped by repository and directory.
func (uc *CountRulesByRepositoryUseCase) Execute(
	ctx context.Context,
	organizationID string,
	allowedRepoScope []string,
) ([]DrixyRuleRepositoryCount, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	entity, err := uc.rulesService.FindByOrganizationID(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch organization rules: %w", err)
	}
	rules := entity.Rules()
	if entity == nil || len(rules) == 0 {
		return []DrixyRuleRepositoryCount{}, nil
	}

	// Map key: "repoID|dirID"
	countsMap := make(map[string]*DrixyRuleRepositoryCount)

	for _, rule := range rules {
		if rule.Status != interfaces.StatusActive && rule.Status != interfaces.StatusPaused {
			continue
		}

		repoID := rule.RepositoryID
		if repoID == "" {
			repoID = "global"
		}

		var dirID *string
		key := repoID
		if rule.DirectoryID != "" {
			d := rule.DirectoryID
			dirID = &d
			key = repoID + "|" + d
		}

		if entry, exists := countsMap[key]; exists {
			entry.Count++
		} else {
			countsMap[key] = &DrixyRuleRepositoryCount{
				RepositoryID: repoID,
				DirectoryID:  dirID,
				Count:        1,
			}
		}
	}

	// Filter by allowed repo scope if provided
	var allowedSet map[string]bool
	if len(allowedRepoScope) > 0 {
		allowedSet = make(map[string]bool)
		for _, r := range allowedRepoScope {
			allowedSet[r] = true
		}
		allowedSet["global"] = true
	}

	result := make([]DrixyRuleRepositoryCount, 0, len(countsMap))
	for _, entry := range countsMap {
		if allowedSet != nil && !allowedSet[entry.RepositoryID] {
			continue
		}
		result = append(result, *entry)
	}

	return result, nil
}
