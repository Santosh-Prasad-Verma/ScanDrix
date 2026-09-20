package usecases

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/pkg/models"
)

// PRFileChange represents a file modified in a pull request.
type PRFileChange struct {
	Filename    string `json:"filename"`
	Status      string `json:"status"` // added, modified, removed
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	Patch       string `json:"patch,omitempty"`
	FileContent string `json:"fileContent,omitempty"`
}

// IPullRequestDataProvider fetches PR diff and metadata from SCM.
type IPullRequestDataProvider interface {
	GetChangedFiles(ctx context.Context, repo models.TrackedRepository, prNumber int) ([]PRFileChange, error)
	GetSavedSuggestions(ctx context.Context, orgID string, prNumber int) ([]domain.CodeSuggestion, error)
}

// PRUseCases coordinates PR files, suggestion queries, and summary generation.
type PRUseCases struct {
	provider   IPullRequestDataProvider
	templates  domain.IMessageTemplateProcessor
}

// NewPRUseCases creates a new PR use case coordinator.
func NewPRUseCases(provider IPullRequestDataProvider, templates domain.IMessageTemplateProcessor) *PRUseCases {
	return &PRUseCases{
		provider:  provider,
		templates: templates,
	}
}

// GetPullRequestFiles retrieves the changed files list with diff stats.
func (u *PRUseCases) GetPullRequestFiles(ctx context.Context, repo models.TrackedRepository, prNumber int) ([]PRFileChange, error) {
	if prNumber <= 0 {
		return nil, fmt.Errorf("invalid pull request number: %d", prNumber)
	}
	return u.provider.GetChangedFiles(ctx, repo, prNumber)
}

// PRSuggestionsQuery filters for retrieving suggestions.
type PRSuggestionsQuery struct {
	OrganizationID string
	PRNumber       int
	Format         string // "json" or "markdown"
	Severity       string // comma-separated
	Category       string // comma-separated
}

// PRSuggestionsResult holds formatted or raw suggestions.
type PRSuggestionsResult struct {
	Count       int                      `json:"count"`
	Suggestions []domain.CodeSuggestion  `json:"suggestions,omitempty"`
	Markdown    string                   `json:"markdown,omitempty"`
}

// GetPullRequestSuggestions queries and filters delivered suggestions for a pull request.
func (u *PRUseCases) GetPullRequestSuggestions(ctx context.Context, query PRSuggestionsQuery) (*PRSuggestionsResult, error) {
	all, err := u.provider.GetSavedSuggestions(ctx, query.OrganizationID, query.PRNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch saved suggestions: %w", err)
	}

	sevSet := make(map[string]bool)
	if query.Severity != "" {
		for _, s := range strings.Split(query.Severity, ",") {
			sevSet[strings.ToUpper(strings.TrimSpace(s))] = true
		}
	}

	catSet := make(map[string]bool)
	if query.Category != "" {
		for _, c := range strings.Split(query.Category, ",") {
			catSet[strings.ToUpper(strings.TrimSpace(c))] = true
		}
	}

	var filtered []domain.CodeSuggestion
	for _, s := range all {
		if len(sevSet) > 0 && !sevSet[string(s.Severity)] {
			continue
		}
		if len(catSet) > 0 && !catSet[string(s.Category)] {
			continue
		}
		filtered = append(filtered, s)
	}

	if strings.ToLower(query.Format) == "markdown" {
		var md strings.Builder
		md.WriteString(fmt.Sprintf("## 🔍 ScanDrix Review Findings (%d)\n\n", len(filtered)))
		for i, s := range filtered {
			md.WriteString(fmt.Sprintf("### %d. [%s] %s (`%s` L%d-L%d)\n\n",
				i+1, s.Severity, s.OneSentenceSummary, s.RelevantFile, s.RelevantLinesStart, s.RelevantLinesEnd))
			md.WriteString(s.SuggestionContent + "\n\n")
			if s.ImprovedCode != "" {
				md.WriteString("```suggestion\n" + s.ImprovedCode + "\n```\n\n")
			}
		}
		return &PRSuggestionsResult{
			Count:    len(filtered),
			Markdown: md.String(),
		}, nil
	}

	return &PRSuggestionsResult{
		Count:       len(filtered),
		Suggestions: filtered,
	}, nil
}

// PreviewPRSummary produces a formatted preview of the overall summary comment.
func (u *PRUseCases) PreviewPRSummary(ctx context.Context, author, repoName string, prNumber int, suggestions []domain.CodeSuggestion, customTemplate string) string {
	if customTemplate == "" {
		customTemplate = domain.DefaultEndReviewTemplate().Content
	}

	var findingsList strings.Builder
	for _, s := range suggestions {
		findingsList.WriteString(fmt.Sprintf("- **[%s]** `%s`:%d — %s\n", s.Severity, s.RelevantFile, s.RelevantLinesStart, s.OneSentenceSummary))
	}

	summaryText := fmt.Sprintf("Reviewed pull request #%d for %s. Evaluated %d candidate findings.", prNumber, author, len(suggestions))

	vars := domain.TemplateVariables{
		Author:        author,
		PRNumber:      prNumber,
		RepoName:      repoName,
		Summary:       summaryText,
		FindingsCount: len(suggestions),
		FindingsList:  findingsList.String(),
		RulesChecked:  len(suggestions),
		DurationSecs:  1.5,
	}

	if u.templates != nil {
		return u.templates.Process(customTemplate, vars)
	}

	// Default fallback processor
	res := customTemplate
	res = strings.ReplaceAll(res, "{{author}}", vars.Author)
	res = strings.ReplaceAll(res, "{{summary}}", vars.Summary)
	res = strings.ReplaceAll(res, "{{findings}}", vars.FindingsList)
	return res
}
