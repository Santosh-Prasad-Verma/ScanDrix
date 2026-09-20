// Package services provides production-grade infrastructure implementations for ScanDrix code review.
package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
)

// SuggestionRepository defines the persistence contract for code review suggestions.
type SuggestionRepository interface {
	SaveSuggestions(ctx context.Context, reviewID uuid.UUID, suggestions []domain.CodeSuggestion) error
	GetSuggestionsByReviewID(ctx context.Context, reviewID uuid.UUID) ([]domain.CodeSuggestion, error)
	GetSuggestionsByPR(ctx context.Context, repo string, pull int) ([]domain.CodeSuggestion, error)
	UpdateDeliveryStatus(ctx context.Context, suggestionID string, status domain.DeliveryStatus, reason string) error
	UpdateImplementationStatus(ctx context.Context, suggestionID string, status domain.ImplementationStatus) error
	RecordSuggestionFeedback(ctx context.Context, feedback domain.CodeReviewFeedback) error
}

// SeverityLimitsConfig defines caps on suggestions by severity level.
type SeverityLimitsConfig struct {
	MaxCritical int `json:"max_critical"`
	MaxMajor    int `json:"max_major"`
	MaxMinor    int `json:"max_minor"`
	MaxInfo     int `json:"max_info"`
	MaxTotal    int `json:"max_total"`
}

// DefaultSeverityLimits provides sensible enterprise defaults.
func DefaultSeverityLimits() SeverityLimitsConfig {
	return SeverityLimitsConfig{
		MaxCritical: 25,
		MaxMajor:    15,
		MaxMinor:    10,
		MaxInfo:     5,
		MaxTotal:    30,
	}
}

// DiffLineHunk records added/deleted line ranges for diff boundary checks.
type DiffLineHunk struct {
	FilePath  string `json:"file_path"`
	OldStart  int    `json:"old_start"`
	OldLength int    `json:"old_length"`
	NewStart  int    `json:"new_start"`
	NewLength int    `json:"new_length"`
}

// LineDelta records line shifts caused by commits between review runs.
type LineDelta struct {
	FilePath   string `json:"file_path"`
	LineNumber int    `json:"line_number"`
	Delta      int    `json:"delta"`
}

// SuggestionCluster groups related findings across lines or files.
type SuggestionCluster struct {
	ClusterID   string                 `json:"cluster_id"`
	PrimaryID   string                 `json:"primary_id"`
	Category    domain.ReviewCategory  `json:"category"`
	Severity    domain.ReviewSeverity  `json:"severity"`
	Suggestions []domain.CodeSuggestion `json:"suggestions"`
	Summary     string                 `json:"summary"`
}

// DeepSuggestionService manages the complete lifecycle, filtering, clustering, and prioritization of code suggestions.
type DeepSuggestionService struct {
	mu         sync.RWMutex
	repo       SuggestionRepository
	safeguards *SafeguardPipelineEngine
}

// NewDeepSuggestionService initializes the suggestion management engine.
func NewDeepSuggestionService(repo SuggestionRepository, safeguards *SafeguardPipelineEngine) *DeepSuggestionService {
	return &DeepSuggestionService{
		repo:       repo,
		safeguards: safeguards,
	}
}

// CalculateSuggestionRankScore produces a deterministic, weighted priority score.
// Score = SeverityWeight(0.40) + Confidence(0.30) + ComplexityWeight(0.15) + Centrality(0.15)
func (s *DeepSuggestionService) CalculateSuggestionRankScore(sug domain.CodeSuggestion) float64 {
	sevWeight := 0.2
	switch sug.Severity {
	case domain.SeverityCritical:
		sevWeight = 1.0
	case domain.SeverityMajor:
		sevWeight = 0.75
	case domain.SeverityMinor:
		sevWeight = 0.45
	case domain.SeverityInfo:
		sevWeight = 0.2
	}

	conf := sug.Confidence
	if conf <= 0 {
		conf = 0.5
	} else if conf > 1.0 {
		conf = 1.0
	}

	catWeight := 0.5
	switch sug.Category {
	case domain.CategorySecurity:
		catWeight = 1.0
	case domain.CategoryBug:
		catWeight = 0.85
	case domain.CategoryPerformance:
		catWeight = 0.7
	case domain.CategoryArchitecture:
		catWeight = 0.65
	case domain.CategoryRules:
		catWeight = 0.6
	case domain.CategoryStyle:
		catWeight = 0.2
	}

	linesSpan := float64(sug.EndLine - sug.StartLine + 1)
	sizeFactor := 1.0 / (1.0 + math.Log1p(math.Max(0, linesSpan-1)))

	score := (sevWeight * 0.40) + (conf * 0.30) + (catWeight * 0.15) + (sizeFactor * 0.15)
	return math.Round(score*1000) / 1000
}

// FilterSuggestionsByReviewOptions eliminates findings violating review scope or paths.
func (s *DeepSuggestionService) FilterSuggestionsByReviewOptions(
	suggestions []domain.CodeSuggestion,
	config domain.CodeReviewConfig,
) ([]domain.CodeSuggestion, []domain.CodeSuggestion) {
	var kept []domain.CodeSuggestion
	var discarded []domain.CodeSuggestion

	for _, sug := range suggestions {
		// Ignore patterns check
		if config.MatchesPathPatterns(sug.GetFilePath(), config.IgnorePatterns) {
			sug.DeliveryStatus = domain.DeliveryStatusDiscarded
			sug.DiscardReason = "file matches review ignore pattern"
			discarded = append(discarded, sug)
			continue
		}

		// Strictness threshold check
		if config.Strictness == domain.StrictnessStrict {
			// In strict mode, keep all severities
		} else if config.Strictness == domain.StrictnessBalanced {
			if sug.Severity == domain.SeverityInfo && sug.Category == domain.CategoryStyle {
				sug.DeliveryStatus = domain.DeliveryStatusDiscarded
				sug.DiscardReason = "style info finding suppressed in balanced mode"
				discarded = append(discarded, sug)
				continue
			}
		} else if config.Strictness == domain.StrictnessPermissive {
			if sug.Severity == domain.SeverityMinor || sug.Severity == domain.SeverityInfo {
				sug.DeliveryStatus = domain.DeliveryStatusDiscarded
				sug.DiscardReason = "minor/info finding suppressed in permissive mode"
				discarded = append(discarded, sug)
				continue
			}
		}

		kept = append(kept, sug)
	}

	return kept, discarded
}

// FilterSuggestionsCodeDiff validates that suggestions fall strictly within modified or added diff lines.
func (s *DeepSuggestionService) FilterSuggestionsCodeDiff(
	suggestions []domain.CodeSuggestion,
	hunks []DiffLineHunk,
) ([]domain.CodeSuggestion, []domain.CodeSuggestion) {
	fileHunks := make(map[string][]DiffLineHunk)
	for _, h := range hunks {
		fileHunks[h.FilePath] = append(fileHunks[h.FilePath], h)
	}

	var kept []domain.CodeSuggestion
	var discarded []domain.CodeSuggestion

	for _, sug := range suggestions {
		hList, hasFile := fileHunks[sug.FilePath]
		if !hasFile {
			sug.DeliveryStatus = domain.DeliveryStatusDiscarded
			sug.DiscardReason = fmt.Sprintf("file %s not present in modified diff", sug.FilePath)
			discarded = append(discarded, sug)
			continue
		}

		isWithinHunk := false
		for _, h := range hList {
			hunkEnd := h.NewStart + h.NewLength - 1
			if h.NewLength == 0 {
				hunkEnd = h.NewStart
			}
			// Tolerant boundary: allow up to 2 context lines for multi-line replacements
			if sug.StartLine >= (h.NewStart-2) && sug.EndLine <= (hunkEnd+2) {
				isWithinHunk = true
				break
			}
		}

		if !isWithinHunk {
			sug.DeliveryStatus = domain.DeliveryStatusDiscarded
			sug.DiscardReason = fmt.Sprintf("suggestion line range [%d-%d] outside modified diff hunks", sug.StartLine, sug.EndLine)
			discarded = append(discarded, sug)
		} else {
			kept = append(kept, sug)
		}
	}

	return kept, discarded
}

// PrioritizeSuggestionsBySeverityLimits applies per-severity quotas and maximum review caps.
func (s *DeepSuggestionService) PrioritizeSuggestionsBySeverityLimits(
	suggestions []domain.CodeSuggestion,
	limits SeverityLimitsConfig,
) ([]domain.CodeSuggestion, []domain.CodeSuggestion) {
	// Group suggestions by severity
	bySev := make(map[domain.ReviewSeverity][]domain.CodeSuggestion)
	for _, sug := range suggestions {
		if sug.PriorityScore <= 0 {
			sug.PriorityScore = s.CalculateSuggestionRankScore(sug)
		}
		bySev[sug.Severity] = append(bySev[sug.Severity], sug)
	}

	// Sort each severity bucket descending by PriorityScore
	for sev := range bySev {
		sort.Slice(bySev[sev], func(i, j int) bool {
			return bySev[sev][i].PriorityScore > bySev[sev][j].PriorityScore
		})
	}

	var prioritized []domain.CodeSuggestion
	var discarded []domain.CodeSuggestion

	appendWithinLimit := func(sev domain.ReviewSeverity, max int) {
		list := bySev[sev]
		for i, sug := range list {
			if i < max && len(prioritized) < limits.MaxTotal {
				prioritized = append(prioritized, sug)
			} else {
				sug.DeliveryStatus = domain.DeliveryStatusDiscarded
				sug.DiscardReason = fmt.Sprintf("exceeded severity quota for %s (limit: %d)", sev, max)
				discarded = append(discarded, sug)
			}
		}
	}

	// High-to-low severity prioritization
	appendWithinLimit(domain.SeverityCritical, limits.MaxCritical)
	appendWithinLimit(domain.SeverityMajor, limits.MaxMajor)
	appendWithinLimit(domain.SeverityMinor, limits.MaxMinor)
	appendWithinLimit(domain.SeverityInfo, limits.MaxInfo)

	return prioritized, discarded
}

// SortSuggestionsByFilePathAndSeverity sorts findings logically for human review.
func (s *DeepSuggestionService) SortSuggestionsByFilePathAndSeverity(suggestions []domain.CodeSuggestion) []domain.CodeSuggestion {
	res := make([]domain.CodeSuggestion, len(suggestions))
	copy(res, suggestions)

	sevRank := map[domain.ReviewSeverity]int{
		domain.SeverityCritical: 4,
		domain.SeverityMajor:    3,
		domain.SeverityMinor:    2,
		domain.SeverityInfo:     1,
	}

	sort.Slice(res, func(i, j int) bool {
		if res[i].GetFilePath() != res[j].GetFilePath() {
			return res[i].GetFilePath() < res[j].GetFilePath()
		}
		if res[i].GetStartLine() != res[j].GetStartLine() {
			return res[i].GetStartLine() < res[j].GetStartLine()
		}
		return sevRank[res[i].Severity] > sevRank[res[j].Severity]
	})

	return res
}

// ClusterRelatedSuggestions performs spatial and category clustering to group duplicate or co-located findings.
func (s *DeepSuggestionService) ClusterRelatedSuggestions(
	suggestions []domain.CodeSuggestion,
	lineThreshold int,
) []SuggestionCluster {
	if lineThreshold <= 0 {
		lineThreshold = 10
	}

	byFileCat := make(map[string][]domain.CodeSuggestion)
	for _, sug := range suggestions {
		key := fmt.Sprintf("%s::%s", sug.GetFilePath(), sug.Category)
		byFileCat[key] = append(byFileCat[key], sug)
	}

	var clusters []SuggestionCluster

	for _, group := range byFileCat {
		sort.Slice(group, func(i, j int) bool {
			return group[i].GetStartLine() < group[j].GetStartLine()
		})

		var currentCluster []domain.CodeSuggestion
		for _, sug := range group {
			if len(currentCluster) == 0 {
				currentCluster = append(currentCluster, sug)
				continue
			}

			last := currentCluster[len(currentCluster)-1]
			// If within line proximity threshold and same category, cluster together
			if (sug.GetStartLine() - last.GetEndLine()) <= lineThreshold {
				currentCluster = append(currentCluster, sug)
			} else {
				clusters = append(clusters, buildCluster(currentCluster))
				currentCluster = []domain.CodeSuggestion{sug}
			}
		}

		if len(currentCluster) > 0 {
			clusters = append(clusters, buildCluster(currentCluster))
		}
	}

	return clusters
}

func buildCluster(sugs []domain.CodeSuggestion) SuggestionCluster {
	highestSev := domain.SeverityInfo
	sevRank := map[domain.ReviewSeverity]int{
		domain.SeverityCritical: 4,
		domain.SeverityMajor:    3,
		domain.SeverityMinor:    2,
		domain.SeverityInfo:     1,
	}

	primaryID := sugs[0].ID
	maxScore := -1.0

	for _, s := range sugs {
		if sevRank[s.Severity] > sevRank[highestSev] {
			highestSev = s.Severity
		}
		if s.PriorityScore > maxScore {
			maxScore = s.PriorityScore
			primaryID = s.ID
		}
	}

	return SuggestionCluster{
		ClusterID:   uuid.New().String(),
		PrimaryID:   primaryID.String(),
		Category:    sugs[0].Category,
		Severity:    highestSev,
		Suggestions: sugs,
		Summary:     fmt.Sprintf("%d related %s findings in %s around line %d", len(sugs), sugs[0].Category, sugs[0].GetFilePath(), sugs[0].GetStartLine()),
	}
}

// RebaseSuggestionLineNumbers shifts line numbers of suggestions when new commits modify lines above them.
func (s *DeepSuggestionService) RebaseSuggestionLineNumbers(
	suggestions []domain.CodeSuggestion,
	deltas []LineDelta,
) []domain.CodeSuggestion {
	deltasByFile := make(map[string][]LineDelta)
	for _, d := range deltas {
		deltasByFile[d.FilePath] = append(deltasByFile[d.FilePath], d)
	}

	for f := range deltasByFile {
		sort.Slice(deltasByFile[f], func(i, j int) bool {
			return deltasByFile[f][i].LineNumber < deltasByFile[f][j].LineNumber
		})
	}

	res := make([]domain.CodeSuggestion, len(suggestions))
	for i, sug := range suggestions {
		copySug := sug
		fileDeltas := deltasByFile[sug.GetFilePath()]
		accumShift := 0

		for _, d := range fileDeltas {
			if d.LineNumber < sug.GetStartLine() {
				accumShift += d.Delta
			}
		}

		copySug.StartLine = max(1, sug.GetStartLine()+accumShift)
		copySug.EndLine = max(copySug.StartLine, sug.GetEndLine()+accumShift)
		copySug.RelevantLinesStart = copySug.StartLine
		copySug.RelevantLinesEnd = copySug.EndLine
		res[i] = copySug
	}

	return res
}

// DeduplicateByFingerprint eliminates exact duplicate findings.
func (s *DeepSuggestionService) DeduplicateByFingerprint(suggestions []domain.CodeSuggestion) []domain.CodeSuggestion {
	seen := make(map[string]bool)
	var unique []domain.CodeSuggestion

	for _, sug := range suggestions {
		fp := sug.GetFingerprint()
		if fp == "" {
			h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", sug.GetFilePath(), sug.GetStartLine(), sug.GetSuggestedReplacement())))
			fp = hex.EncodeToString(h[:])
			sug.Fingerprint = fp
		}

		if !seen[fp] {
			seen[fp] = true
			unique = append(unique, sug)
		}
	}

	return unique
}

// ValidateImplementedSuggestions inspects modified files to verify whether recommendations were adopted.
func (s *DeepSuggestionService) ValidateImplementedSuggestions(
	ctx context.Context,
	repo string,
	pull int,
	suggestions []domain.CodeSuggestion,
	newFileContents map[string]string,
) ([]domain.CodeSuggestion, []domain.CodeSuggestion) {
	var implemented []domain.CodeSuggestion
	var pending []domain.CodeSuggestion

	for _, sug := range suggestions {
		content, hasFile := newFileContents[sug.GetFilePath()]
		if !hasFile {
			pending = append(pending, sug)
			continue
		}

		replacement := strings.TrimSpace(sug.GetSuggestedReplacement())
		existing := strings.TrimSpace(sug.GetOriginalDiff())

		isAdopted := false

		// Direct substring check for proposed replacement
		if replacement != "" && strings.Contains(content, replacement) {
			isAdopted = true
		} else if existing != "" && !strings.Contains(content, existing) {
			// Defect was removed/rewritten even if not character-identical
			isAdopted = true
		}

		if isAdopted {
			copySug := sug
			copySug.ImplementationStatus = domain.ImplementationStatusImplemented
			implemented = append(implemented, copySug)
		} else {
			copySug := sug
			copySug.ImplementationStatus = domain.ImplementationStatusPending
			pending = append(pending, copySug)
		}
	}

	return implemented, pending
}

// ResolveImplementedSuggestionsOnPlatform closes review comment threads on the SCM for fixed items.
func (s *DeepSuggestionService) ResolveImplementedSuggestionsOnPlatform(
	ctx context.Context,
	adapter SCMPlatformCommentAdapter,
	repo string,
	pull int,
	implemented []domain.CodeSuggestion,
) (int, error) {
	if adapter == nil {
		return 0, fmt.Errorf("SCM platform adapter is nil")
	}

	resolvedCount := 0
	for _, sug := range implemented {
		if sug.Comment != nil && sug.Comment.PlatformCommentID != "" {
			err := adapter.ResolveThread(ctx, repo, pull, sug.Comment.PlatformCommentID, ThreadStatusFixed)
			if err == nil {
				_ = adapter.MinimizeComment(ctx, repo, sug.Comment.PlatformCommentID, MinimizationReasonResolved)
				resolvedCount++
			}
		}
	}

	return resolvedCount, nil
}

// PrioritizeSuggestionsWithDrixyRulesControl reserves guaranteed quota slots for custom enterprise rules.
func (s *DeepSuggestionService) PrioritizeSuggestionsWithDrixyRulesControl(
	suggestions []domain.CodeSuggestion,
	enterpriseRuleIDs []string,
	limits SeverityLimitsConfig,
) ([]domain.CodeSuggestion, []domain.CodeSuggestion) {
	ruleIDMap := make(map[string]bool)
	for _, id := range enterpriseRuleIDs {
		ruleIDMap[id] = true
	}

	var ruleSuggestions []domain.CodeSuggestion
	var generalSuggestions []domain.CodeSuggestion

	for _, sug := range suggestions {
		if sug.RuleID != "" && (ruleIDMap[sug.RuleID] || len(enterpriseRuleIDs) == 0) {
			ruleSuggestions = append(ruleSuggestions, sug)
		} else {
			generalSuggestions = append(generalSuggestions, sug)
		}
	}

	var prioritized []domain.CodeSuggestion
	var discarded []domain.CodeSuggestion

	// Rule suggestions get first priority
	for _, rs := range ruleSuggestions {
		if len(prioritized) < limits.MaxTotal {
			prioritized = append(prioritized, rs)
		} else {
			rs.DeliveryStatus = domain.DeliveryStatusDiscarded
			rs.DiscardReason = "exceeded total review suggestion budget (rules overflow)"
			discarded = append(discarded, rs)
		}
	}

	// Fill remaining quota with general suggestions
	for _, gs := range generalSuggestions {
		if len(prioritized) < limits.MaxTotal {
			prioritized = append(prioritized, gs)
		} else {
			gs.DeliveryStatus = domain.DeliveryStatusDiscarded
			gs.DiscardReason = "exceeded total review suggestion budget"
			discarded = append(discarded, gs)
		}
	}

	return prioritized, discarded
}

// PrioritizeSuggestionsByFile enforces maximum comments per file to avoid review fatigue.
func (s *DeepSuggestionService) PrioritizeSuggestionsByFile(
	suggestions []domain.CodeSuggestion,
	maxPerFile int,
) ([]domain.CodeSuggestion, []domain.CodeSuggestion) {
	if maxPerFile <= 0 {
		maxPerFile = 5
	}

	byFile := make(map[string][]domain.CodeSuggestion)
	for _, sug := range suggestions {
		byFile[sug.GetFilePath()] = append(byFile[sug.GetFilePath()], sug)
	}

	sevRank := map[domain.ReviewSeverity]int{
		domain.SeverityCritical: 4,
		domain.SeverityMajor:    3,
		domain.SeverityMinor:    2,
		domain.SeverityInfo:     1,
	}

	var kept []domain.CodeSuggestion
	var discarded []domain.CodeSuggestion

	for _, list := range byFile {
		// Sort by severity desc, then priority score desc
		sort.Slice(list, func(i, j int) bool {
			if sevRank[list[i].Severity] != sevRank[list[j].Severity] {
				return sevRank[list[i].Severity] > sevRank[list[j].Severity]
			}
			return list[i].PriorityScore > list[j].PriorityScore
		})

		for i, sug := range list {
			if i < maxPerFile {
				kept = append(kept, sug)
			} else {
				sug.DeliveryStatus = domain.DeliveryStatusDiscarded
				sug.DiscardReason = fmt.Sprintf("exceeded per-file cap of %d comments in %s", maxPerFile, sug.GetFilePath())
				discarded = append(discarded, sug)
			}
		}
	}

	return kept, discarded
}

// PrioritizeSuggestionsByPR balances review remarks across the entire pull request.
func (s *DeepSuggestionService) PrioritizeSuggestionsByPR(
	suggestions []domain.CodeSuggestion,
	maxTotal int,
) ([]domain.CodeSuggestion, []domain.CodeSuggestion) {
	if maxTotal <= 0 {
		maxTotal = 30
	}

	if len(suggestions) <= maxTotal {
		return suggestions, nil
	}

	// Round-robin selection across files to ensure broad coverage
	byFile := make(map[string][]domain.CodeSuggestion)
	for _, sug := range suggestions {
		byFile[sug.GetFilePath()] = append(byFile[sug.GetFilePath()], sug)
	}

	var files []string
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)

	var kept []domain.CodeSuggestion
	var discarded []domain.CodeSuggestion
	pointers := make(map[string]int)

	for len(kept) < maxTotal {
		addedAny := false
		for _, f := range files {
			idx := pointers[f]
			if idx < len(byFile[f]) {
				kept = append(kept, byFile[f][idx])
				pointers[f] = idx + 1
				addedAny = true
				if len(kept) >= maxTotal {
					break
				}
			}
		}
		if !addedAny {
			break
		}
	}

	// Collect remaining suggestions as discarded
	for _, f := range files {
		idx := pointers[f]
		for i := idx; i < len(byFile[f]); i++ {
			sug := byFile[f][i]
			sug.DeliveryStatus = domain.DeliveryStatusDiscarded
			sug.DiscardReason = fmt.Sprintf("exceeded PR-wide comment budget of %d", maxTotal)
			discarded = append(discarded, sug)
		}
	}

	return kept, discarded
}

// NormalizeSeverity adjusts findings severity dynamically based on file criticality and symbol reachability.
func (s *DeepSuggestionService) NormalizeSeverity(
	sug domain.CodeSuggestion,
	isSecuritySensitive bool,
	callerCount int,
) domain.ReviewSeverity {
	current := sug.Severity

	if isSecuritySensitive {
		if current == domain.SeverityMinor {
			return domain.SeverityMajor
		}
		if current == domain.SeverityInfo {
			return domain.SeverityMinor
		}
	}

	if callerCount >= 20 {
		if current == domain.SeverityMajor {
			return domain.SeverityCritical
		}
	} else if callerCount >= 5 {
		if current == domain.SeverityMinor {
			return domain.SeverityMajor
		}
	}

	return current
}

// CalculateSuggestionRankScoreWithContext computes a composite rank score (0.0 - 100.0).
func (s *DeepSuggestionService) CalculateSuggestionRankScoreWithContext(
	sug domain.CodeSuggestion,
	reachabilityScore float64,
	proximityScore float64,
) float64 {
	sevWeights := map[domain.ReviewSeverity]float64{
		domain.SeverityCritical: 100.0,
		domain.SeverityMajor:    70.0,
		domain.SeverityMinor:    35.0,
		domain.SeverityInfo:     10.0,
	}

	sevScore := sevWeights[sug.Severity]
	conf := sug.Confidence
	if conf <= 0.0 {
		conf = 0.8
	}

	reach := math.Min(1.0, math.Max(0.0, reachabilityScore))
	prox := math.Min(1.0, math.Max(0.0, proximityScore))

	rank := (sevScore * 0.40) + (conf * 30.0) + (reach * 20.0) + (prox * 10.0)
	return math.Round(rank*100) / 100
}

// FilterActiveReviewSuggestions excludes suggestions that were already dispatched in previous review runs.
func (s *DeepSuggestionService) FilterActiveReviewSuggestions(
	suggestions []domain.CodeSuggestion,
	previouslySentFingerprints map[string]bool,
) []domain.CodeSuggestion {
	if len(previouslySentFingerprints) == 0 {
		return suggestions
	}

	var fresh []domain.CodeSuggestion
	for _, s := range suggestions {
		if !previouslySentFingerprints[s.GetFingerprint()] {
			fresh = append(fresh, s)
		}
	}
	return fresh
}

// AddRelatedSuggestionsFromPrioritizedParents decorates prioritized suggestions with references to clustered children.
func (s *DeepSuggestionService) AddRelatedSuggestionsFromPrioritizedParents(
	prioritized []domain.CodeSuggestion,
	clusters []SuggestionCluster,
) []domain.CodeSuggestion {
	clusterMap := make(map[string]SuggestionCluster)
	for _, c := range clusters {
		clusterMap[c.PrimaryID] = c
	}

	res := make([]domain.CodeSuggestion, len(prioritized))
	for i, p := range prioritized {
		copySug := p
		if c, found := clusterMap[p.ID.String()]; found {
			var childIDs []string
			for _, child := range c.Suggestions {
				if child.ID != p.ID {
					childIDs = append(childIDs, child.ID.String())
				}
			}
			copySug.BrokenRuleIDs = append(copySug.BrokenRuleIDs, childIDs...)
		}
		res[i] = copySug
	}

	return res
}

// FilterSuggestionsBySeverityLevel returns findings meeting or exceeding a minimum severity threshold.
func (s *DeepSuggestionService) FilterSuggestionsBySeverityLevel(
	suggestions []domain.CodeSuggestion,
	minSeverity domain.ReviewSeverity,
) []domain.CodeSuggestion {
	sevRank := map[domain.ReviewSeverity]int{
		domain.SeverityCritical: 4,
		domain.SeverityMajor:    3,
		domain.SeverityMinor:    2,
		domain.SeverityInfo:     1,
	}

	minRank := sevRank[minSeverity]
	var filtered []domain.CodeSuggestion

	for _, s := range suggestions {
		if sevRank[s.Severity] >= minRank {
			filtered = append(filtered, s)
		}
	}

	return filtered
}

// ProcessSeverityFilter extracts only suggestions belonging to the explicitly permitted severities.
func (s *DeepSuggestionService) ProcessSeverityFilter(
	suggestions []domain.CodeSuggestion,
	allowedSeverities []domain.ReviewSeverity,
) []domain.CodeSuggestion {
	allowed := make(map[domain.ReviewSeverity]bool)
	for _, a := range allowedSeverities {
		allowed[a] = true
	}

	var filtered []domain.CodeSuggestion
	for _, s := range suggestions {
		if allowed[s.Severity] {
			filtered = append(filtered, s)
		}
	}

	return filtered
}

// AnalyzeSuggestionsSeverity produces a frequency breakdown of findings by severity.
func (s *DeepSuggestionService) AnalyzeSuggestionsSeverity(suggestions []domain.CodeSuggestion) map[domain.ReviewSeverity]int {
	tally := make(map[domain.ReviewSeverity]int)
	for _, s := range suggestions {
		tally[s.Severity]++
	}
	return tally
}


