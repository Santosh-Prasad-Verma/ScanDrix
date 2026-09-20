package usecases

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/scandrix/backend/internal/clireview/domain"
)

// DefaultTraceContextPackTokenBudget caps context decisions injected into review prompts.
const DefaultTraceContextPackTokenBudget = domain.TraceContextPackTokenBudget

// BuildTraceContextPackInput defines input to the context-pack compiler.
type BuildTraceContextPackInput struct {
	OrganizationAndTeamData domain.OrganizationAndTeamData `json:"organizationAndTeamData,omitempty"`
	OrganizationID          string                         `json:"organizationId,omitempty"`
	TeamID                  string                         `json:"teamId,omitempty"`
	Repository              domain.RepositoryRef           `json:"repository,omitempty"`
	RepositoryID            string                         `json:"repositoryId,omitempty"`
	RepositoryName          string                         `json:"repositoryName,omitempty"`
	ChangedFilePaths        []string                       `json:"changedFilePaths"`
	Branch                  string                         `json:"branch"`
	TokenBudget             int                            `json:"tokenBudget,omitempty"`
}

// BuildTraceContextPackResult returns selected trace decisions and token stats.
type BuildTraceContextPackResult struct {
	Decisions        []domain.TraceContextDecision `json:"decisions"`
	DroppedForBudget int                           `json:"droppedForBudget"`
	EstimatedTokens  int                           `json:"estimatedTokens"`
}

// BuildTraceContextPackUseCase extracts relevant architecture decisions for a PR or diff.
type BuildTraceContextPackUseCase struct {
	reader domain.ITraceDecisionBranchReader
}

// NewBuildTraceContextPackUseCase creates a new context-pack use case.
func NewBuildTraceContextPackUseCase(reader domain.ITraceDecisionBranchReader) *BuildTraceContextPackUseCase {
	return &BuildTraceContextPackUseCase{
		reader: reader,
	}
}

// Execute filters and ranks recorded decisions against touched diff paths.
func (uc *BuildTraceContextPackUseCase) Execute(ctx context.Context, input BuildTraceContextPackInput) (*BuildTraceContextPackResult, error) {
	empty := &BuildTraceContextPackResult{
		Decisions:        []domain.TraceContextDecision{},
		DroppedForBudget: 0,
		EstimatedTokens:  0,
	}

	var changedFilePaths []string
	for _, entry := range input.ChangedFilePaths {
		norm := NormalizePath(entry)
		if norm != "" {
			changedFilePaths = append(changedFilePaths, norm)
		}
	}

	if len(changedFilePaths) == 0 {
		return empty, nil
	}

	orgID := input.OrganizationID
	if orgID == "" {
		orgID = input.OrganizationAndTeamData.OrganizationID
	}
	teamID := input.TeamID
	if teamID == "" {
		teamID = input.OrganizationAndTeamData.TeamID
	}
	repoID := input.RepositoryID
	if repoID == "" {
		repoID = input.Repository.ID
	}
	repoName := input.RepositoryName
	if repoName == "" {
		repoName = input.Repository.Name
	}

	if uc.reader == nil {
		return empty, nil
	}

	record, err := uc.reader.Read(ctx, domain.ReadTraceDecisionBranchInput{
		OrganizationID: orgID,
		TeamID:         teamID,
		RepositoryID:   repoID,
		RepositoryName: repoName,
		Branch:         input.Branch,
	})
	if err != nil || record == nil {
		return empty, nil
	}

	var matching []domain.TraceContextDecision
	for _, d := range DedupeDecisions(record.Decisions) {
		if MatchesAnyPath(d, changedFilePaths) {
			matching = append(matching, d)
		}
	}

	if len(matching) == 0 {
		return empty, nil
	}

	budget := input.TokenBudget
	if budget <= 0 {
		budget = domain.TraceContextPackTokenBudget
	}

	result := ApplyBudget(matching, budget)
	return &result, nil
}

// ApplyBudget trims decisions based on pinned status and confidence to fit token limit.
func ApplyBudget(decisions []domain.TraceContextDecision, tokenBudget int) BuildTraceContextPackResult {
	ordered := make([]domain.TraceContextDecision, len(decisions))
	copy(ordered, decisions)

	sort.SliceStable(ordered, func(i, j int) bool {
		return CompareForPack(ordered[i], ordered[j]) < 0
	})

	var kept []domain.TraceContextDecision
	used := 0
	dropped := 0

	for _, d := range ordered {
		cost := domain.EstimateTokens(RenderDecision(d))

		if d.Pinned {
			kept = append(kept, d)
			used += cost
			continue
		}

		if used+cost > tokenBudget {
			dropped++
			continue
		}

		kept = append(kept, d)
		used += cost
	}

	return BuildTraceContextPackResult{
		Decisions:        kept,
		DroppedForBudget: dropped,
		EstimatedTokens:  used,
	}
}

// RenderDecision formats a single decision into text.
func RenderDecision(decision domain.TraceContextDecision) string {
	parts := []string{fmt.Sprintf("- %s", decision.Decision)}

	if decision.Rationale != "" {
		parts = append(parts, fmt.Sprintf("  why: %s", decision.Rationale))
	}

	var meta []string
	if decision.Type != "" {
		meta = append(meta, string(decision.Type))
	}
	if decision.Origin != "" {
		meta = append(meta, fmt.Sprintf("origin: %s", decision.Origin))
	}
	meta = append(meta, fmt.Sprintf("confidence: %.2f", decision.Confidence))
	if len(decision.Scope) > 0 {
		meta = append(meta, fmt.Sprintf("scope: %s", strings.Join(decision.Scope, ", ")))
	}

	parts = append(parts, fmt.Sprintf("  (%s)", strings.Join(meta, " · ")))
	return strings.Join(parts, "\n")
}

// RenderTraceContextPack formats decisions for LLM prompt injection.
func RenderTraceContextPack(decisions []domain.TraceContextDecision) string {
	if len(decisions) == 0 {
		return ""
	}

	lines := []string{
		"### Recorded Decisions (why this code looks the way it does)",
		"",
		"These decisions were captured from the agent sessions that produced the",
		"code under review, scoped to the files in this diff. They may be stale",
		"or wrong and are not proof that the implementation is correct. Verify",
		"their claims and never suppress a concrete finding merely because the",
		"recorded decision describes the behavior as deliberate.",
		"",
	}

	for _, d := range decisions {
		lines = append(lines, RenderDecision(d))
	}

	return strings.Join(lines, "\n")
}

// CompareForPack compares decisions: pinned first, then highest confidence, then decision text.
func CompareForPack(a, b domain.TraceContextDecision) int {
	if a.Pinned != b.Pinned {
		if a.Pinned {
			return -1
		}
		return 1
	}

	diff := b.Confidence - a.Confidence
	if diff > 0.0001 {
		return 1
	}
	if diff < -0.0001 {
		return -1
	}

	return strings.Compare(a.Decision, b.Decision)
}

// DedupeDecisions deduplicates recorded decisions, keeping the highest confidence.
func DedupeDecisions(decisions []domain.TraceContextDecision) []domain.TraceContextDecision {
	type entry struct {
		d   domain.TraceContextDecision
		idx int
	}
	seen := make(map[string]entry)
	var order []string

	for _, d := range decisions {
		if strings.TrimSpace(d.Decision) == "" {
			continue
		}
		scopes := make([]string, len(d.Scope))
		copy(scopes, d.Scope)
		sort.Strings(scopes)
		key := fmt.Sprintf("%s|%s", d.Decision, strings.Join(scopes, ","))

		existing, exists := seen[key]
		if !exists {
			seen[key] = entry{d: d, idx: len(order)}
			order = append(order, key)
		} else if d.Confidence > existing.d.Confidence {
			seen[key] = entry{d: d, idx: existing.idx}
		}
	}

	result := make([]domain.TraceContextDecision, len(order))
	for _, k := range order {
		e := seen[k]
		result[e.idx] = e.d
	}

	return result
}

// MatchesAnyPath tests if a decision scope matches any touched files.
func MatchesAnyPath(decision domain.TraceContextDecision, changedFilePaths []string) bool {
	var scope []string
	for _, s := range decision.Scope {
		norm := NormalizePath(s)
		if norm != "" {
			scope = append(scope, norm)
		}
	}

	if len(scope) == 0 {
		return false
	}

	changedSet := make(map[string]struct{})
	for _, cp := range changedFilePaths {
		norm := NormalizePath(cp)
		if norm != "" {
			changedSet[norm] = struct{}{}
		}
	}

	changedPrefixes := PathPrefixes(changedSet)

	for _, s := range scope {
		if _, ok := changedPrefixes[s]; ok {
			return true
		}
		singlePrefixes := PathPrefixes(map[string]struct{}{s: {}})
		for p := range singlePrefixes {
			if _, ok := changedSet[p]; ok {
				return true
			}
		}
	}

	return false
}

// PathPrefixes computes all directory prefixes for given paths.
func PathPrefixes(paths map[string]struct{}) map[string]struct{} {
	prefixes := make(map[string]struct{})
	for p := range paths {
		segments := strings.Split(p, "/")
		for i := 1; i <= len(segments); i++ {
			prefixes[strings.Join(segments[:i], "/")] = struct{}{}
		}
	}
	return prefixes
}

// NormalizePath trims, cleans, and standardizes slash separators.
func NormalizePath(value string) string {
	s := strings.TrimSpace(value)
	s = strings.ReplaceAll(s, "\\", "/")
	s = strings.TrimPrefix(s, "./")
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimSuffix(s, "/")
	return s
}
