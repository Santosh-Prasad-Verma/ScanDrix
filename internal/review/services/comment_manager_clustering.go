package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/scandrix/backend/pkg/models"
)

// ClusteringType designates whether a suggestion is the primary master comment or an anchored sub-location.
type ClusteringType string

const (
	ClusteringTypeParent  ClusteringType = "PARENT"
	ClusteringTypeRelated ClusteringType = "RELATED"
)

// SubLocationAnchor identifies an occurrence of a repeated finding across files or lines.
type SubLocationAnchor struct {
	FilePath    string `json:"file_path"`
	LineStart   int    `json:"line_start"`
	LineEnd     int    `json:"line_end"`
	CommitSHA   string `json:"commit_sha,omitempty"`
	Snippet     string `json:"snippet,omitempty"`
	FindingID   string `json:"finding_id"`
}

// ClusteringInformation decorates suggestions with master/sub-location hierarchy matching repeatedClusteringSchema.
type ClusteringInformation struct {
	Type                  ClusteringType      `json:"type"`
	ParentSuggestionID    string              `json:"parent_suggestion_id,omitempty"`
	RelatedSuggestionsIDs []string            `json:"related_suggestions_ids,omitempty"`
	ProblemDescription    string              `json:"problem_description,omitempty"`
	ActionStatement       string              `json:"action_statement,omitempty"`
	SubLocations          []SubLocationAnchor `json:"sub_locations,omitempty"`
	ClusterSummary        string              `json:"cluster_summary,omitempty"`
}

// RepeatedClusteringEntry mirrors the ScanDrix repeatedClusteringSchema single entry.
type RepeatedClusteringEntry struct {
	ID                 string   `json:"id"`
	SameSuggestionsID  []string `json:"sameSuggestionsId,omitempty"`
	ProblemDescription string   `json:"problemDescription,omitempty"`
	ActionStatement    string   `json:"actionStatement,omitempty"`
}

// RepeatedClusteringPayload represents the top-level wire schema for structured LLM clustering.
type RepeatedClusteringPayload struct {
	CodeSuggestions []RepeatedClusteringEntry `json:"codeSuggestions"`
}

// ClusteredMasterSuggestion groups repeated occurrences into a high-visibility master review item.
type ClusteredMasterSuggestion struct {
	MasterID              string                   `json:"master_id"`
	Category              string                   `json:"category"`
	Severity              models.FindingSeverity   `json:"severity"`
	RuleID                string                   `json:"rule_id,omitempty"`
	MasterFinding         models.CodeFinding       `json:"master_finding"`
	RelatedFindings       []models.CodeFinding     `json:"related_findings"`
	ProblemDescription    string                   `json:"problem_description"`
	ActionStatement       string                   `json:"action_statement"`
	Locations             []SubLocationAnchor      `json:"locations"`
	FormattedMasterBody   string                   `json:"formatted_master_body"`
}

// SuggestionClusterEngine clusters recurring suggestions across pull requests to eliminate reviewer alert fatigue.
type SuggestionClusterEngine struct {
	mu           sync.RWMutex
	maxDistance  float64
	minClusterSz int
}

// NewSuggestionClusterEngine initializes the clustering engine.
func NewSuggestionClusterEngine() *SuggestionClusterEngine {
	return &SuggestionClusterEngine{
		maxDistance:  0.35,
		minClusterSz: 2,
	}
}

// ClusterFindings groups repeated findings across multiple files into master suggestions with sub-locations.
func (e *SuggestionClusterEngine) ClusterFindings(
	ctx context.Context,
	findings []models.CodeFinding,
) ([]ClusteredMasterSuggestion, []models.CodeFinding) {
	if len(findings) == 0 {
		return nil, nil
	}

	// 1. Group findings by rule ID and category
	groups := make(map[string][]models.CodeFinding)
	for _, f := range findings {
		key := fmt.Sprintf("%s::%s", strings.ToLower(f.Category), strings.ToLower(f.Title))
		if f.Fingerprint != "" {
			// If fingerprint has common prefix, use normalized rule key
			key = fmt.Sprintf("%s::%s", strings.ToLower(f.Category), normalizeTitleForClustering(f.Title))
		}
		groups[key] = append(groups[key], f)
	}

	var masters []ClusteredMasterSuggestion
	var standalone []models.CodeFinding
	clusteredIDs := make(map[string]bool)

	for _, group := range groups {
		if len(group) < e.minClusterSz {
			for _, f := range group {
				if !clusteredIDs[f.ID.String()] {
					standalone = append(standalone, f)
				}
			}
			continue
		}

		// Sort group: highest severity first, then by file path and line number
		sort.Slice(group, func(i, j int) bool {
			sevI := severityRank(group[i].Severity)
			sevJ := severityRank(group[j].Severity)
			if sevI != sevJ {
				return sevI > sevJ
			}
			if group[i].FilePath != group[j].FilePath {
				return group[i].FilePath < group[j].FilePath
			}
			return group[i].StartLine < group[j].StartLine
		})

		primary := group[0]
		var related []models.CodeFinding
		var anchors []SubLocationAnchor

		anchors = append(anchors, SubLocationAnchor{
			FilePath:  primary.FilePath,
			LineStart: primary.StartLine,
			LineEnd:   primary.EndLine,
			Snippet:   truncateSnippet(primary.SuggestedDiff, 60),
			FindingID: primary.ID.String(),
		})
		clusteredIDs[primary.ID.String()] = true

		for _, other := range group[1:] {
			related = append(related, other)
			anchors = append(anchors, SubLocationAnchor{
				FilePath:  other.FilePath,
				LineStart: other.StartLine,
				LineEnd:   other.EndLine,
				Snippet:   truncateSnippet(other.SuggestedDiff, 60),
				FindingID: other.ID.String(),
			})
			clusteredIDs[other.ID.String()] = true
		}

		probDesc := fmt.Sprintf(
			"Identical pattern '%s' detected across %d distinct locations in this pull request.",
			primary.Title, len(group),
		)
		actionStmt := fmt.Sprintf(
			"Apply consistent remediation pattern across all %d identified occurrences.",
			len(group),
		)

		master := ClusteredMasterSuggestion{
			MasterID:           primary.ID.String(),
			Category:           primary.Category,
			Severity:           primary.Severity,
			MasterFinding:      primary,
			RelatedFindings:    related,
			ProblemDescription: probDesc,
			ActionStatement:    actionStmt,
			Locations:          anchors,
		}
		master.FormattedMasterBody = e.FormatMasterComment(master)
		masters = append(masters, master)
	}

	return masters, standalone
}

// FormatMasterComment renders Markdown formatted master comment with a clean occurrence matrix table.
func (e *SuggestionClusterEngine) FormatMasterComment(master ClusteredMasterSuggestion) string {
	var sb strings.Builder

	icon := "💡"
	switch master.Severity {
	case models.SeverityCritical:
		icon = "🚨"
	case models.SeverityHigh:
		icon = "⚠️"
	case models.SeverityMedium:
		icon = "🟡"
	case models.SeverityLow:
		icon = "ℹ️"
	}

	sb.WriteString(fmt.Sprintf("%s **ScanDrix Master Cluster [%s]** — `%s`\n\n", icon, master.Severity, master.MasterFinding.Title))
	sb.WriteString(fmt.Sprintf("%s\n\n", master.ProblemDescription))
	sb.WriteString(fmt.Sprintf("**Remediation Guidance:** %s\n\n", master.ActionStatement))

	// Occurrence Table
	sb.WriteString(fmt.Sprintf("### 📍 Affected Locations (%d occurrences)\n\n", len(master.Locations)))
	sb.WriteString("| File | Line | Snippet Preview |\n")
	sb.WriteString("| :--- | :--- | :--- |\n")

	for _, loc := range master.Locations {
		snip := loc.Snippet
		if snip == "" {
			snip = "*(see diff)*"
		}
		snip = strings.ReplaceAll(snip, "|", "\\|")
		snip = strings.ReplaceAll(snip, "\n", " ")
		sb.WriteString(fmt.Sprintf("| `%s` | L%d-L%d | `%s` |\n", loc.FilePath, loc.LineStart, loc.LineEnd, snip))
	}
	sb.WriteString("\n")

	// Primary suggested diff if available
	if strings.TrimSpace(master.MasterFinding.SuggestedDiff) != "" {
		sb.WriteString("<details open>\n<summary><b>Primary Suggested Fix</b></summary>\n\n")
		sb.WriteString("```suggestion\n")
		sb.WriteString(strings.TrimSpace(master.MasterFinding.SuggestedDiff))
		sb.WriteString("\n```\n</details>\n\n")
	}

	sb.WriteString(fmt.Sprintf("*⚡ ScanDrix Suggestion Cluster `%s` · Consolidated to avoid notification fatigue*", master.MasterID[:8]))
	return sb.String()
}

// ConvertToRepeatedClusteringPayload converts clustered findings into the wire schema for telemetry or LLM round-trips.
func (e *SuggestionClusterEngine) ConvertToRepeatedClusteringPayload(masters []ClusteredMasterSuggestion) RepeatedClusteringPayload {
	var entries []RepeatedClusteringEntry
	for _, m := range masters {
		var sameIDs []string
		for _, rel := range m.RelatedFindings {
			sameIDs = append(sameIDs, rel.ID.String())
		}
		entries = append(entries, RepeatedClusteringEntry{
			ID:                 m.MasterID,
			SameSuggestionsID:  sameIDs,
			ProblemDescription: m.ProblemDescription,
			ActionStatement:    m.ActionStatement,
		})
	}
	return RepeatedClusteringPayload{CodeSuggestions: entries}
}

// ParseRepeatedClusteringPayload deserializes a structured LLM response matching repeatedClusteringSchema.
func ParseRepeatedClusteringPayload(jsonPayload string) (*RepeatedClusteringPayload, error) {
	var payload RepeatedClusteringPayload
	if err := json.Unmarshal([]byte(jsonPayload), &payload); err != nil {
		return nil, fmt.Errorf("failed deserializing repeated clustering payload: %w", err)
	}
	return &payload, nil
}

func normalizeTitleForClustering(title string) string {
	t := strings.ToLower(strings.TrimSpace(title))
	// Remove variable prefixes or line tags
	if idx := strings.Index(t, ":"); idx != -1 && idx < 20 {
		t = strings.TrimSpace(t[idx+1:])
	}
	return t
}

func severityRank(s models.FindingSeverity) int {
	switch s {
	case models.SeverityCritical:
		return 4
	case models.SeverityHigh:
		return 3
	case models.SeverityMedium:
		return 2
	case models.SeverityLow:
		return 1
	default:
		return 0
	}
}

func truncateSnippet(snip string, maxLen int) string {
	s := strings.TrimSpace(snip)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// -----------------------------------------------------------------------------
// SCM Thread Pagination & Reconciler
// -----------------------------------------------------------------------------

// PaginationCursor holds page or cursor tokens for multi-platform comment sync.
type PaginationCursor struct {
	PageSize      int    `json:"page_size"`
	CurrentPage   int    `json:"current_page"`
	NextPageToken string `json:"next_page_token,omitempty"`
	HasNextPage   bool   `json:"has_next_page"`
	TotalPages    int    `json:"total_pages,omitempty"`
}

// PaginatedThreadPage contains a slice of comments with navigation metadata.
type PaginatedThreadPage struct {
	Threads []CommentThreadState `json:"threads"`
	Cursor  PaginationCursor     `json:"cursor"`
}

// ThreadPageFetcher defines the client interface for retrieving comment batches from git providers.
type ThreadPageFetcher interface {
	FetchCommentThreadPage(ctx context.Context, repo string, pull int, cursor PaginationCursor) (*PaginatedThreadPage, error)
}

// PaginatedThreadSync retrieves all comments across multiple pages with deduplication and loop protection.
type PaginatedThreadSync struct {
	maxPages int
	fetcher  ThreadPageFetcher
}

// NewPaginatedThreadSync constructs a thread synchronizer.
func NewPaginatedThreadSync(fetcher ThreadPageFetcher, maxPages ...int) *PaginatedThreadSync {
	limit := 50
	if len(maxPages) > 0 && maxPages[0] > 0 {
		limit = maxPages[0]
	}
	return &PaginatedThreadSync{
		maxPages: limit,
		fetcher:  fetcher,
	}
}

// SyncAllCommentThreads exhaustively iterates over all pages to assemble the complete review thread state.
func (s *PaginatedThreadSync) SyncAllCommentThreads(ctx context.Context, repo string, pull int) ([]CommentThreadState, error) {
	if s.fetcher == nil {
		return nil, fmt.Errorf("no thread page fetcher configured")
	}

	var allThreads []CommentThreadState
	seenThreadIDs := make(map[string]bool)

	cursor := PaginationCursor{
		PageSize:    100,
		CurrentPage: 1,
		HasNextPage: true,
	}

	for page := 1; page <= s.maxPages && cursor.HasNextPage; page++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		res, err := s.fetcher.FetchCommentThreadPage(ctx, repo, pull, cursor)
		if err != nil {
			return nil, fmt.Errorf("page %d fetch error for %s#%d: %w", page, repo, pull, err)
		}

		for _, th := range res.Threads {
			if !seenThreadIDs[th.ThreadID] {
				seenThreadIDs[th.ThreadID] = true
				allThreads = append(allThreads, th)
			}
		}

		if !res.Cursor.HasNextPage || res.Cursor.NextPageToken == cursor.NextPageToken && page > 1 {
			break
		}

		cursor = res.Cursor
		cursor.CurrentPage = page + 1
	}

	return allThreads, nil
}

// -----------------------------------------------------------------------------
// SCM Comment Truncation & Markdown Recovery Engine
// -----------------------------------------------------------------------------

// SCMProviderCommentLimits defines character thresholds across major git hosting platforms.
type SCMProviderCommentLimits struct {
	MaxBodyLength    int `json:"max_body_length"`
	MaxSummaryLength int `json:"max_summary_length"`
}

// ProviderLimits returns official provider constraints.
func ProviderLimits(provider models.SCMProvider) SCMProviderCommentLimits {
	switch provider {
	case models.ProviderGitLab:
		return SCMProviderCommentLimits{
			MaxBodyLength:    1_000_000,
			MaxSummaryLength: 1_000_000,
		}
	case models.ProviderAzure:
		return SCMProviderCommentLimits{
			MaxBodyLength:    40_000,
			MaxSummaryLength: 40_000,
		}
	case models.ProviderBitbucket:
		return SCMProviderCommentLimits{
			MaxBodyLength:    32_768,
			MaxSummaryLength: 32_768,
		}
	default: // GitHub
		return SCMProviderCommentLimits{
			MaxBodyLength:    65_536,
			MaxSummaryLength: 65_536,
		}
	}
}

// SCMCommentTruncationFitter guarantees PR summaries and comments never exceed platform limits or break markdown.
type SCMCommentTruncationFitter struct {
	provider models.SCMProvider
	limits   SCMProviderCommentLimits
}

// NewSCMCommentTruncationFitter creates a truncation and recovery engine.
func NewSCMCommentTruncationFitter(provider models.SCMProvider) *SCMCommentTruncationFitter {
	return &SCMCommentTruncationFitter{
		provider: provider,
		limits:   ProviderLimits(provider),
	}
}

// FitComment safely truncates long markdown bodies and repairs unclosed syntax tags.
func (f *SCMCommentTruncationFitter) FitComment(markdown string) string {
	maxLen := f.limits.MaxBodyLength
	if len(markdown) <= maxLen {
		return markdown
	}

	// Truncate leaving headroom for repair notices
	headroom := 512
	targetLen := maxLen - headroom
	if targetLen < 100 {
		targetLen = maxLen / 2
	}

	// Cut at clean line boundary if possible
	cutIdx := strings.LastIndex(markdown[:targetLen], "\n")
	if cutIdx == -1 {
		cutIdx = targetLen
	}
	truncated := markdown[:cutIdx]

	// Add continuation notice
	var sb strings.Builder
	sb.WriteString(truncated)
	sb.WriteString("\n\n---\n*⚠️ Content truncated due to " + string(f.provider) + " comment character limit (" + fmt.Sprintf("%d", maxLen) + " chars).*\n")

	// Structural Markdown Repair
	repaired := RepairUnclosedMarkdown(sb.String())
	return repaired
}

// RepairUnclosedMarkdown analyzes and closes open code fences, tables, and HTML details tags.
func RepairUnclosedMarkdown(content string) string {
	var sb strings.Builder
	sb.WriteString(content)

	// 1. Repair triple-backtick code fences
	fenceCount := strings.Count(content, "```")
	if fenceCount%2 != 0 {
		sb.WriteString("\n```\n")
	}

	// 2. Repair open <details> tags
	openDetails := strings.Count(content, "<details")
	closeDetails := strings.Count(content, "</details>")
	for i := 0; i < (openDetails - closeDetails); i++ {
		sb.WriteString("\n</details>\n")
	}

	// 3. Repair open <table> tags
	openTable := strings.Count(content, "<table")
	closeTable := strings.Count(content, "</table>")
	for i := 0; i < (openTable - closeTable); i++ {
		sb.WriteString("\n</table>\n")
	}

	// 4. Ensure trailing newline
	res := sb.String()
	if !strings.HasSuffix(res, "\n") {
		res += "\n"
	}
	return res
}
