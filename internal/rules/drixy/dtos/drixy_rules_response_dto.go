// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: drixy_rules_response_dto.go
// ═══════════════════════════════════════════════════════════════

package dtos

import (
	"github.com/scandrix/backend/internal/rules/drixy/domain/contracts"
	"github.com/scandrix/backend/internal/rules/drixy/domain/interfaces"
)

// DrixyRulesResponse wraps a single rule response.
type DrixyRulesResponse struct {
	Success bool                  `json:"success"`
	Data    *interfaces.DrixyRule `json:"data,omitempty"`
	Message string                `json:"message,omitempty"`
}

// DrixyRulesListResponse wraps a list of rules.
type DrixyRulesListResponse struct {
	Success bool                   `json:"success"`
	Data    []interfaces.DrixyRule `json:"data"`
	Total   int                    `json:"total"`
}

// PendingRulesResponse details pending rules with breakdown counts.
type PendingRulesResponse struct {
	Items  []interfaces.DrixyRule `json:"items"`
	Counts struct {
		Total    int `json:"total"`
		Rules    int `json:"rules"`
		Memories int `json:"memories"`
	} `json:"counts"`
}

// LibraryRulesResponse wraps paginated library query results.
type LibraryRulesResponse struct {
	Data       []contracts.LibraryDrixyRule `json:"data"`
	Pagination PaginationMetadata           `json:"pagination"`
}

// RulesLimitStatusResponse reports plan quota usage.
type RulesLimitStatusResponse struct {
	Total     int  `json:"total"`
	Active    int  `json:"active"`
	Limit     *int `json:"limit"`
	Remaining *int `json:"remaining"`
}

// SyncStatusResponse reports feature initialization status.
type SyncStatusResponse struct {
	IDERulesSyncEnabledFirstTime        bool `json:"ideRulesSyncEnabledFirstTime"`
	DrixyRulesGeneratorEnabledFirstTime bool `json:"drixyRulesGeneratorEnabledFirstTime"`
}

// InheritedRulesResponse models global, repo, and directory inherited rules.
type InheritedRulesResponse struct {
	GlobalRules    []interfaces.DrixyRule `json:"globalRules"`
	RepoRules      []interfaces.DrixyRule `json:"repoRules"`
	DirectoryRules []interfaces.DrixyRule `json:"directoryRules"`
}
