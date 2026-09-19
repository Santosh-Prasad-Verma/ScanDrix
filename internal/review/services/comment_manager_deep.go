// Package services provides production-grade infrastructure implementations for ScanDrix code review.
package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
)

// ThreadStatus defines the lifecycle status of a code review comment thread.
type ThreadStatus string

const (
	ThreadStatusActive    ThreadStatus = "ACTIVE"
	ThreadStatusFixed     ThreadStatus = "FIXED"
	ThreadStatusWontFix   ThreadStatus = "WONTFIX"
	ThreadStatusClosed    ThreadStatus = "CLOSED"
	ThreadStatusByDesign  ThreadStatus = "BY_DESIGN"
	ThreadStatusPending   ThreadStatus = "PENDING"
)

// MinimizationReason defines why a comment was collapsed or hidden on the SCM.
type MinimizationReason string

const (
	MinimizationReasonOutdated  MinimizationReason = "OUTDATED"
	MinimizationReasonResolved  MinimizationReason = "RESOLVED"
	MinimizationReasonDuplicate MinimizationReason = "DUPLICATE"
	MinimizationReasonOffTopic  MinimizationReason = "OFF_TOPIC"
	MinimizationReasonSpam      MinimizationReason = "SPAM"
)

// SCMCommentAnchor specifies exact line and side positioning for inline comments across git platforms.
type SCMCommentAnchor struct {
	FilePath       string `json:"file_path"`
	Line           int    `json:"line"`
	StartLine      int    `json:"start_line,omitempty"`
	Side           string `json:"side"` // "RIGHT" for added lines, "LEFT" for deleted lines
	StartSide      string `json:"start_side,omitempty"`
	OriginalCommit string `json:"original_commit,omitempty"`
	DiffHunk       string `json:"diff_hunk,omitempty"`
}

// SCMReviewComment represents an inline suggestion or remark ready for SCM submission.
type SCMReviewComment struct {
	ID             string            `json:"id"`
	SuggestionID   string            `json:"suggestion_id"`
	RuleID         string            `json:"rule_id,omitempty"`
	Anchor         SCMCommentAnchor  `json:"anchor"`
	Body           string            `json:"body"`
	InReplyToID    string            `json:"in_reply_to_id,omitempty"`
	IsDraft        bool              `json:"is_draft"`
	Severity       string            `json:"severity"`
	Category       string            `json:"category"`
	Fingerprint    string            `json:"fingerprint"`
	ThreadID       string            `json:"thread_id,omitempty"`
	Status         ThreadStatus      `json:"status"`
	CreatedAt      time.Time         `json:"created_at"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// SCMReviewBatch defines a grouped review payload submitted atomically to SCM providers.
type SCMReviewBatch struct {
	ReviewID      uuid.UUID          `json:"review_id"`
	WorkspaceID   uuid.UUID          `json:"workspace_id"`
	RepoNamespace string             `json:"repo_namespace"`
	PullNumber    int                `json:"pull_number"`
	HeadSHA       string             `json:"head_sha"`
	BaseSHA       string             `json:"base_sha"`
	SummaryBody   string             `json:"summary_body"`
	Event         string             `json:"event"` // "COMMENT", "APPROVE", "REQUEST_CHANGES"
	Comments      []SCMReviewComment `json:"comments"`
}

// CommentThreadState tracks active conversation threads on a pull request.
type CommentThreadState struct {
	ThreadID       string             `json:"thread_id"`
	RootCommentID  string             `json:"root_comment_id"`
	FilePath       string             `json:"file_path"`
	Line           int                `json:"line"`
	Fingerprint    string             `json:"fingerprint"`
	Status         ThreadStatus       `json:"status"`
	Replies        []SCMReviewComment `json:"replies,omitempty"`
	IsMinimized    bool               `json:"is_minimized"`
	MinimizeReason MinimizationReason `json:"minimize_reason,omitempty"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

// SCMPlatformCommentAdapter defines the low-level platform interaction interface.
type SCMPlatformCommentAdapter interface {
	CreateReviewBatch(ctx context.Context, batch SCMReviewBatch) (string, error)
	CreateSingleComment(ctx context.Context, repo string, pull int, comment SCMReviewComment) (string, error)
	ReplyToThread(ctx context.Context, repo string, pull int, threadID string, reply SCMReviewComment) (string, error)
	ResolveThread(ctx context.Context, repo string, pull int, threadID string, status ThreadStatus) error
	MinimizeComment(ctx context.Context, repo string, commentID string, reason MinimizationReason) error
	ListExistingThreads(ctx context.Context, repo string, pull int) ([]CommentThreadState, error)
	CreateComment(ctx context.Context, repo string, pull int, body string) (string, error)
	UpdateComment(ctx context.Context, repo string, commentID string, body string) error
}

// DeepCommentManager orchestrates multi-platform SCM comment lifecycles.
type DeepCommentManager struct {
	mu                sync.RWMutex
	adapters          map[string]SCMPlatformCommentAdapter
	templateProcessor *MessageTemplateProcessor
	activeThreads     map[string]map[string]*CommentThreadState // key: repo/pull -> threadID
	fingerprintIndex  map[string]map[string]string              // key: repo/pull -> fingerprint -> threadID
}

// NewDeepCommentManager initializes the advanced SCM comment manager.
func NewDeepCommentManager(processor *MessageTemplateProcessor) *DeepCommentManager {
	if processor == nil {
		processor = NewMessageTemplateProcessor()
	}
	return &DeepCommentManager{
		adapters:          make(map[string]SCMPlatformCommentAdapter),
		templateProcessor: processor,
		activeThreads:     make(map[string]map[string]*CommentThreadState),
		fingerprintIndex:  make(map[string]map[string]string),
	}
}

// RegisterAdapter binds a platform-specific SCM adapter (github, gitlab, azuredevops, bitbucket).
func (m *DeepCommentManager) RegisterAdapter(platform string, adapter SCMPlatformCommentAdapter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.adapters[strings.ToLower(platform)] = adapter
}

// GetAdapter retrieves the platform adapter.
func (m *DeepCommentManager) GetAdapter(platform string) (SCMPlatformCommentAdapter, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.adapters[strings.ToLower(platform)]
	return a, ok
}

func prKey(repo string, pull int) string {
	return fmt.Sprintf("%s#%d", strings.ToLower(repo), pull)
}

// SyncExistingThreads fetches and caches current review discussions from the SCM.
func (m *DeepCommentManager) SyncExistingThreads(ctx context.Context, platformName, repo string, pull int) error {
	adapter, ok := m.GetAdapter(platformName)
	if !ok {
		return fmt.Errorf("unsupported scm platform: %s", platformName)
	}

	threads, err := adapter.ListExistingThreads(ctx, repo, pull)
	if err != nil {
		return fmt.Errorf("failed listing threads for %s #%d: %w", repo, pull, err)
	}

	key := prKey(repo, pull)
	m.mu.Lock()
	defer m.mu.Unlock()

	threadMap := make(map[string]*CommentThreadState)
	fpMap := make(map[string]string)

	for _, th := range threads {
		copyTh := th
		threadMap[th.ThreadID] = &copyTh
		if th.Fingerprint != "" {
			fpMap[th.Fingerprint] = th.ThreadID
		}
	}

	m.activeThreads[key] = threadMap
	m.fingerprintIndex[key] = fpMap
	return nil
}

// FormatDeepSuggestionComment renders complete ScanDrix suggestion markdown with metadata tags and collapsible diffs.
func (m *DeepCommentManager) FormatDeepSuggestionComment(s domain.CodeSuggestion, author string) string {
	var b strings.Builder

	// Header line
	icon := "💡"
	switch s.Severity {
	case domain.SeverityCritical:
		icon = "🚨"
	case domain.SeverityMajor:
		icon = "⚠️"
	case domain.SeverityMinor:
		icon = "ℹ️"
	case domain.SeverityInfo:
		icon = "🔍"
	}

	b.WriteString(fmt.Sprintf("%s **ScanDrix Suggestion [%s]** — `%s`\n\n", icon, s.Severity, s.Category))

	if author != "" {
		b.WriteString(fmt.Sprintf("Hey @%s, ScanDrix identified an opportunity for improvement:\n\n", author))
	}

	b.WriteString(strings.TrimSpace(s.Description) + "\n\n")

	// Committable code suggestion block
	if strings.TrimSpace(s.SuggestedReplacement) != "" {
		b.WriteString("```suggestion\n")
		b.WriteString(strings.TrimRight(s.SuggestedReplacement, "\n"))
		b.WriteString("\n```\n\n")
	}

	// Explanation & rationale
	if strings.TrimSpace(s.Explanation) != "" {
		b.WriteString("> **Rationale:** " + strings.TrimSpace(s.Explanation) + "\n\n")
	}

	// Collapsible original context diff if available
	if strings.TrimSpace(s.OriginalDiff) != "" {
		b.WriteString("<details>\n<summary>🔎 View Original Diff Context</summary>\n\n```diff\n")
		b.WriteString(strings.TrimSpace(s.OriginalDiff))
		b.WriteString("\n```\n</details>\n\n")
	}

	// Machine-readable ScanDrix metadata comments for round-trip tracking and resolution
	b.WriteString(fmt.Sprintf("<!-- scandrix-suggestion-id: %s -->\n", s.ID))
	if s.RuleID != "" {
		b.WriteString(fmt.Sprintf("<!-- scandrix-rule: %s -->\n", s.RuleID))
	}
	fp := s.Fingerprint
	if fp == "" {
		h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", s.FilePath, s.StartLine, s.SuggestedReplacement)))
		fp = hex.EncodeToString(h[:])
	}
	b.WriteString(fmt.Sprintf("<!-- scandrix-fingerprint: %s -->\n", fp))
	b.WriteString("<!-- scandrix-origin: automated-review -->\n")

	return b.String()
}

// BuildReviewSummaryMarkdown creates the high-level PR overview comment.
func (m *DeepCommentManager) BuildReviewSummaryMarkdown(
	reviewID uuid.UUID,
	prTitle string,
	prNumber int,
	author string,
	headSHA string,
	suggestions []domain.CodeSuggestion,
	duration time.Duration,
	reviewMode string,
) string {
	var b strings.Builder

	b.WriteString("## 🛡️ ScanDrix AI Code Review Summary\n\n")
	b.WriteString(fmt.Sprintf("Reviewed commit `%s` for PR #%d by @%s in **%s**.\n\n", headSHA[:min(8, len(headSHA))], prNumber, author, duration.Round(time.Millisecond)))

	// Metrics table
	critCount := 0
	majCount := 0
	minCount := 0
	infoCount := 0

	for _, s := range suggestions {
		switch s.Severity {
		case domain.SeverityCritical:
			critCount++
		case domain.SeverityMajor:
			majCount++
		case domain.SeverityMinor:
			minCount++
		case domain.SeverityInfo:
			infoCount++
		}
	}

	total := len(suggestions)
	statusBadge := "🟢 PASSED"
	if critCount > 0 {
		statusBadge = "🔴 CHANGES REQUIRED"
	} else if majCount > 0 {
		statusBadge = "🟡 WARNINGS DETECTED"
	}

	b.WriteString(fmt.Sprintf("| Status | Total Findings | 🚨 Critical | ⚠️ Major | ℹ️ Minor | 🔍 Info |\n"))
	b.WriteString(fmt.Sprintf("| :---: | :---: | :---: | :---: | :---: | :---: |\n"))
	b.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d | %d |\n\n", statusBadge, total, critCount, majCount, minCount, infoCount))

	if total == 0 {
		b.WriteString("✨ **No security or architectural defects detected.** All checked invariants satisfied.\n\n")
	} else {
		b.WriteString("### 📋 Breakdown by File\n\n")
		files := make(map[string][]domain.CodeSuggestion)
		for _, s := range suggestions {
			files[s.FilePath] = append(files[s.FilePath], s)
		}

		sortedFiles := make([]string, 0, len(files))
		for f := range files {
			sortedFiles = append(sortedFiles, f)
		}
		sort.Strings(sortedFiles)

		for _, f := range sortedFiles {
			fileSugs := files[f]
			b.WriteString(fmt.Sprintf("- **`%s`** (%d finding%s)\n", f, len(fileSugs), pluralS(len(fileSugs))))
			for _, s := range fileSugs {
				b.WriteString(fmt.Sprintf("  - [%s] Line %d: %s\n", s.Severity, s.StartLine, truncateString(s.Description, 80)))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("> *Review Mode: `%s` · Review ID: `%s` · Powered by [ScanDrix](https://scandrix.dev)*\n", reviewMode, reviewID.String()))
	b.WriteString(fmt.Sprintf("<!-- scandrix-review-summary-id: %s -->\n", reviewID.String()))

	return b.String()
}

// GenerateInitialComment creates the in-progress status placeholder comment.
func (m *DeepCommentManager) GenerateInitialComment(prNumber int, author string, headSHA string) string {
	return fmt.Sprintf(`### ⚙️ ScanDrix AI Code Review In Progress

ScanDrix is currently analyzing PR #%d (`+"`%s`"+`) authored by @%s.

- [x] Parsing unified diff and AST symbol hierarchy
- [ ] Running multi-agent security and architectural deliberation
- [ ] Enforcing diff boundary safeguards and syntax checks
- [ ] Publishing verified committable suggestions

*This comment will be updated automatically when the review completes.*
<!-- scandrix-initial-comment: %d -->`, prNumber, headSHA[:min(8, len(headSHA))], author, prNumber)
}

// PrepareReviewBatch compiles domain suggestions into an atomic multi-platform SCM review batch.
func (m *DeepCommentManager) PrepareReviewBatch(
	reviewID uuid.UUID,
	workspaceID uuid.UUID,
	repo string,
	pull int,
	headSHA string,
	baseSHA string,
	suggestions []domain.CodeSuggestion,
	summaryBody string,
	prAuthor string,
) SCMReviewBatch {
	key := prKey(repo, pull)
	m.mu.RLock()
	existingFPs := m.fingerprintIndex[key]
	m.mu.RUnlock()

	var comments []SCMReviewComment
	hasCritical := false

	for _, s := range suggestions {
		if s.DeliveryStatus == domain.DeliveryStatusSuppressed || s.DeliveryStatus == domain.DeliveryStatusDiscarded {
			continue
		}
		if s.Severity == domain.SeverityCritical {
			hasCritical = true
		}

		fp := s.Fingerprint
		if fp == "" {
			h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", s.FilePath, s.StartLine, s.SuggestedReplacement)))
			fp = hex.EncodeToString(h[:])
		}

		// Avoid re-posting duplicate comment if thread already exists
		if existingFPs != nil {
			if _, exists := existingFPs[fp]; exists {
				continue
			}
		}

		body := m.FormatDeepSuggestionComment(s, prAuthor)

		side := "RIGHT"
		comments = append(comments, SCMReviewComment{
			ID:           uuid.New().String(),
			SuggestionID: s.ID.String(),
			RuleID:       s.RuleID,
			Anchor: SCMCommentAnchor{
				FilePath:       s.GetFilePath(),
				Line:           s.GetEndLine(),
				StartLine:      s.GetStartLine(),
				Side:           side,
				OriginalCommit: headSHA,
			},
			Body:        body,
			IsDraft:     false,
			Severity:    string(s.Severity),
			Category:    string(s.Category),
			Fingerprint: fp,
			Status:      ThreadStatusActive,
			CreatedAt:   time.Now().UTC(),
		})
	}

	event := "COMMENT"
	if hasCritical {
		event = "REQUEST_CHANGES"
	}

	return SCMReviewBatch{
		ReviewID:      reviewID,
		WorkspaceID:   workspaceID,
		RepoNamespace: repo,
		PullNumber:    pull,
		HeadSHA:       headSHA,
		BaseSHA:       baseSHA,
		SummaryBody:   summaryBody,
		Event:         event,
		Comments:      comments,
	}
}

// SubmitReviewBatch dispatches review comments through the matching platform adapter.
func (m *DeepCommentManager) SubmitReviewBatch(ctx context.Context, platformName string, batch SCMReviewBatch) (string, error) {
	adapter, ok := m.GetAdapter(platformName)
	if !ok {
		return "", fmt.Errorf("no adapter registered for platform: %s", platformName)
	}

	submissionID, err := adapter.CreateReviewBatch(ctx, batch)
	if err != nil {
		return "", fmt.Errorf("failed submitting review batch to %s: %w", platformName, err)
	}

	// Update local thread cache
	key := prKey(batch.RepoNamespace, batch.PullNumber)
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeThreads[key] == nil {
		m.activeThreads[key] = make(map[string]*CommentThreadState)
	}
	if m.fingerprintIndex[key] == nil {
		m.fingerprintIndex[key] = make(map[string]string)
	}

	for _, c := range batch.Comments {
		m.activeThreads[key][c.ID] = &CommentThreadState{
			ThreadID:      c.ID,
			RootCommentID: c.ID,
			FilePath:      c.Anchor.FilePath,
			Line:          c.Anchor.Line,
			Fingerprint:   c.Fingerprint,
			Status:        ThreadStatusActive,
			UpdatedAt:     time.Now().UTC(),
		}
		if c.Fingerprint != "" {
			m.fingerprintIndex[key][c.Fingerprint] = c.ID
		}
	}

	return submissionID, nil
}

// ReconcileResolvedComments checks previous active comments and resolves or minimizes them if the diff resolved them.
func (m *DeepCommentManager) ReconcileResolvedComments(
	ctx context.Context,
	platformName string,
	repo string,
	pull int,
	currentSuggestions []domain.CodeSuggestion,
) (int, error) {
	adapter, ok := m.GetAdapter(platformName)
	if !ok {
		return 0, fmt.Errorf("unsupported platform: %s", platformName)
	}

	key := prKey(repo, pull)
	m.mu.RLock()
	cachedThreads := m.activeThreads[key]
	m.mu.RUnlock()

	if len(cachedThreads) == 0 {
		return 0, nil
	}

	activeFPs := make(map[string]bool)
	for _, s := range currentSuggestions {
		fp := s.GetFingerprint()
		if fp == "" {
			h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", s.GetFilePath(), s.GetStartLine(), s.GetSuggestedReplacement())))
			fp = hex.EncodeToString(h[:])
		}
		activeFPs[fp] = true
	}

	resolvedCount := 0
	for threadID, thread := range cachedThreads {
		if thread.Status != ThreadStatusActive {
			continue
		}
		// If an active thread's finding fingerprint is NO LONGER present in current findings, it has been resolved!
		if !activeFPs[thread.Fingerprint] {
			err := adapter.ResolveThread(ctx, repo, pull, threadID, ThreadStatusFixed)
			if err == nil {
				_ = adapter.MinimizeComment(ctx, repo, thread.RootCommentID, MinimizationReasonResolved)
				thread.Status = ThreadStatusFixed
				thread.IsMinimized = true
				thread.MinimizeReason = MinimizationReasonResolved
				resolvedCount++
			}
		}
	}

	return resolvedCount, nil
}

// ExtractSuggestionIDFromCommentBody parses the metadata comment embedded in review remarks.
func ExtractSuggestionIDFromCommentBody(body string) string {
	re := regexp.MustCompile(`<!--\s*scandrix-suggestion-id:\s*([a-zA-Z0-9_-]+)\s*-->`)
	matches := re.FindStringSubmatch(body)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// ExtractFingerprintFromCommentBody parses the cryptographic fingerprint tag.
func ExtractFingerprintFromCommentBody(body string) string {
	re := regexp.MustCompile(`<!--\s*scandrix-fingerprint:\s*([a-fA-F0-9]+)\s*-->`)
	matches := re.FindStringSubmatch(body)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func truncateString(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// ChangedFileInfo encapsulates file metadata evaluated during PR review.
type ChangedFileInfo struct {
	Path         string `json:"path"`
	OldPath      string `json:"old_path,omitempty"`
	Status       string `json:"status"` // "added", "modified", "deleted", "renamed"
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
	Changes      int    `json:"changes"`
	PatchContent string `json:"patch_content,omitempty"`
}

// PRSummaryParams defines the payload for generating comprehensive PR summaries.
type PRSummaryParams struct {
	ReviewID        uuid.UUID               `json:"review_id"`
	Title           string                  `json:"title"`
	PullNumber      int                     `json:"pull_number"`
	Author          string                  `json:"author"`
	HeadSHA         string                  `json:"head_sha"`
	BaseSHA         string                  `json:"base_sha"`
	ChangedFiles    []ChangedFileInfo       `json:"changed_files"`
	Suggestions     []domain.CodeSuggestion `json:"suggestions"`
	Config          domain.CodeReviewConfig `json:"config"`
	StartTime       time.Time               `json:"start_time"`
	Duration        time.Duration           `json:"duration"`
	CustomNotes     []string                `json:"custom_notes,omitempty"`
	AttestationURL  string                  `json:"attestation_url,omitempty"`
}

// InitialCommentParams specifies metadata for the initial progress comment.
type InitialCommentParams struct {
	ReviewID    uuid.UUID `json:"review_id"`
	PullNumber  int       `json:"pull_number"`
	Author      string    `json:"author"`
	HeadSHA     string    `json:"head_sha"`
	TotalFiles  int       `json:"total_files"`
	EstimatedDuration time.Duration `json:"estimated_duration"`
}

// ReviewProgressUpdate tracks ongoing pipeline execution state.
type ReviewProgressUpdate struct {
	CurrentStage string  `json:"current_stage"`
	StageIndex   int     `json:"stage_index"`
	TotalStages  int     `json:"total_stages"`
	Percent      float64 `json:"percent"`
	FilesDone    int     `json:"files_done"`
	TotalFiles   int     `json:"total_files"`
	Message      string  `json:"message"`
}

// LastReviewParams holds data for closing the final review cycle.
type LastReviewParams struct {
	ReviewID          uuid.UUID               `json:"review_id"`
	Repo              string                  `json:"repo"`
	PullNumber        int                     `json:"pull_number"`
	Author            string                  `json:"author"`
	HeadSHA           string                  `json:"head_sha"`
	Status            string                  `json:"status"` // "PASSED", "CHANGES_REQUESTED", "COMMENTED"
	Suggestions       []domain.CodeSuggestion `json:"suggestions"`
	Duration          time.Duration           `json:"duration"`
	TotalFilesScanned int                     `json:"total_files_scanned"`
	SLSARating        string                  `json:"slsa_rating"`
}

// GeneratePullRequestSummaryMarkdown produces a full enterprise-grade markdown summary.
func (m *DeepCommentManager) GeneratePullRequestSummaryMarkdown(
	ctx context.Context,
	params PRSummaryParams,
) (string, error) {
	var sb strings.Builder

	// Header
	sb.WriteString("# 🔍 ScanDrix Code Review Summary\n\n")

	// Risk Assessment & Key Metrics
	riskScore, riskLevel := m.CalculateRiskScore(params.Suggestions)
	statusBadge := "✅ **Review Passed**"
	if riskScore >= 70 {
		statusBadge = "🚨 **Changes Requested**"
	} else if riskScore >= 35 {
		statusBadge = "⚠️ **Review Warnings**"
	}

	sb.WriteString(fmt.Sprintf("%s | **Risk Assessment: %s (%d/100)**\n\n", statusBadge, riskLevel, riskScore))

	// Overview Table
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("| :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **Pull Request** | #%d: %s |\n", params.PullNumber, params.Title))
	sb.WriteString(fmt.Sprintf("| **Author** | @%s |\n", params.Author))
	sb.WriteString(fmt.Sprintf("| **Commit Head** | `%s` |\n", truncateString(params.HeadSHA, 10)))
	sb.WriteString(fmt.Sprintf("| **Files Evaluated** | %d files (%s) |\n", len(params.ChangedFiles), params.Duration.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("| **Total Findings** | %d actionable suggestions |\n", len(params.Suggestions)))
	sb.WriteString("\n")

	// Severity Breakdown
	sevCounts := make(map[domain.ReviewSeverity]int)
	catCounts := make(map[domain.ReviewCategory]int)
	for _, s := range params.Suggestions {
		sevCounts[s.Severity]++
		catCounts[s.Category]++
	}

	sb.WriteString("### 📊 Severity & Category Distribution\n\n")
	sb.WriteString("| Severity | Count | Category | Count |\n")
	sb.WriteString("| :--- | :---: | :--- | :---: |\n")

	severities := []domain.ReviewSeverity{
		domain.SeverityCritical,
		domain.SeverityMajor,
		domain.SeverityMinor,
		domain.SeverityInfo,
	}
	categories := []domain.ReviewCategory{
		domain.CategorySecurity,
		domain.CategoryBug,
		domain.CategoryPerformance,
		domain.CategoryArchitecture,
	}

	maxRows := len(severities)
	if len(categories) > maxRows {
		maxRows = len(categories)
	}

	for i := 0; i < maxRows; i++ {
		sevCol := ""
		if i < len(severities) {
			s := severities[i]
			sevCol = fmt.Sprintf("| **%s** | %d ", s, sevCounts[s])
		} else {
			sevCol = "| | "
		}

		catCol := ""
		if i < len(categories) {
			c := categories[i]
			catCol = fmt.Sprintf("| %s | %d |", c, catCounts[c])
		} else {
			catCol = "| | |"
		}
		sb.WriteString(sevCol + catCol + "\n")
	}
	sb.WriteString("\n")

	// File Impact Analysis Table
	if len(params.ChangedFiles) > 0 {
		sb.WriteString(m.FormatFileImpactTable(params.ChangedFiles, params.Suggestions))
	}

	// Action Items & Security Hotspots
	critMajor := make([]domain.CodeSuggestion, 0)
	for _, s := range params.Suggestions {
		if s.Severity == domain.SeverityCritical || s.Severity == domain.SeverityMajor {
			critMajor = append(critMajor, s)
		}
	}

	if len(critMajor) > 0 {
		sb.WriteString("### 🚨 Priority Action Items\n\n")
		for _, s := range critMajor {
			icon := "🔴"
			if s.Severity == domain.SeverityMajor {
				icon = "🟠"
			}
			sb.WriteString(fmt.Sprintf("- %s **[%s]** [`%s:%d`](file://%s#L%d): %s\n",
				icon,
				s.Category,
				s.GetFilePath(),
				s.GetStartLine(),
				s.GetFilePath(),
				s.GetStartLine(),
				s.GetDescription(),
			))
		}
		sb.WriteString("\n")
	}

	// Config Summary Collapsible
	sb.WriteString("<details>\n<summary>⚙️ ScanDrix Configuration Applied</summary>\n\n")
	sb.WriteString(m.GenerateConfigReviewMarkdown(params.Config))
	sb.WriteString("</details>\n\n")

	// Footer & SLSA Attestation
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("⚡ *Analyzed by ScanDrix AI Engine (`%s`) in %v.*", params.ReviewID, params.Duration.Round(time.Millisecond)))
	if params.AttestationURL != "" {
		sb.WriteString(fmt.Sprintf(" | [SLSA Provenance Attestation](%s)", params.AttestationURL))
	}
	sb.WriteString("\n")

	return sb.String(), nil
}

// GenerateConfigReviewMarkdown documents the active configuration options applied to the review.
func (m *DeepCommentManager) GenerateConfigReviewMarkdown(config domain.CodeReviewConfig) string {
	var sb strings.Builder
	sb.WriteString("| Setting | Value |\n")
	sb.WriteString("| :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **Evaluation Strictness** | `%s` |\n", config.Strictness))
	sb.WriteString(fmt.Sprintf("| **Auto Approve Enabled** | `%t` |\n", config.AutoApprove))
	sb.WriteString(fmt.Sprintf("| **Approval Threshold** | `%.0f%%` |\n", config.ApprovalThreshold*100))
	sb.WriteString(fmt.Sprintf("| **Max Suggestions Cap** | `%d` |\n", config.MaxSuggestions))
	sb.WriteString(fmt.Sprintf("| **File Size Limit** | `%d lines / %d KB` |\n", config.FileSizeLimits.MaxLines, config.FileSizeLimits.MaxBytes/1024))
	sb.WriteString(fmt.Sprintf("| **Syntax Sandbox Checks** | `%t` |\n", config.SyntaxCheckInSandbox))
	sb.WriteString(fmt.Sprintf("| **Heavy Review Mode** | `%t` |\n", config.ReviewHeavyMode))

	if len(config.IgnorePaths) > 0 {
		sb.WriteString(fmt.Sprintf("| **Ignored Paths** | `%s` |\n", strings.Join(config.IgnorePaths, ", ")))
	}
	sb.WriteString("\n")
	return sb.String()
}

// GenerateLastReviewCommentBody builds the closure remark for the review cycle.
func (m *DeepCommentManager) GenerateLastReviewCommentBody(params LastReviewParams) string {
	var sb strings.Builder
	sb.WriteString("## 🏁 ScanDrix Review Finished\n\n")

	icon := "✅"
	switch params.Status {
	case "CHANGES_REQUESTED":
		icon = "🛑"
	case "WARNING":
		icon = "⚠️"
	}

	sb.WriteString(fmt.Sprintf("%s **Status: %s** on commit `%s`\n\n", icon, params.Status, truncateString(params.HeadSHA, 10)))
	sb.WriteString(fmt.Sprintf("- **Scan Duration:** %v\n", params.Duration.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("- **Files Scanned:** %d\n", params.TotalFilesScanned))
	sb.WriteString(fmt.Sprintf("- **Actionable Remarks:** %d\n", len(params.Suggestions)))
	if params.SLSARating != "" {
		sb.WriteString(fmt.Sprintf("- **Security Rating:** %s\n", params.SLSARating))
	}
	sb.WriteString("\n*ScanDrix ensures deterministic code quality with zero stubs and real verification.* <!-- scandrix:review-completed -->\n")

	return sb.String()
}

// ChunkChangedFilesForSummary splits large PR file sets into bounded batches.
func (m *DeepCommentManager) ChunkChangedFilesForSummary(files []ChangedFileInfo, maxChunkSize int) [][]ChangedFileInfo {
	if maxChunkSize <= 0 {
		maxChunkSize = 25
	}
	var chunks [][]ChangedFileInfo
	for i := 0; i < len(files); i += maxChunkSize {
		end := i + maxChunkSize
		if end > len(files) {
			end = len(files)
		}
		chunks = append(chunks, files[i:end])
	}
	return chunks
}

// CalculateRiskScore computes a weighted risk score (0-100) based on identified suggestions.
func (m *DeepCommentManager) CalculateRiskScore(suggestions []domain.CodeSuggestion) (int, string) {
	if len(suggestions) == 0 {
		return 0, "LOW"
	}

	score := 0
	for _, s := range suggestions {
		switch s.Severity {
		case domain.SeverityCritical:
			score += 35
		case domain.SeverityMajor:
			score += 15
		case domain.SeverityMinor:
			score += 5
		case domain.SeverityInfo:
			score += 1
		}
	}

	if score > 100 {
		score = 100
	}

	level := "LOW"
	if score >= 70 {
		level = "CRITICAL"
	} else if score >= 40 {
		level = "HIGH"
	} else if score >= 20 {
		level = "MEDIUM"
	}

	return score, level
}

// FormatFileImpactTable builds a markdown table detailing change magnitude and suggestion counts per file.
func (m *DeepCommentManager) FormatFileImpactTable(files []ChangedFileInfo, suggestions []domain.CodeSuggestion) string {
	sugsByFile := make(map[string]int)
	for _, s := range suggestions {
		sugsByFile[s.GetFilePath()]++
	}

	var sb strings.Builder
	sb.WriteString("### 📁 Changed Files Impact\n\n")
	sb.WriteString("| File | Changes (+/-) | Actionable Remarks |\n")
	sb.WriteString("| :--- | :---: | :---: |\n")

	for _, f := range files {
		cnt := sugsByFile[f.Path]
		cntStr := "-"
		if cnt > 0 {
			cntStr = fmt.Sprintf("⚠️ %d remark%s", cnt, pluralS(cnt))
		}
		sb.WriteString(fmt.Sprintf("| `%s` | +%d / -%d | %s |\n", f.Path, f.Additions, f.Deletions, cntStr))
	}
	sb.WriteString("\n")
	return sb.String()
}

// CreateInitialComment posts an upfront sticky comment indicating that ScanDrix is analyzing the PR.
func (m *DeepCommentManager) CreateInitialComment(
	ctx context.Context,
	platformName, repo string,
	pull int,
	params InitialCommentParams,
) (string, error) {
	adapter, ok := m.GetAdapter(platformName)
	if !ok {
		return "", fmt.Errorf("adapter not registered for platform %s", platformName)
	}

	body := fmt.Sprintf(
		"### 🤖 ScanDrix Review In Progress...\n\n"+
			"ScanDrix is currently analyzing **%d changed file%s** on commit `%s`.\n\n"+
			"- **Review ID:** `%s`\n"+
			"- **Estimated Analysis Time:** ~%v\n\n"+
			"⚡ *Inspecting AST symbols, running security heuristics, and verifying contracts.* <!-- scandrix-initial-comment -->",
		params.TotalFiles,
		pluralS(params.TotalFiles),
		truncateString(params.HeadSHA, 10),
		params.ReviewID,
		params.EstimatedDuration.Round(time.Second),
	)

	return adapter.CreateComment(ctx, repo, pull, body)
}

// UpdateInitialCommentProgress updates the sticky comment with real-time pipeline telemetry.
func (m *DeepCommentManager) UpdateInitialCommentProgress(
	ctx context.Context,
	platformName, repo string,
	pull int,
	commentID string,
	progress ReviewProgressUpdate,
) error {
	adapter, ok := m.GetAdapter(platformName)
	if !ok {
		return fmt.Errorf("adapter not registered for platform %s", platformName)
	}

	barWidth := 20
	completedBars := int(progress.Percent * float64(barWidth))
	if completedBars > barWidth {
		completedBars = barWidth
	}
	bar := strings.Repeat("█", completedBars) + strings.Repeat("░", barWidth-completedBars)

	body := fmt.Sprintf(
		"### 🤖 ScanDrix Review In Progress...\n\n"+
			"**Progress: [%s] %.0f%%**\n\n"+
			"- **Stage:** `%s` (%d/%d)\n"+
			"- **Files Processed:** %d/%d\n"+
			"- **Status:** %s\n\n"+
			"⚡ *Deterministic verification with zero stubs.* <!-- scandrix-initial-comment -->",
		bar,
		progress.Percent*100,
		progress.CurrentStage,
		progress.StageIndex,
		progress.TotalStages,
		progress.FilesDone,
		progress.TotalFiles,
		progress.Message,
	)

	return adapter.UpdateComment(ctx, repo, commentID, body)
}

// FinishInitialComment completes the initial progress comment with final status.
func (m *DeepCommentManager) FinishInitialComment(
	ctx context.Context,
	platformName, repo string,
	pull int,
	commentID string,
	status string,
	duration time.Duration,
) error {
	adapter, ok := m.GetAdapter(platformName)
	if !ok {
		return fmt.Errorf("adapter not registered for platform %s", platformName)
	}

	icon := "✅"
	if status == "CHANGES_REQUESTED" {
		icon = "🛑"
	}

	body := fmt.Sprintf(
		"### %s ScanDrix Review Completed\n\n"+
			"Analysis finished in **%v**.\n"+
			"Detailed file suggestions and overall summary posted below. <!-- scandrix-initial-comment -->",
		icon,
		duration.Round(time.Millisecond),
	)

	return adapter.UpdateComment(ctx, repo, commentID, body)
}

// ProcessEndReviewMessageTemplate populates end-of-review message templates with live context.
func (m *DeepCommentManager) ProcessEndReviewMessageTemplate(template string, vars map[string]string) string {
	res := template
	for k, v := range vars {
		res = strings.ReplaceAll(res, "{"+k+"}", v)
		res = strings.ReplaceAll(res, "{{"+k+"}}", v)
	}
	return res
}

// BuildDefaultTemplateContext produces replacement variables for template processors.
func (m *DeepCommentManager) BuildDefaultTemplateContext(
	reviewID uuid.UUID,
	repo string,
	pull int,
	author string,
	startTime time.Time,
	suggestions []domain.CodeSuggestion,
) map[string]string {
	critCount := 0
	majorCount := 0
	for _, s := range suggestions {
		if s.Severity == domain.SeverityCritical {
			critCount++
		} else if s.Severity == domain.SeverityMajor {
			majorCount++
		}
	}

	elapsed := time.Since(startTime).Round(time.Millisecond)

	_, riskLevel := m.CalculateRiskScore(suggestions)

	return map[string]string{
		"review_id":         reviewID.String(),
		"repo":              repo,
		"pull_number":       fmt.Sprintf("%d", pull),
		"author":            author,
		"suggestions_count": fmt.Sprintf("%d", len(suggestions)),
		"critical_count":    fmt.Sprintf("%d", critCount),
		"major_count":       fmt.Sprintf("%d", majorCount),
		"elapsed_time":      elapsed.String(),
		"risk_level":        riskLevel,
		"timestamp":         time.Now().UTC().Format(time.RFC3339),
	}
}

// UpdateOverallComment posts or replaces the overall PR review comment.
func (m *DeepCommentManager) UpdateOverallComment(
	ctx context.Context,
	platformName, repo string,
	pull int,
	body string,
) (string, error) {
	adapter, ok := m.GetAdapter(platformName)
	if !ok {
		return "", fmt.Errorf("adapter not registered for platform %s", platformName)
	}

	return adapter.CreateComment(ctx, repo, pull, body)
}

// FindLastReviewComment locates the most recent ScanDrix review comment on a pull request.
func (m *DeepCommentManager) FindLastReviewComment(
	ctx context.Context,
	platformName, repo string,
	pull int,
) (*SCMReviewComment, error) {
	key := prKey(repo, pull)
	m.mu.RLock()
	defer m.mu.RUnlock()

	threads := m.activeThreads[key]
	if len(threads) == 0 {
		return nil, nil
	}

	var latest *SCMReviewComment
	var latestTime time.Time

	for _, t := range threads {
		if t.UpdatedAt.After(latestTime) {
			latestTime = t.UpdatedAt
			latest = &SCMReviewComment{
				ID:           t.RootCommentID,
				ThreadID:     t.ThreadID,
				Fingerprint:  t.Fingerprint,
				Status:       t.Status,
				CreatedAt:    t.UpdatedAt,
			}
		}
	}

	return latest, nil
}

// MinimizeLastReviewComment hides outdated review comments on the SCM.
func (m *DeepCommentManager) MinimizeLastReviewComment(
	ctx context.Context,
	platformName, repo string,
	pull int,
	reason MinimizationReason,
) error {
	adapter, ok := m.GetAdapter(platformName)
	if !ok {
		return fmt.Errorf("adapter not registered for platform %s", platformName)
	}

	last, err := m.FindLastReviewComment(ctx, platformName, repo, pull)
	if err != nil || last == nil {
		return err
	}

	return adapter.MinimizeComment(ctx, repo, last.ID, reason)
}

// CreateLineComments submits a list of inline comments individually with retries.
func (m *DeepCommentManager) CreateLineComments(
	ctx context.Context,
	platformName, repo string,
	pull int,
	headSHA string,
	comments []SCMReviewComment,
) ([]string, []error) {
	adapter, ok := m.GetAdapter(platformName)
	if !ok {
		return nil, []error{fmt.Errorf("adapter not registered for platform %s", platformName)}
	}

	var commentIDs []string
	var errs []error

	for _, c := range comments {
		c.Anchor.OriginalCommit = headSHA
		id, err := m.CreateReviewCommentWithRetry(ctx, adapter, repo, pull, c, 3)
		if err != nil {
			// Try fallback fuzzy line positioning if original anchor failed
			fallbackID, fallbackErr := m.TryFallbackSuggestion(ctx, adapter, repo, pull, c)
			if fallbackErr != nil {
				errs = append(errs, fmt.Errorf("failed line comment on %s:%d: %w", c.Anchor.FilePath, c.Anchor.Line, err))
				continue
			}
			commentIDs = append(commentIDs, fallbackID)
		} else {
			commentIDs = append(commentIDs, id)
		}
	}

	return commentIDs, errs
}

// CreateReviewCommentWithRetry attempts comment creation with exponential backoff on transient errors.
func (m *DeepCommentManager) CreateReviewCommentWithRetry(
	ctx context.Context,
	adapter SCMPlatformCommentAdapter,
	repo string,
	pull int,
	comment SCMReviewComment,
	maxRetries int,
) (string, error) {
	var lastErr error
	backoff := 200 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		batch := SCMReviewBatch{
			RepoNamespace: repo,
			PullNumber:    pull,
			HeadSHA:       comment.Anchor.OriginalCommit,
			Event:         "COMMENT",
			Comments:      []SCMReviewComment{comment},
		}

		submissionID, err := adapter.CreateReviewBatch(ctx, batch)
		if err == nil {
			return submissionID, nil
		}

		lastErr = err
		// Check for unretryable client validation errors (e.g. line outside diff)
		if strings.Contains(err.Error(), "422") || strings.Contains(err.Error(), "outside diff") {
			return "", err
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
		}
	}

	return "", lastErr
}

// TryFallbackSuggestion repositions a comment anchor or converts it to a file-level note if the line shifted.
func (m *DeepCommentManager) TryFallbackSuggestion(
	ctx context.Context,
	adapter SCMPlatformCommentAdapter,
	repo string,
	pull int,
	comment SCMReviewComment,
) (string, error) {
	// Attempt anchoring to line 1 or converting to top-of-file comment
	fallback := comment
	fallback.Anchor.Line = 1
	fallback.Anchor.StartLine = 1
	fallback.Body = fmt.Sprintf("> ⚠️ *Original remark targeted line %d (shifted in recent commits):*\n\n%s", comment.Anchor.Line, comment.Body)

	batch := SCMReviewBatch{
		RepoNamespace: repo,
		PullNumber:    pull,
		HeadSHA:       comment.Anchor.OriginalCommit,
		Event:         "COMMENT",
		Comments:      []SCMReviewComment{fallback},
	}

	return adapter.CreateReviewBatch(ctx, batch)
}

// RepeatedCodeReviewSuggestionClustering groups co-located or recurring suggestions to avoid reviewer fatigue.
func (m *DeepCommentManager) RepeatedCodeReviewSuggestionClustering(
	ctx context.Context,
	suggestions []domain.CodeSuggestion,
	windowSize int,
) ([]domain.CodeSuggestion, []SuggestionCluster) {
	if windowSize <= 0 {
		windowSize = 5
	}

	byCatAndRule := make(map[string][]domain.CodeSuggestion)
	for _, s := range suggestions {
		key := fmt.Sprintf("%s::%s", s.Category, s.RuleID)
		byCatAndRule[key] = append(byCatAndRule[key], s)
	}

	var parentSuggestions []domain.CodeSuggestion
	var clusters []SuggestionCluster

	for _, group := range byCatAndRule {
		if len(group) <= 1 {
			parentSuggestions = append(parentSuggestions, group...)
			continue
		}

		// Sort by file and start line
		sort.Slice(group, func(i, j int) bool {
			if group[i].GetFilePath() != group[j].GetFilePath() {
				return group[i].GetFilePath() < group[j].GetFilePath()
			}
			return group[i].GetStartLine() < group[j].GetStartLine()
		})

		// Pick highest severity as primary parent
		highestSev := domain.SeverityInfo
		sevRank := map[domain.ReviewSeverity]int{
			domain.SeverityCritical: 4,
			domain.SeverityMajor:    3,
			domain.SeverityMinor:    2,
			domain.SeverityInfo:     1,
		}

		parentIdx := 0
		for i, s := range group {
			if sevRank[s.Severity] > sevRank[highestSev] {
				highestSev = s.Severity
				parentIdx = i
			}
		}

		parent := group[parentIdx]
		clusterID := uuid.New().String()

		cluster := SuggestionCluster{
			ClusterID:   clusterID,
			PrimaryID:   parent.ID.String(),
			Category:    parent.Category,
			Severity:    highestSev,
			Suggestions: group,
			Summary:     fmt.Sprintf("Found %d repeated %s patterns across %s", len(group), parent.Category, parent.GetFilePath()),
		}

		clusters = append(clusters, cluster)
		parentSuggestions = append(parentSuggestions, parent)
	}

	return parentSuggestions, clusters
}

// EnrichParentSuggestionsWithRelated decorates primary parent suggestions with references to clustered occurrences.
func (m *DeepCommentManager) EnrichParentSuggestionsWithRelated(
	parents []domain.CodeSuggestion,
	clusters []SuggestionCluster,
) []domain.CodeSuggestion {
	clusterByPrimary := make(map[string]SuggestionCluster)
	for _, c := range clusters {
		clusterByPrimary[c.PrimaryID] = c
	}

	res := make([]domain.CodeSuggestion, len(parents))
	for i, p := range parents {
		copySug := p
		if c, found := clusterByPrimary[p.ID.String()]; found && len(c.Suggestions) > 1 {
			var relatedNote strings.Builder
			relatedNote.WriteString(fmt.Sprintf("\n\n---\n*Also detected at %d other location%s:*", len(c.Suggestions)-1, pluralS(len(c.Suggestions)-1)))
			for _, other := range c.Suggestions {
				if other.ID == p.ID {
					continue
				}
				relatedNote.WriteString(fmt.Sprintf("\n- `%s:%d`", other.GetFilePath(), other.GetStartLine()))
			}
			copySug.Explanation = copySug.GetExplanation() + relatedNote.String()
		}
		res[i] = copySug
	}

	return res
}

// ExtractAllClusteredIDs collects all unique suggestion IDs belonging to clusters.
func (m *DeepCommentManager) ExtractAllClusteredIDs(clusters []SuggestionCluster) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, c := range clusters {
		for _, s := range c.Suggestions {
			idStr := s.ID.String()
			if !seen[idStr] {
				seen[idStr] = true
				ids = append(ids, idStr)
			}
		}
	}
	return ids
}

// FilterNonClusteredSuggestions returns only suggestions that were not subsumed into multi-occurrence clusters.
func (m *DeepCommentManager) FilterNonClusteredSuggestions(
	suggestions []domain.CodeSuggestion,
	clusteredIDs map[string]bool,
) []domain.CodeSuggestion {
	var nonClustered []domain.CodeSuggestion
	for _, s := range suggestions {
		if !clusteredIDs[s.ID.String()] {
			nonClustered = append(nonClustered, s)
		}
	}
	return nonClustered
}

// CreatePrLevelReviewComments publishes top-level architectural suggestions not anchored to diff hunks.
func (m *DeepCommentManager) CreatePrLevelReviewComments(
	ctx context.Context,
	platformName, repo string,
	pull int,
	comments []SCMReviewComment,
) ([]string, error) {
	adapter, ok := m.GetAdapter(platformName)
	if !ok {
		return nil, fmt.Errorf("adapter not registered for platform %s", platformName)
	}

	var createdIDs []string
	for _, c := range comments {
		body := fmt.Sprintf("### 🌐 PR-Level Architecture Finding: %s\n\n%s", c.Category, c.Body)
		id, err := adapter.CreateComment(ctx, repo, pull, body)
		if err != nil {
			return createdIDs, err
		}
		createdIDs = append(createdIDs, id)
	}

	return createdIDs, nil
}

// SanitizeBitbucketMarkdown formats GitHub-flavored markdown tags for Bitbucket Cloud/Server compatibility.
func (m *DeepCommentManager) SanitizeBitbucketMarkdown(markdown string) string {
	res := markdown
	// Replace <details><summary>Title</summary>Content</details> with blockquote
	reDetails := regexp.MustCompile(`(?s)<details>\s*<summary>(.*?)</summary>(.*?)</details>`)
	res = reDetails.ReplaceAllStringFunc(res, func(m string) string {
		sub := reDetails.FindStringSubmatch(m)
		if len(sub) == 3 {
			title := strings.TrimSpace(sub[1])
			content := strings.TrimSpace(sub[2])
			return fmt.Sprintf("#### %s\n> %s", title, strings.ReplaceAll(content, "\n", "\n> "))
		}
		return m
	})

	// Replace ```suggestion with ```diff
	res = strings.ReplaceAll(res, "```suggestion", "```diff")
	return res
}

// FormatBitbucketDiffAnchor structures inline positioning coordinates for Bitbucket Server/Cloud REST endpoints.
func (m *DeepCommentManager) FormatBitbucketDiffAnchor(comment SCMReviewComment) map[string]any {
	lineType := "CONTEXT"
	if comment.Anchor.Side == "RIGHT" {
		lineType = "ADDED"
	}

	return map[string]any{
		"path":     comment.Anchor.FilePath,
		"line":     comment.Anchor.Line,
		"lineType": lineType,
		"fileType": "TO",
	}
}
