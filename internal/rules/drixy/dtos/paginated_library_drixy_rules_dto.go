// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: paginated_library_drixy_rules_dto.go
// ═══════════════════════════════════════════════════════════════

package dtos

import "github.com/scandrix/backend/internal/rules/drixy/domain/contracts"

// DrixyPaginationMetadata models standard API response pagination headers.
type DrixyPaginationMetadata struct {
	CurrentPage  int  `json:"currentPage"`
	TotalPages   int  `json:"totalPages"`
	TotalItems   int  `json:"totalItems"`
	ItemsPerPage int  `json:"itemsPerPage"`
	HasNextPage  bool `json:"hasNextPage"`
	HasPrevPage  bool `json:"hasPreviousPage"`
}

// PaginatedLibraryDrixyRulesResponse wraps catalog rules with page coordinates.
type PaginatedLibraryDrixyRulesResponse struct {
	Data       []contracts.LibraryDrixyRule `json:"data"`
	Pagination DrixyPaginationMetadata      `json:"pagination"`
}
