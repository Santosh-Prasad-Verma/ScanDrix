// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: find_library_drixy_rules.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"math"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
)

// FindLibraryDrixyRulesUseCase paginates through curated library rules.
type FindLibraryDrixyRulesUseCase struct {
	rulesService contracts.IDrixyRulesService
}

// NewFindLibraryDrixyRulesUseCase constructs a new use case instance.
func NewFindLibraryDrixyRulesUseCase(rulesService contracts.IDrixyRulesService) *FindLibraryDrixyRulesUseCase {
	return &FindLibraryDrixyRulesUseCase{rulesService: rulesService}
}

// Execute retrieves library rules with pagination.
func (uc *FindLibraryDrixyRulesUseCase) Execute(ctx context.Context, filters dtos.FindLibraryDrixyRulesDto) (*dtos.LibraryRulesResponse, error) {
	page := filters.Page
	if page <= 0 {
		page = 1
	}
	limit := filters.Limit
	if limit <= 0 {
		limit = 100
	}

	filterMap := map[string]any{
		"search":   filters.Search,
		"language": filters.Language,
		"scope":    filters.Scope,
	}

	allRules, err := uc.rulesService.GetLibraryDrixyRulesWithFeedback(ctx, "", filterMap, "")
	if err != nil {
		return nil, err
	}

	totalItems := len(allRules)
	totalPages := int(math.Ceil(float64(totalItems) / float64(limit)))
	offset := filters.Skip
	if offset == 0 {
		offset = (page - 1) * limit
	}

	var paginated []contracts.LibraryDrixyRule
	if offset < totalItems {
		end := offset + limit
		if end > totalItems {
			end = totalItems
		}
		paginated = allRules[offset:end]
	}

	return &dtos.LibraryRulesResponse{
		Data: paginated,
		Pagination: dtos.PaginationMetadata{
			CurrentPage:     page,
			TotalPages:      totalPages,
			TotalItems:      totalItems,
			ItemsPerPage:    limit,
			HasNextPage:     page < totalPages,
			HasPreviousPage: page > 1,
		},
	}, nil
}
