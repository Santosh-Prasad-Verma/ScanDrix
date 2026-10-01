// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: generate_initial_drixy_rules.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"fmt"
	"sync"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// GenerateInitialDrixyRulesUseCase seeds an organization repository with rules from past PRs when enabled for the first time.
type GenerateInitialDrixyRulesUseCase struct {
	rulesService         contracts.IDrixyRulesService
	generateRulesUseCase *GenerateDrixyRulesUseCase
	lockMu               sync.Mutex
	activeRuns           map[string]bool
}

// NewGenerateInitialDrixyRulesUseCase constructs the initial generator use case.
func NewGenerateInitialDrixyRulesUseCase(
	rulesService contracts.IDrixyRulesService,
	generateRulesUseCase *GenerateDrixyRulesUseCase,
) *GenerateInitialDrixyRulesUseCase {
	return &GenerateInitialDrixyRulesUseCase{
		rulesService:         rulesService,
		generateRulesUseCase: generateRulesUseCase,
		activeRuns:           make(map[string]bool),
	}
}

// Execute seeds initial historical rules if not already present.
func (uc *GenerateInitialDrixyRulesUseCase) Execute(
	ctx context.Context,
	organizationID, teamID string,
	repositoryID string,
) error {
	if organizationID == "" || repositoryID == "" {
		return nil
	}

	lockKey := fmt.Sprintf("%s:%s", organizationID, repositoryID)
	uc.lockMu.Lock()
	if uc.activeRuns[lockKey] {
		uc.lockMu.Unlock()
		return nil
	}
	uc.activeRuns[lockKey] = true
	uc.lockMu.Unlock()

	defer func() {
		uc.lockMu.Lock()
		delete(uc.activeRuns, lockKey)
		uc.lockMu.Unlock()
	}()

	hasRules, err := uc.HasPastReviewRules(ctx, organizationID, repositoryID)
	if err != nil || hasRules {
		return nil
	}

	if uc.generateRulesUseCase != nil {
		_, _ = uc.generateRulesUseCase.Execute(ctx, GenerateDrixyRulesDTO{
			TeamID:          teamID,
			Months:          3,
			RepositoriesIDs: []string{repositoryID},
		}, organizationID)
	}

	return nil
}

// HasPastReviewRules determines if the repository already carries rules harvested from past reviews.
func (uc *GenerateInitialDrixyRulesUseCase) HasPastReviewRules(
	ctx context.Context,
	organizationID, repositoryID string,
) (bool, error) {
	entity, err := uc.rulesService.FindByOrganizationID(ctx, organizationID)
	if err != nil || entity == nil {
		return false, err
	}

	for _, rule := range entity.Rules() {
		if rule.RepositoryID == repositoryID && rule.Origin == interfaces.DrixyRulesOriginPastReviews {
			return true, nil
		}
	}

	return false, nil
}
