package stages

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/drixy"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

// HunkFormatterStage generates GitHub/GitLab compatible markdown comments.
type HunkFormatterStage struct{}

func NewHunkFormatterStage() *HunkFormatterStage {
	return &HunkFormatterStage{}
}

func (s *HunkFormatterStage) Name() string {
	return "hunk_comment_formatter"
}

func (s *HunkFormatterStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	var comments []pipeline.SCMInlineComment

	for _, f := range pCtx.AllFindings {
		badge := "⚡ **Drixy Security Alert**"
		switch f.Severity {
		case models.SeverityCritical:
			badge = "🔴 **DRIXY CRITICAL SECURITY RISK**"
		case models.SeverityHigh:
			badge = "🟠 **DRIXY HIGH VULNERABILITY**"
		case models.SeverityMedium:
			badge = "🟡 **DRIXY MEDIUM ISSUE**"
		case models.SeverityLow:
			badge = "🔵 **DRIXY LOW ISSUE**"
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%s: %s\n\n", badge, f.Title))
		sb.WriteString(fmt.Sprintf("%s\n\n", f.Description))

		suggestion := f.SuggestedDiff
		if suggestion == "" {
			suggestion = f.Remediation
		}

		if strings.TrimSpace(suggestion) != "" {
			sb.WriteString("```suggestion\n")
			sb.WriteString(suggestion)
			if !strings.HasSuffix(suggestion, "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("```\n")
		}

		sb.WriteString("\n*⚡ " + drixy.Name + " AST Reviewer · React 👍 to accept or 👎 to reject*")

		comments = append(comments, pipeline.SCMInlineComment{
			FilePath: f.FilePath,
			Line:     f.StartLine,
			Body:     sb.String(),
		})
	}

	pCtx.InlineComments = comments
	return nil
}
