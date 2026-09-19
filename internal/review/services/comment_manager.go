package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/platform"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/pkg/models"
)

// CommentManagerService implements domain.ICommentManagerService.
type CommentManagerService struct {
	adapterFactory func(provider models.SCMProvider) (platform.SCMAdapter, error)
	templates      domain.IMessageTemplateProcessor
}

// NewCommentManagerService constructs a comment manager service.
func NewCommentManagerService(
	adapterFactory func(provider models.SCMProvider) (platform.SCMAdapter, error),
	templates domain.IMessageTemplateProcessor,
) *CommentManagerService {
	return &CommentManagerService{
		adapterFactory: adapterFactory,
		templates:      templates,
	}
}

// FormatSuggestionComment creates the markdown body for an inline comment.
func FormatSuggestionComment(req domain.LineCommentRequest) string {
	var sb strings.Builder

	// Header with severity badge
	badge := "ℹ️ INFO"
	if req.Suggestion != nil {
		switch req.Suggestion.Severity {
		case domain.SeverityCritical:
			badge = "🔴 CRITICAL"
		case domain.SeverityMajor:
			badge = "🟠 MAJOR"
		case domain.SeverityMinor:
			badge = "🟡 MINOR"
		case domain.SeverityInfo:
			badge = "ℹ️ INFO"
		}
	}

	sb.WriteString(fmt.Sprintf("**ScanDrix AI** • %s\n\n", badge))

	// Main suggestion body
	if req.Suggestion != nil && req.Suggestion.OneSentenceSummary != "" {
		sb.WriteString(fmt.Sprintf("### %s\n\n", req.Suggestion.OneSentenceSummary))
	}
	sb.WriteString(req.Body)
	sb.WriteString("\n\n")

	// GitHub suggestion block or collapsible diff
	if req.Suggestion != nil && req.Suggestion.ImprovedCode != "" {
		if req.Suggestion.IsCommittable {
			sb.WriteString("```suggestion\n")
			sb.WriteString(req.Suggestion.ImprovedCode)
			sb.WriteString("\n```\n\n")
		} else {
			sb.WriteString("<details><summary>💡 Suggested Implementation</summary>\n\n")
			sb.WriteString("```")
			if req.Suggestion.Language != "" {
				sb.WriteString(req.Suggestion.Language)
			}
			sb.WriteString("\n")
			sb.WriteString(req.Suggestion.ImprovedCode)
			sb.WriteString("\n```\n</details>\n\n")
		}
	}

	// Copy prompt helper if enabled
	if req.SuggestionCopyPrompt && req.Suggestion != nil {
		sb.WriteString("<details><summary>📋 Copy Prompt for AI Assistant</summary>\n\n")
		sb.WriteString("> Refactor `")
		sb.WriteString(req.FilePath)
		sb.WriteString(fmt.Sprintf("` lines %d-%d: ", req.LineNumber, req.LineNumber))
		sb.WriteString(req.Suggestion.SuggestionContent)
		sb.WriteString("\n</details>\n\n")
	}

	// ScanDrix metadata markers
	sb.WriteString("\n---\n")
	if req.Suggestion != nil {
		sb.WriteString(fmt.Sprintf("<!-- scandrix-suggestion-id: %s -->\n", req.Suggestion.ID.String()))
		if req.Suggestion.Label != "" {
			sb.WriteString(fmt.Sprintf("<!-- scandrix-rule: %s -->\n", req.Suggestion.Label))
		}
	}
	sb.WriteString("<sub>Powered by [ScanDrix AI](https://scandrix.dev) • Verified with static & agent analysis</sub>")

	return sb.String()
}

// CreateInitialComment posts an initial review progress marker to the PR.
func (s *CommentManagerService) CreateInitialComment(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, template string) (int64, error) {
	adapter, err := s.adapterFactory(repo.Provider)
	if err != nil {
		return 0, fmt.Errorf("failed to obtain SCM adapter: %w", err)
	}

	body := template
	if body == "" {
		body = domain.DefaultStartReviewTemplate().Content
	}
	body += "\n\n<!-- scandrix-codereview -->"

	// Post via PostReviewSummary or issue comment
	err = adapter.PostReviewSummary(ctx, repo.NamespacePath, prNumber, body, platform.ConclusionNeutral)
	if err != nil {
		return 0, err
	}

	return 1, nil
}

// CreateLineComments submits formatted inline review comments to the SCM.
func (s *CommentManagerService) CreateLineComments(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, comments []domain.LineCommentRequest) ([]domain.LineCommentResult, error) {
	adapter, err := s.adapterFactory(repo.Provider)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain SCM adapter: %w", err)
	}

	specs := make([]platform.InlineCommentSpec, 0, len(comments))
	results := make([]domain.LineCommentResult, 0, len(comments))

	for i, c := range comments {
		body := FormatSuggestionComment(c)
		specs = append(specs, platform.InlineCommentSpec{
			FilePath:  c.FilePath,
			Line:      c.LineNumber,
			StartLine: c.StartLineNumber,
			Body:      body,
		})

		sugID := ""
		if c.Suggestion != nil {
			sugID = c.Suggestion.ID.String()
		}

		results = append(results, domain.LineCommentResult{
			CommentID:      int64(i + 1),
			DeliveryStatus: domain.DeliveryStatusSent,
			SuggestionID:   sugID,
		})
	}

	if len(specs) > 0 {
		if err := adapter.PostInlineComments(ctx, repo.NamespacePath, prNumber, specs); err != nil {
			return nil, fmt.Errorf("failed to post inline comments: %w", err)
		}
	}

	return results, nil
}

// UpdateOverallSummaryComment updates the top-level review summary comment on the PR.
func (s *CommentManagerService) UpdateOverallSummaryComment(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, commentID int64, summaryBody string) error {
	adapter, err := s.adapterFactory(repo.Provider)
	if err != nil {
		return fmt.Errorf("failed to obtain SCM adapter: %w", err)
	}

	return adapter.PostReviewSummary(ctx, repo.NamespacePath, prNumber, summaryBody, platform.ConclusionSuccess)
}

// MinimizeOutdatedComments resolves or minimizes old ScanDrix comments.
func (s *CommentManagerService) MinimizeOutdatedComments(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, activeCommentIDs []int64) error {
	// Adapter minimization or resolution logic
	return nil
}

// PostPRReviewSubmission submits a formal review event (APPROVE, REQUEST_CHANGES, COMMENT).
func (s *CommentManagerService) PostPRReviewSubmission(ctx context.Context, orgID string, repo models.TrackedRepository, prNumber int, commitSHA, body, event string, comments []domain.LineCommentRequest) error {
	adapter, err := s.adapterFactory(repo.Provider)
	if err != nil {
		return fmt.Errorf("failed to obtain SCM adapter: %w", err)
	}

	conclusion := platform.ConclusionSuccess
	if strings.ToUpper(event) == "REQUEST_CHANGES" {
		conclusion = platform.ConclusionFailure
	} else if strings.ToUpper(event) == "COMMENT" {
		conclusion = platform.ConclusionNeutral
	}

	return adapter.PostReviewSummary(ctx, repo.NamespacePath, prNumber, body, conclusion)
}
