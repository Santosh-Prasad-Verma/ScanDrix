package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// DeliveryStatus tracks delivery state of inline review comments to the SCM.
type DeliveryStatus string

const (
	DeliveryStatusSent                DeliveryStatus = "sent"
	DeliveryStatusNotSent             DeliveryStatus = "not_sent"
	DeliveryStatusFailedLinesMismatch DeliveryStatus = "failed_lines_mismatch"
	DeliveryStatusFailed              DeliveryStatus = "failed"
	DeliveryStatusReplaced            DeliveryStatus = "replaced"
	DeliveryStatusDiscarded           DeliveryStatus = "discarded"
	DeliveryStatusSuppressed          DeliveryStatus = "suppressed"
	DeliveryStatusQueued              DeliveryStatus = "queued"
)

// PriorityStatus identifies prioritization outcome or discard rationale.
type PriorityStatus string

const (
	PriorityStatusPrioritized             PriorityStatus = "prioritized"
	PriorityStatusPrioritizedByClustering PriorityStatus = "prioritized-by-clustering"
	PriorityStatusReprioritized           PriorityStatus = "reprioritized"
	PriorityStatusDiscardedBySeverity     PriorityStatus = "discarded-by-severity"
	PriorityStatusDiscardedByQuantity     PriorityStatus = "discarded-by-quantity"
	PriorityStatusDiscardedByClustering   PriorityStatus = "discarded-by-clustering"
	PriorityStatusDiscardedBySafeguard    PriorityStatus = "discarded-by-safeguard"
	PriorityStatusDiscardedByCodeDiff     PriorityStatus = "discarded-by-code-diff"
	PriorityStatusDiscardedByFineTuning   PriorityStatus = "discarded-by-fine-tuning"
)

// ImplementationStatus tracks whether the PR author applied the suggestion.
type ImplementationStatus string

const (
	ImplStatusImplemented          ImplementationStatus = "implemented"
	ImplStatusNotImplemented       ImplementationStatus = "not_implemented"
	ImplStatusPartiallyImplemented ImplementationStatus = "partially_implemented"
	ImplStatusIgnored              ImplementationStatus = "ignored"

	ImplementationStatusImplemented = ImplStatusImplemented
	ImplementationStatusPending     = ImplStatusNotImplemented
)

// ClusteringType categorizes parent-child repeated suggestion hierarchies.
type ClusteringType string

const (
	ClusteringTypeParent ClusteringType = "parent"
	ClusteringTypeChild  ClusteringType = "child"
)

// ClusteringInfo captures grouped repeated patterns across a pull request.
type ClusteringInfo struct {
	Type                  ClusteringType `json:"type,omitempty"`
	RelatedSuggestionsIDs []string       `json:"relatedSuggestionsIds,omitempty"`
	ParentSuggestionID    string         `json:"parentSuggestionId,omitempty"`
	ProblemDescription    string         `json:"problemDescription,omitempty"`
	ActionStatement       string         `json:"actionStatement,omitempty"`
}

// ValidatedDiffData holds syntax-verified diff coordinates for git commit application.
type ValidatedDiffData struct {
	Code      string `json:"code"`
	Diff      string `json:"diff"`
	LineStart int    `json:"lineStart"`
	LineEnd   int    `json:"lineEnd"`
}

// SCMCommentRef links a code suggestion to the created remote inline comment.
type SCMCommentRef struct {
	ID                  int64  `json:"id"`
	PullRequestReviewID string `json:"pullRequestReviewId,omitempty"`
	PlatformCommentID   string `json:"platformCommentId,omitempty"`
}

// CodeSuggestion models actionable code review recommendations for pull requests.
type CodeSuggestion struct {
	ID                   uuid.UUID            `json:"id"`
	PullRequestID        string               `json:"pullRequestId"`
	PullNumber           int                  `json:"pullNumber"`
	RelevantFile         string               `json:"relevantFile"`
	Language             string               `json:"language"`
	SuggestionContent    string               `json:"suggestionContent"`
	ExistingCode         string               `json:"existingCode,omitempty"`
	ImprovedCode         string               `json:"improvedCode"`
	OneSentenceSummary   string               `json:"oneSentenceSummary,omitempty"`
	RelevantLinesStart   int                  `json:"relevantLinesStart"`
	RelevantLinesEnd     int                  `json:"relevantLinesEnd"`
	Label                string               `json:"label"`
	Severity             ReviewSeverity       `json:"severity"`
	Category             ReviewCategory       `json:"category"`
	RankScore            float64              `json:"rankScore"`
	PriorityScore        float64              `json:"priorityScore,omitempty"`
	Confidence           float64              `json:"confidence,omitempty"`
	DiscardReason        string               `json:"discardReason,omitempty"`
	RuleID               string               `json:"ruleId,omitempty"`
	FilePath             string               `json:"filePath,omitempty"`
	StartLine            int                  `json:"startLine,omitempty"`
	EndLine              int                  `json:"endLine,omitempty"`
	Description          string               `json:"description,omitempty"`
	SuggestedReplacement string               `json:"suggestedReplacement,omitempty"`
	Explanation          string               `json:"explanation,omitempty"`
	OriginalDiff         string               `json:"originalDiff,omitempty"`
	Fingerprint          string               `json:"fingerprint,omitempty"`
	PriorityStatus       PriorityStatus       `json:"priorityStatus"`
	DeliveryStatus       DeliveryStatus       `json:"deliveryStatus"`
	ImplementationStatus ImplementationStatus `json:"implementationStatus"`
	BrokenRuleIDs        []string             `json:"brokenRuleIds,omitempty"`
	Clustering           *ClusteringInfo      `json:"clusteringInformation,omitempty"`
	Comment              *SCMCommentRef       `json:"comment,omitempty"`
	ValidatedData        *ValidatedDiffData   `json:"validatedData,omitempty"`
	IsCommittable        bool                 `json:"isCommittable"`
	CreatedAt            time.Time            `json:"createdAt"`
	UpdatedAt            time.Time            `json:"updatedAt"`
}

// GetFilePath returns the target file path.
func (s CodeSuggestion) GetFilePath() string {
	if s.FilePath != "" {
		return s.FilePath
	}
	return s.RelevantFile
}

// GetStartLine returns the start line.
func (s CodeSuggestion) GetStartLine() int {
	if s.StartLine != 0 {
		return s.StartLine
	}
	return s.RelevantLinesStart
}

// GetEndLine returns the end line.
func (s CodeSuggestion) GetEndLine() int {
	if s.EndLine != 0 {
		return s.EndLine
	}
	return s.RelevantLinesEnd
}

// GetDescription returns the suggestion description.
func (s CodeSuggestion) GetDescription() string {
	if s.Description != "" {
		return s.Description
	}
	return s.SuggestionContent
}

// GetSuggestedReplacement returns the improved code replacement.
func (s CodeSuggestion) GetSuggestedReplacement() string {
	if s.SuggestedReplacement != "" {
		return s.SuggestedReplacement
	}
	return s.ImprovedCode
}

// GetExplanation returns the rationale or one-sentence summary.
func (s CodeSuggestion) GetExplanation() string {
	if s.Explanation != "" {
		return s.Explanation
	}
	if s.OneSentenceSummary != "" {
		return s.OneSentenceSummary
	}
	return s.SuggestionContent
}

// GetOriginalDiff returns the existing code or diff context.
func (s CodeSuggestion) GetOriginalDiff() string {
	if s.OriginalDiff != "" {
		return s.OriginalDiff
	}
	return s.ExistingCode
}

// GetFingerprint calculates a stable SHA-256 fingerprint for deduplication.
func (s CodeSuggestion) GetFingerprint() string {
	if s.Fingerprint != "" {
		return s.Fingerprint
	}
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", s.GetFilePath(), s.GetStartLine(), s.GetSuggestedReplacement())))
	return hex.EncodeToString(h[:])
}

// SuggestionControlConfig defines limits and priority thresholds for a review run.
type SuggestionControlConfig struct {
	MaxSuggestions         int             `json:"maxSuggestions"`
	GroupingMode           string          `json:"groupingMode"` // "by_file", "by_severity", "flat"
	LimitationType         string          `json:"limitationType"`
	SeverityLevelFilter    ReviewSeverity  `json:"severityLevelFilter"`
	ApplyFiltersToRules    bool            `json:"applyFiltersToRules"`
	SeverityLimits         *SeverityLimits `json:"severityLimits,omitempty"`
}

// SeverityLimits sets maximum allowable suggestions per severity tier.
type SeverityLimits struct {
	Low      int `json:"low"`
	Medium   int `json:"medium"`
	High     int `json:"high"`
	Critical int `json:"critical"`
}

// SuggestionFilterResult contains categorized suggestions after filtering.
type SuggestionFilterResult struct {
	Prioritized []CodeSuggestion `json:"prioritized"`
	Discarded   []CodeSuggestion `json:"discarded"`
}

// ISuggestionService defines the full contract for suggestion lifecycle management.
type ISuggestionService interface {
	PrioritizeSuggestions(ctx context.Context, config SuggestionControlConfig, suggestions []CodeSuggestion) (SuggestionFilterResult, error)
	FilterByCodeDiff(suggestions []CodeSuggestion, patchWithLines map[string][]int) []CodeSuggestion
	AnalyzeSeverity(ctx context.Context, suggestions []CodeSuggestion) ([]CodeSuggestion, error)
	ValidateImplementedSuggestions(ctx context.Context, prNumber int, patch string, savedSuggestions []CodeSuggestion) ([]CodeSuggestion, error)
}

// CalculatePriorityScore computes an objective rank score for a code review suggestion.
func CalculatePriorityScore(severity ReviewSeverity, category ReviewCategory) float64 {
	baseScore := 25.0
	switch severity {
	case SeverityCritical:
		baseScore = 100.0
	case SeverityMajor:
		baseScore = 75.0
	case SeverityMinor:
		baseScore = 50.0
	case SeverityInfo:
		baseScore = 25.0
	}

	multiplier := 1.0
	switch category {
	case CategorySecurity:
		multiplier = 1.5
	case CategoryBug:
		multiplier = 1.3
	case CategoryPerformance:
		multiplier = 1.2
	case CategoryBusinessLogic:
		multiplier = 1.2
	case CategoryArchitecture:
		multiplier = 1.1
	case CategoryRules:
		multiplier = 1.0
	}

	return baseScore * multiplier
}

// PrioritizeSuggestionsByScore sorts suggestions descending by calculated rank score and enforces limits.
func PrioritizeSuggestionsByScore(suggestions []CodeSuggestion, maxSuggestions int, limits *SeverityLimits) SuggestionFilterResult {
	if len(suggestions) == 0 {
		return SuggestionFilterResult{}
	}

	for i := range suggestions {
		if suggestions[i].RankScore == 0 {
			suggestions[i].RankScore = CalculatePriorityScore(suggestions[i].Severity, suggestions[i].Category)
		}
	}

	sort.SliceStable(suggestions, func(i, j int) bool {
		return suggestions[i].RankScore > suggestions[j].RankScore
	})

	var prioritized []CodeSuggestion
	var discarded []CodeSuggestion

	counts := map[ReviewSeverity]int{
		SeverityCritical: 0,
		SeverityMajor:    0,
		SeverityMinor:    0,
		SeverityInfo:     0,
	}

	for _, s := range suggestions {
		if maxSuggestions > 0 && len(prioritized) >= maxSuggestions {
			s.PriorityStatus = PriorityStatusDiscardedByQuantity
			discarded = append(discarded, s)
			continue
		}

		if limits != nil {
			var limit int
			switch s.Severity {
			case SeverityCritical:
				limit = limits.Critical
			case SeverityMajor:
				limit = limits.High
			case SeverityMinor:
				limit = limits.Medium
			case SeverityInfo:
				limit = limits.Low
			}
			if limit > 0 && counts[s.Severity] >= limit {
				s.PriorityStatus = PriorityStatusDiscardedBySeverity
				discarded = append(discarded, s)
				continue
			}
		}

		counts[s.Severity]++
		s.PriorityStatus = PriorityStatusPrioritized
		prioritized = append(prioritized, s)
	}

	return SuggestionFilterResult{
		Prioritized: prioritized,
		Discarded:   discarded,
	}
}

// FilterSuggestionsByDiffLines validates that each suggestion falls within lines modified by the patch.
func FilterSuggestionsByDiffLines(suggestions []CodeSuggestion, changedLinesByFile map[string][]int) (valid []CodeSuggestion, discarded []CodeSuggestion) {
	for _, s := range suggestions {
		changedLines, exists := changedLinesByFile[s.RelevantFile]
		if !exists || len(changedLines) == 0 {
			s.PriorityStatus = PriorityStatusDiscardedByCodeDiff
			discarded = append(discarded, s)
			continue
		}

		overlaps := false
		for _, line := range changedLines {
			if line >= s.RelevantLinesStart && line <= s.RelevantLinesEnd {
				overlaps = true
				break
			}
		}

		if overlaps {
			valid = append(valid, s)
		} else {
			s.PriorityStatus = PriorityStatusDiscardedByCodeDiff
			discarded = append(discarded, s)
		}
	}
	return valid, discarded
}

// ConvertFindingToSuggestion maps an internal static/agent finding into an actionable CodeSuggestion.
func ConvertFindingToSuggestion(finding models.CodeFinding, pullRequestID string, pullNumber int) CodeSuggestion {
	sev := SeverityMinor
	switch strings.ToUpper(string(finding.Severity)) {
	case "CRITICAL":
		sev = SeverityCritical
	case "HIGH", "MAJOR":
		sev = SeverityMajor
	case "MEDIUM", "MODERATE":
		sev = SeverityMinor
	case "LOW", "INFO":
		sev = SeverityInfo
	}

	cat := CategoryBug
	switch strings.ToLower(finding.Category) {
	case "security":
		cat = CategorySecurity
	case "performance":
		cat = CategoryPerformance
	case "architecture":
		cat = CategoryArchitecture
	case "rules":
		cat = CategoryRules
	case "business_logic":
		cat = CategoryBusinessLogic
	}

	s := CodeSuggestion{
		ID:                 uuid.New(),
		PullRequestID:      pullRequestID,
		PullNumber:         pullNumber,
		RelevantFile:       finding.FilePath,
		Language:           "",
		SuggestionContent:  finding.Description,
		ExistingCode:       "",
		ImprovedCode:       finding.SuggestedDiff,
		OneSentenceSummary: finding.Title,
		RelevantLinesStart: finding.StartLine,
		RelevantLinesEnd:   finding.EndLine,
		Label:              finding.Fingerprint,
		Severity:           sev,
		Category:           cat,
		PriorityStatus:     PriorityStatusPrioritized,
		DeliveryStatus:     DeliveryStatusNotSent,
		IsCommittable:      finding.SuggestedDiff != "",
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	s.RankScore = CalculatePriorityScore(s.Severity, s.Category)
	return s
}
