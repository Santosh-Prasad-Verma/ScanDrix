package adapters

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/clireview/domain"
)

const maxSummaryDiffChars = 60000

// PublicPrAiSummaryService produces concise AI summaries of public pull requests.
type PublicPrAiSummaryService struct {
	llmRunner func(ctx context.Context, prompt string) (string, error)
}

// NewPublicPrAiSummaryService creates a new summary service.
func NewPublicPrAiSummaryService(runner func(ctx context.Context, prompt string) (string, error)) *PublicPrAiSummaryService {
	return &PublicPrAiSummaryService{
		llmRunner: runner,
	}
}

// Generate synthesizes an executive summary of the pull request changes.
func (s *PublicPrAiSummaryService) Generate(ctx context.Context, pr *domain.PublicPrMetadata, diff string) (string, error) {
	if pr == nil {
		return "", nil
	}

	truncatedDiff := diff
	truncated := false
	if len(diff) > maxSummaryDiffChars {
		truncatedDiff = diff[:maxSummaryDiffChars]
		truncated = true
	}

	prompt := s.buildPrompt(pr, truncatedDiff, truncated)

	if s.llmRunner != nil {
		text, err := s.llmRunner(ctx, prompt)
		if err == nil && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text), nil
		}
	}

	// Deterministic heuristic fallback when LLM is unavailable
	var bullets []string
	if pr.Additions > 0 || pr.Deletions > 0 {
		bullets = append(bullets, fmt.Sprintf("- Modified %d file(s) with +%d/-%d line changes", pr.ChangedFiles, pr.Additions, pr.Deletions))
	}
	if pr.CommitsCount > 0 {
		bullets = append(bullets, fmt.Sprintf("- Incorporated %d commit(s) from `%s` into `%s`", pr.CommitsCount, pr.HeadRef, pr.BaseRef))
	}
	if len(pr.Commits) > 0 {
		bullets = append(bullets, fmt.Sprintf("- Latest commit message: %s", pr.Commits[0].Message))
	}

	return fmt.Sprintf("**Overview:** Pull request `%s` proposes changes across %d file(s).\n\n**Key changes:**\n%s",
		pr.Title,
		pr.ChangedFiles,
		strings.Join(bullets, "\n"),
	), nil
}

func (s *PublicPrAiSummaryService) buildPrompt(pr *domain.PublicPrMetadata, diff string, truncated bool) string {
	var lines []string
	lines = append(lines, "You are reviewing a GitHub pull request. Write a concise summary for someone who hasn't read the PR yet.")
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Pull request: %s/%s#%d", pr.Owner, pr.Repo, pr.PRNumber))
	lines = append(lines, fmt.Sprintf("Title: %s", pr.Title))
	if pr.Author != nil {
		lines = append(lines, fmt.Sprintf("Author: %s", pr.Author.Login))
	}
	lines = append(lines, fmt.Sprintf("Branches: %s ← %s", pr.BaseRef, pr.HeadRef))
	lines = append(lines, fmt.Sprintf("Files changed: %d (+%d −%d)", pr.ChangedFiles, pr.Additions, pr.Deletions))
	lines = append(lines, "")
	lines = append(lines, "Output format (markdown, no preamble, keep tight):")
	lines = append(lines, "")
	lines = append(lines, "One short paragraph (1–3 sentences) describing what the PR does in plain English. Mention the core mechanism, not file names.")
	lines = append(lines, "")
	lines = append(lines, "**Key changes:**")
	lines = append(lines, "")
	lines = append(lines, "- 3 to 5 bullets, each starts with a verb in past tense (Added/Refactored/Fixed/…).")
	lines = append(lines, "- Each bullet names the concrete thing changed and why it matters. Reference the most relevant file path inline as backticks when useful.")
	lines = append(lines, "- No marketing fluff, no 'this PR' / 'this change' filler.")
	lines = append(lines, "")
	if truncated {
		lines = append(lines, "Note: the diff is large — only the first ~60k characters are shown. Focus on what you can see.")
		lines = append(lines, "")
	}
	lines = append(lines, "Unified diff:")
	lines = append(lines, "```diff")
	lines = append(lines, diff)
	lines = append(lines, "```")

	return strings.Join(lines, "\n")
}
