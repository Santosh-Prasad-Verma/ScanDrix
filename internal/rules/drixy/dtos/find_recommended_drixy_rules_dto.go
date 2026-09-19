// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Rule System
// File: find_recommended_drixy_rules_dto.go
// ═══════════════════════════════════════════════════════════════

package dtos

// FindRecommendedDrixyRulesDto specifies limits for recommendation synthesis.
type FindRecommendedDrixyRulesDto struct {
	Limit        int    `json:"limit,omitempty"`
	RepositoryID string `json:"repositoryId,omitempty"`
	Language     string `json:"language,omitempty"`
}

// FindSuggestionsByRuleDto is an alias for FindSuggestionsByRuleDTO.
type FindSuggestionsByRuleDto = FindSuggestionsByRuleDTO
