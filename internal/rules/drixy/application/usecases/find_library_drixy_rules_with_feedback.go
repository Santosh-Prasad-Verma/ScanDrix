// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: find_library_drixy_rules_with_feedback.go
// ═══════════════════════════════════════════════════════════════

package usecases

import (
	"context"
	"math"

	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/dtos"
)

// PaginatedLibraryDrixyRulesResponse wraps paginated library rules output.
type PaginatedLibraryDrixyRulesResponse struct {
	Data       []contracts.LibraryDrixyRule `json:"data"`
	Pagination dtos.PaginationMetadata      `json:"pagination"`
}

// FindLibraryDrixyRulesWithFeedbackUseCase queries catalog rules enriched with voter feedback.
type FindLibraryDrixyRulesWithFeedbackUseCase struct {
	rulesService contracts.IDrixyRulesService
}

// NewFindLibraryDrixyRulesWithFeedbackUseCase constructs the usecase.
func NewFindLibraryDrixyRulesWithFeedbackUseCase(
	rulesService contracts.IDrixyRulesService,
) *FindLibraryDrixyRulesWithFeedbackUseCase {
	return &FindLibraryDrixyRulesWithFeedbackUseCase{
		rulesService: rulesService,
	}
}

// Execute retrieves library rules with user feedback and computes pagination.
func (uc *FindLibraryDrixyRulesWithFeedbackUseCase) Execute(
	ctx context.Context,
	organizationID string,
	filters dtos.FindLibraryDrixyRulesDto,
	userID string,
) (*PaginatedLibraryDrixyRulesResponse, error) {
	page := filters.Page
	if page <= 0 {
		page = 1
	}
	limit := filters.Limit
	if limit <= 0 {
		limit = 100
	}

	filterMap := make(map[string]any)
	if filters.Search != "" {
		filterMap["search"] = filters.Search
	}
	if filters.Language != "" {
		filterMap["language"] = filters.Language
	}
	if len(filters.Buckets) > 0 {
		filterMap["buckets"] = filters.Buckets
	}
	if filters.Scope != "" {
		filterMap["scope"] = filters.Scope
	}
	if filters.Severity != "" {
		filterMap["severity"] = filters.Severity
	}

	allRules, err := uc.rulesService.GetLibraryDrixyRulesWithFeedback(ctx, organizationID, filterMap, userID)
	if err != nil {
		return nil, err
	}

	totalItems := len(allRules)
	totalPages := int(math.Ceil(float64(totalItems) / float64(limit)))

	offset := filters.Skip
	if offset <= 0 {
		offset = (page - 1) * limit
	}

	paginatedRules := make([]contracts.LibraryDrixyRule, 0)
	if offset < totalItems {
		end := offset + limit
		if end > totalItems {
			end = totalItems
		}
		paginatedRules = allRules[offset:end]
	}

	pagination := dtos.PaginationMetadata{
		CurrentPage:     page,
		TotalPages:      totalPages,
		TotalItems:      totalItems,
		ItemsPerPage:    limit,
		HasNextPage:     page < totalPages,
		HasPreviousPage: page > 1,
	}

	return &PaginatedLibraryDrixyRulesResponse{
		Data:       paginatedRules,
		Pagination: pagination,
	}, nil
}
