package operations

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// -------------------------------------------------------------------------------------
// Multi-Provider Issue Synchronization Models
// -------------------------------------------------------------------------------------

// SCMIssue models an issue or ticket in an external source code management system.
type SCMIssue struct {
	ID          string            `json:"id"`
	Number      int               `json:"number"`
	Title       string            `json:"title"`
	Body        string            `json:"body"`
	State       string            `json:"state"` // "open", "closed"
	Labels      []string          `json:"labels"`
	Assignees   []string          `json:"assignees"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ClosedAt    *time.Time        `json:"closed_at,omitempty"`
	URL         string            `json:"url"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// TrackedFinding represents an active finding mapped to an SCM issue or PR comment.
type TrackedFinding struct {
	Fingerprint string    `json:"fingerprint"` // sha256 of file+rule+line/content
	RuleID      string    `json:"rule_id"`
	FilePath    string    `json:"file_path"`
	Line        int       `json:"line"`
	Severity    string    `json:"severity"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	SCMIssueID  string    `json:"scm_issue_id,omitempty"`
	CommentID   string    `json:"comment_id,omitempty"`
	Status      string    `json:"status"` // "open", "resolved", "dismissed"
	FirstSeenAt time.Time `json:"first_seen_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}

// SyncResult captures the output of a finding synchronization pass.
type SyncResult struct {
	TotalProcessed int              `json:"total_processed"`
	CreatedCount   int              `json:"created_count"`
	UpdatedCount   int              `json:"updated_count"`
	ResolvedCount  int              `json:"resolved_count"`
	UnchangedCount int              `json:"unchanged_count"`
	CreatedIssues  []SCMIssue       `json:"created_issues"`
	ResolvedIssues []SCMIssue       `json:"resolved_issues"`
}

// ResolutionReport summarizes resolved vs remaining findings between commits.
type ResolutionReport struct {
	ResolvedFindings []TrackedFinding `json:"resolved_findings"`
	RemainingFindings []TrackedFinding `json:"remaining_findings"`
	NewFindings       []TrackedFinding `json:"new_findings"`
}

// ReactionSummary aggregates reactions across review comments.
type ReactionSummary struct {
	CommentID   string         `json:"comment_id"`
	TotalCount  int            `json:"total_count"`
	Reactions   map[string]int `json:"reactions"` // e.g. "+1": 3, "-1": 0, "heart": 2
	HasPositive bool           `json:"has_positive"`
	HasNegative bool           `json:"has_negative"`
}

// ReviewCommentReference references a comment for reaction aggregation.
type ReviewCommentReference struct {
	CommentID string
	Provider  models.SCMProvider
	Reactions []string
}

// ProviderIssuePayload formats payload for remote issue creation.
type ProviderIssuePayload struct {
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Labels    []string `json:"labels"`
	Assignees []string `json:"assignees,omitempty"`
}

// -------------------------------------------------------------------------------------
// IssueSyncService
// -------------------------------------------------------------------------------------

// IssueSyncService coordinates cross-platform issue creation, resolution, and tracking.
type IssueSyncService struct {
	mu            sync.RWMutex
	trackedStore  map[string]TrackedFinding // Fingerprint -> TrackedFinding
	issueStore    map[string]SCMIssue       // SCMIssueID -> SCMIssue
}

// NewIssueSyncService creates a new instance of IssueSyncService.
func NewIssueSyncService() *IssueSyncService {
	return &IssueSyncService{
		trackedStore: make(map[string]TrackedFinding),
		issueStore:   make(map[string]SCMIssue),
	}
}

// ComputeFingerprint generates a stable unique identifier for a finding.
func ComputeFingerprint(ruleID, filePath string, line int) string {
	cleanPath := strings.TrimSpace(filePath)
	cleanRule := strings.TrimSpace(ruleID)
	return fmt.Sprintf("%s:%s:%d", cleanRule, cleanPath, line)
}

// TrackFinding registers or updates a finding in the local tracker.
func (s *IssueSyncService) TrackFinding(finding TrackedFinding) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if finding.Fingerprint == "" {
		finding.Fingerprint = ComputeFingerprint(finding.RuleID, finding.FilePath, finding.Line)
	}
	if finding.FirstSeenAt.IsZero() {
		finding.FirstSeenAt = time.Now().UTC()
	}
	if finding.Status == "" {
		finding.Status = "open"
	}
	s.trackedStore[finding.Fingerprint] = finding
}

// GetTrackedFinding retrieves a finding by its fingerprint.
func (s *IssueSyncService) GetTrackedFinding(fingerprint string) (TrackedFinding, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.trackedStore[fingerprint]
	return f, ok
}

// ListTrackedFindings returns all active or tracked findings matching a status filter.
func (s *IssueSyncService) ListTrackedFindings(statusFilter string) []TrackedFinding {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var findings []TrackedFinding
	for _, f := range s.trackedStore {
		if statusFilter == "" || f.Status == statusFilter {
			findings = append(findings, f)
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Fingerprint < findings[j].Fingerprint
	})
	return findings
}

// BuildProviderIssuePayload formats an SCM issue payload adhering to enterprise standards.
func (s *IssueSyncService) BuildProviderIssuePayload(
	provider models.SCMProvider,
	finding TrackedFinding,
) (*ProviderIssuePayload, error) {
	if finding.Title == "" {
		finding.Title = fmt.Sprintf("Security finding: %s in %s", finding.RuleID, finding.FilePath)
	}

	severityLabel := strings.ToUpper(finding.Severity)
	if severityLabel == "" {
		severityLabel = "MEDIUM"
	}

	labels := []string{
		"scandrix",
		"security",
		strings.ToLower(fmt.Sprintf("severity:%s", finding.Severity)),
	}

	body := fmt.Sprintf(`## 🛡️ ScanDrix Automated Security Finding

**Rule:** `+"`%s`"+`
**Severity:** **%s**
**File:** `+"`%s:%d`"+`

### Description
%s

---
*Reported automatically by ScanDrix Code Assurance Engine.*
<!-- scandrix-finding-fingerprint: %s -->
`, finding.RuleID, severityLabel, finding.FilePath, finding.Line, finding.Description, finding.Fingerprint)

	return &ProviderIssuePayload{
		Title:  finding.Title,
		Body:   body,
		Labels: labels,
	}, nil
}

// CheckFindingResolution compares previously tracked open findings with newly detected findings.
func (s *IssueSyncService) CheckFindingResolution(
	currentFindings []TrackedFinding,
) *ResolutionReport {
	s.mu.Lock()
	defer s.mu.Unlock()

	currentMap := make(map[string]TrackedFinding)
	for _, f := range currentFindings {
		fp := f.Fingerprint
		if fp == "" {
			fp = ComputeFingerprint(f.RuleID, f.FilePath, f.Line)
		}
		currentMap[fp] = f
	}

	now := time.Now().UTC()
	var resolved []TrackedFinding
	var remaining []TrackedFinding
	var newFindings []TrackedFinding

	// Check existing findings
	for fp, existing := range s.trackedStore {
		if existing.Status != "open" {
			continue
		}

		if _, exists := currentMap[fp]; !exists {
			// Finding was resolved in this revision
			existing.Status = "resolved"
			existing.ResolvedAt = &now
			s.trackedStore[fp] = existing
			resolved = append(resolved, existing)
		} else {
			remaining = append(remaining, existing)
		}
	}

	// Check for newly introduced findings
	for fp, curr := range currentMap {
		if _, exists := s.trackedStore[fp]; !exists {
			curr.Fingerprint = fp
			curr.FirstSeenAt = now
			curr.Status = "open"
			s.trackedStore[fp] = curr
			newFindings = append(newFindings, curr)
		}
	}

	return &ResolutionReport{
		ResolvedFindings:  resolved,
		RemainingFindings: remaining,
		NewFindings:       newFindings,
	}
}

// AggregateReactions tallies emoji reactions across review comments.
func (s *IssueSyncService) AggregateReactions(
	comments []ReviewCommentReference,
) map[string]ReactionSummary {
	results := make(map[string]ReactionSummary)

	for _, c := range comments {
		counts := make(map[string]int)
		hasPos := false
		hasNeg := false

		for _, r := range c.Reactions {
			norm := normalizeReaction(r)
			counts[norm]++
			if isPositiveReaction(norm) {
				hasPos = true
			}
			if isNegativeReaction(norm) {
				hasNeg = true
			}
		}

		results[c.CommentID] = ReactionSummary{
			CommentID:   c.CommentID,
			TotalCount:  len(c.Reactions),
			Reactions:   counts,
			HasPositive: hasPos,
			HasNegative: hasNeg,
		}
	}

	return results
}

func normalizeReaction(r string) string {
	switch strings.ToLower(strings.TrimSpace(r)) {
	case "+1", "thumbs_up", "thumbsup", "like":
		return "+1"
	case "-1", "thumbs_down", "thumbsdown", "dislike":
		return "-1"
	case "smile", "laugh", "happy":
		return "laugh"
	case "hooray", "tada", "party":
		return "hooray"
	case "confused":
		return "confused"
	case "heart", "love":
		return "heart"
	case "rocket":
		return "rocket"
	case "eyes":
		return "eyes"
	default:
		return r
	}
}

func isPositiveReaction(r string) bool {
	return r == "+1" || r == "laugh" || r == "hooray" || r == "heart" || r == "rocket"
}

func isNegativeReaction(r string) bool {
	return r == "-1" || r == "confused"
}

// GenerateMockIssueID generates a deterministic or UUID-based issue identifier.
func GenerateMockIssueID() string {
	return uuid.New().String()
}
