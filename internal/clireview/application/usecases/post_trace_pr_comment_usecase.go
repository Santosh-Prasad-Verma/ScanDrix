package usecases

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/scandrix/backend/internal/clireview/domain"
)

// TraceCommentMarker identifies ScanDrix Trace sticky comments on pull requests.
const TraceCommentMarker = "<!-- scandrix-trace-decisions -->"

// PostTracePrCommentInput encapsulates parameters for publishing or updating sticky comments.
type PostTracePrCommentInput struct {
	OrganizationID string                        `json:"organizationId"`
	TeamID         string                        `json:"teamId"`
	PRNumber       int                           `json:"prNumber"`
	RepositoryID   string                        `json:"repositoryId"`
	RepositoryName string                        `json:"repositoryName"`
	Decisions      []domain.TraceContextDecision `json:"decisions"`
	PlatformType   string                        `json:"platformType,omitempty"`
	DryRun         bool                          `json:"dryRun,omitempty"`
}

// PostTracePrCommentOutcome reports the outcome of the comment posting attempt.
type PostTracePrCommentOutcome struct {
	Action    string `json:"action"` // 'created' | 'updated' | 'skipped'
	CommentID int64  `json:"commentId,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// IIssueCommentManager abstracts posting and updating PR discussion comments.
type IIssueCommentManager interface {
	ListIssueComments(ctx context.Context, orgID, teamID, repoID string, prNumber int) ([]IssueCommentInfo, error)
	CreateIssueComment(ctx context.Context, orgID, teamID, repoID string, prNumber int, body string) (int64, error)
	UpdateIssueComment(ctx context.Context, orgID, teamID, repoID string, prNumber int, commentID int64, body string) error
}

// IssueCommentInfo conveys essential data from an existing PR comment.
type IssueCommentInfo struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
	Note string `json:"note,omitempty"`
}

// PostTracePrCommentUseCase manages sticky pull request comments carrying architectural trace decisions.
type PostTracePrCommentUseCase struct {
	commentManager IIssueCommentManager
}

// NewPostTracePrCommentUseCase creates a new post trace PR comment use case.
func NewPostTracePrCommentUseCase(commentManager IIssueCommentManager) *PostTracePrCommentUseCase {
	return &PostTracePrCommentUseCase{
		commentManager: commentManager,
	}
}

// Execute publishes or updates the sticky comment carrying trace decisions.
func (uc *PostTracePrCommentUseCase) Execute(ctx context.Context, input PostTracePrCommentInput) (*PostTracePrCommentOutcome, error) {
	var validDecisions []domain.TraceContextDecision
	for _, d := range input.Decisions {
		if strings.TrimSpace(d.Decision) != "" {
			validDecisions = append(validDecisions, d)
		}
	}

	if len(validDecisions) == 0 {
		return &PostTracePrCommentOutcome{Action: "skipped", Reason: "no-decisions"}, nil
	}

	if input.DryRun {
		return &PostTracePrCommentOutcome{Action: "skipped", Reason: "dry-run"}, nil
	}

	body := uc.RenderTraceComment(validDecisions)

	if uc.commentManager == nil {
		return &PostTracePrCommentOutcome{Action: "skipped", Reason: "error"}, nil
	}

	existingID, err := uc.findExistingComment(ctx, input)
	if err != nil {
		return &PostTracePrCommentOutcome{Action: "skipped", Reason: "error"}, nil
	}

	if existingID > 0 {
		err = uc.commentManager.UpdateIssueComment(ctx, input.OrganizationID, input.TeamID, input.RepositoryID, input.PRNumber, existingID, body)
		if err != nil {
			return &PostTracePrCommentOutcome{Action: "skipped", Reason: "error"}, nil
		}
		return &PostTracePrCommentOutcome{Action: "updated", CommentID: existingID}, nil
	}

	createdID, err := uc.commentManager.CreateIssueComment(ctx, input.OrganizationID, input.TeamID, input.RepositoryID, input.PRNumber, body)
	if err != nil {
		return &PostTracePrCommentOutcome{Action: "skipped", Reason: "error"}, nil
	}

	return &PostTracePrCommentOutcome{Action: "created", CommentID: createdID}, nil
}

func (uc *PostTracePrCommentUseCase) findExistingComment(ctx context.Context, input PostTracePrCommentInput) (int64, error) {
	comments, err := uc.commentManager.ListIssueComments(ctx, input.OrganizationID, input.TeamID, input.RepositoryID, input.PRNumber)
	if err != nil {
		return 0, err
	}

	for _, c := range comments {
		candidate := c.Body
		if candidate == "" {
			candidate = c.Note
		}
		if strings.Contains(candidate, TraceCommentMarker) {
			return c.ID, nil
		}
	}
	return 0, nil
}

// RenderTraceComment builds a GitHub markdown block presenting the trace decisions.
func (uc *PostTracePrCommentUseCase) RenderTraceComment(decisions []domain.TraceContextDecision) string {
	return RenderTraceComment(decisions)
}

// RenderTraceComment builds a structured markdown representation of the recorded trace decisions.
func RenderTraceComment(decisions []domain.TraceContextDecision) string {
	grouped := make(map[string][]domain.TraceContextDecision)
	for _, d := range decisions {
		k := string(d.Type)
		if k == "" {
			k = "other"
		}
		grouped[k] = append(grouped[k], d)
	}

	var types []string
	for k := range grouped {
		types = append(types, k)
	}
	// Sort types alphabetically for deterministic ordering
	slices.Sort(types)

	var sections []string
	for _, t := range types {
		items := grouped[t]
		sections = append(sections, fmt.Sprintf("**%s**", humanizeType(t)))
		sections = append(sections, "")
		for _, item := range items {
			details := []string{fmt.Sprintf("- %s", strings.TrimSpace(item.Decision))}
			if strings.TrimSpace(item.Rationale) != "" {
				details = append(details, fmt.Sprintf("  - _why:_ %s", strings.TrimSpace(item.Rationale)))
			}
			if len(item.Scope) > 0 {
				var formattedScope []string
				for _, sc := range item.Scope {
					formattedScope = append(formattedScope, fmt.Sprintf("`%s`", sc))
				}
				details = append(details, fmt.Sprintf("  - _scope:_ %s", strings.Join(formattedScope, ", ")))
			}
			sections = append(sections, strings.Join(details, "\n"))
		}
		sections = append(sections, "")
	}

	lines := []string{
		TraceCommentMarker,
		"## Why this changed",
		"",
		"Captured by ScanDrix Trace from the agent sessions behind this pull request.",
		"",
	}
	lines = append(lines, sections...)
	lines = append(lines, "<sub>Run `scandrix trace <path>` to read these from your terminal. `scandrix trace forget <id>` removes one that is wrong.</sub>")

	return strings.Join(lines, "\n")
}

func humanizeType(t string) string {
	parts := strings.Split(t, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
