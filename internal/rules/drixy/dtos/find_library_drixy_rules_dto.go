// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: find_library_drixy_rules_dto.go
// ═══════════════════════════════════════════════════════════════

package dtos

// FindLibraryDrixyRulesDto defines search and filter queries for the rules library.
type FindLibraryDrixyRulesDto struct {
	Page     int      `json:"page"`
	Limit    int      `json:"limit"`
	Skip     int      `json:"skip,omitempty"`
	Search   string   `json:"search,omitempty"`
	Language string   `json:"language,omitempty"`
	Buckets  []string `json:"buckets,omitempty"`
	Scope    string   `json:"scope,omitempty"`
	Severity string   `json:"severity,omitempty"`
}

// PaginationMetadata provides pagination details.
type PaginationMetadata struct {
	CurrentPage     int  `json:"currentPage"`
	TotalPages      int  `json:"totalPages"`
	TotalItems      int  `json:"totalItems"`
	ItemsPerPage    int  `json:"itemsPerPage"`
	HasNextPage     bool `json:"hasNextPage"`
	HasPreviousPage bool `json:"hasPreviousPage"`
}
