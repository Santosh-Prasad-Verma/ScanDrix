// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Fine-Tuning Subsystem
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package suggestionembedded

// SuggestionEmbeddedEntity is the domain entity representing an embedded suggestion.
type SuggestionEmbeddedEntity struct {
	uuid               string
	suggestionID       string
	suggestionEmbed    []float64
	pullRequestNumber  int
	repositoryID       string
	repositoryFullName string
	organization       *OrganizationRef
	label              string
	severity           string
	feedbackType       string
	improvedCode       string
	suggestionContent  string
	oneSentenceSummary string
	language           string
}

// NewSuggestionEmbeddedEntity constructs an initialized domain entity.
func NewSuggestionEmbeddedEntity(data SuggestionEmbedded) *SuggestionEmbeddedEntity {
	embedCopy := make([]float64, len(data.SuggestionEmbed))
	copy(embedCopy, data.SuggestionEmbed)

	var orgCopy *OrganizationRef
	if data.Organization != nil {
		orgCopy = &OrganizationRef{UUID: data.Organization.UUID}
	}

	return &SuggestionEmbeddedEntity{
		uuid:               data.UUID,
		suggestionID:       data.SuggestionID,
		suggestionEmbed:    embedCopy,
		pullRequestNumber:  data.PullRequestNumber,
		repositoryID:       data.RepositoryID,
		repositoryFullName: data.RepositoryFullName,
		organization:       orgCopy,
		label:              data.Label,
		severity:           data.Severity,
		feedbackType:       data.FeedbackType,
		improvedCode:       data.ImprovedCode,
		suggestionContent:  data.SuggestionContent,
		oneSentenceSummary: data.OneSentenceSummary,
		language:           data.Language,
	}
}

// Getters
func (e *SuggestionEmbeddedEntity) UUID() string               { return e.uuid }
func (e *SuggestionEmbeddedEntity) SuggestionID() string       { return e.suggestionID }
func (e *SuggestionEmbeddedEntity) SuggestionEmbed() []float64 { return e.suggestionEmbed }
func (e *SuggestionEmbeddedEntity) PullRequestNumber() int     { return e.pullRequestNumber }
func (e *SuggestionEmbeddedEntity) RepositoryID() string       { return e.repositoryID }
func (e *SuggestionEmbeddedEntity) RepositoryFullName() string { return e.repositoryFullName }
func (e *SuggestionEmbeddedEntity) Organization() *OrganizationRef {
	if e.organization == nil {
		return nil
	}
	return &OrganizationRef{UUID: e.organization.UUID}
}
func (e *SuggestionEmbeddedEntity) Label() string              { return e.label }
func (e *SuggestionEmbeddedEntity) Severity() string           { return e.severity }
func (e *SuggestionEmbeddedEntity) FeedbackType() string       { return e.feedbackType }
func (e *SuggestionEmbeddedEntity) ImprovedCode() string       { return e.improvedCode }
func (e *SuggestionEmbeddedEntity) SuggestionContent() string  { return e.suggestionContent }
func (e *SuggestionEmbeddedEntity) OneSentenceSummary() string { return e.oneSentenceSummary }
func (e *SuggestionEmbeddedEntity) Language() string           { return e.language }

// ToObject serializes entity back to domain DTO.
func (e *SuggestionEmbeddedEntity) ToObject() SuggestionEmbedded {
	embedCopy := make([]float64, len(e.suggestionEmbed))
	copy(embedCopy, e.suggestionEmbed)

	var orgCopy *OrganizationRef
	if e.organization != nil {
		orgCopy = &OrganizationRef{UUID: e.organization.UUID}
	}

	return SuggestionEmbedded{
		UUID:               e.uuid,
		SuggestionID:       e.suggestionID,
		SuggestionEmbed:    embedCopy,
		PullRequestNumber:  e.pullRequestNumber,
		RepositoryID:       e.repositoryID,
		RepositoryFullName: e.repositoryFullName,
		Organization:       orgCopy,
		Label:              e.label,
		Severity:           e.severity,
		FeedbackType:       e.feedbackType,
		ImprovedCode:       e.improvedCode,
		SuggestionContent:  e.suggestionContent,
		OneSentenceSummary: e.oneSentenceSummary,
		Language:           e.language,
	}
}
