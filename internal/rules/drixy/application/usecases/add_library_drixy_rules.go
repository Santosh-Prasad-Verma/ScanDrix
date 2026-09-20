// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: add_library_drixy_rules.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
)

// AddLibraryDrixyRulesUseCase clones catalog rules into targeted repositories.
type AddLibraryDrixyRulesUseCase struct {
	createOrUpdateUC *CreateOrUpdateDrixyRulesUseCase
}

// NewAddLibraryDrixyRulesUseCase creates a new library rule addition use case.
func NewAddLibraryDrixyRulesUseCase(createUC *CreateOrUpdateDrixyRulesUseCase) *AddLibraryDrixyRulesUseCase {
	return &AddLibraryDrixyRulesUseCase{createOrUpdateUC: createUC}
}

// Execute copies library rule into targeted repositories.
func (uc *AddLibraryDrixyRulesUseCase) Execute(
	ctx context.Context,
	organizationID string,
	dto dtos.AddLibraryDrixyRulesDto,
	userInfo *contracts.UserAuditInfo,
) ([]interfaces.DrixyRule, error) {
	if organizationID == "" {
		return nil, errors.New("organization ID is required")
	}

	var results []interfaces.DrixyRule

	// 1. Add to repositories
	for _, repoID := range dto.RepositoriesIDs {
		createDTO := dtos.CreateDrixyRuleDto{
			UUID:         uuid.New().String(),
			Title:        dto.Title,
			Rule:         dto.Rule,
			Path:         dto.Path,
			Severity:     dto.Severity,
			RepositoryID: repoID,
			Examples:     dto.Examples,
			Origin:       interfaces.DrixyRulesOriginLibrary,
			Status:       interfaces.DrixyRulesStatusActive,
			TeamID:       dto.TeamID,
		}
		saved, err := uc.createOrUpdateUC.Execute(ctx, createDTO, organizationID, userInfo)
		if err == nil && saved != nil {
			results = append(results, *saved)
		}
	}

	// 2. Add to specific directories
	for _, dirInfo := range dto.DirectoriesInfo {
		createDTO := dtos.CreateDrixyRuleDto{
			UUID:         uuid.New().String(),
			Title:        dto.Title,
			Rule:         dto.Rule,
			Path:         dto.Path,
			Severity:     dto.Severity,
			RepositoryID: dirInfo.RepositoryID,
			DirectoryID:  dirInfo.DirectoryID,
			Examples:     dto.Examples,
			Origin:       interfaces.DrixyRulesOriginLibrary,
			Status:       interfaces.DrixyRulesStatusActive,
			TeamID:       dto.TeamID,
		}
		saved, err := uc.createOrUpdateUC.Execute(ctx, createDTO, organizationID, userInfo)
		if err == nil && saved != nil {
			results = append(results, *saved)
		}
	}

	return results, nil
}
