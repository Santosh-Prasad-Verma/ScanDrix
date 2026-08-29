package stages

import (
	"context"
	"fmt"
	"strings"

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
		badge := "🛡️ **ScanDrix Security**"
		switch f.Severity {
		case models.SeverityCritical:
			badge = "🔴 **CRITICAL SECURITY RISK**"
		case models.SeverityHigh:
			badge = "🟠 **HIGH VULNERABILITY**"
		case models.SeverityMedium:
			badge = "🟡 **MEDIUM ISSUE**"
		case models.SeverityLow:
			badge = "🔵 **LOW ISSUE**"
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

		comments = append(comments, pipeline.SCMInlineComment{
			FilePath: f.FilePath,
			Line:     f.StartLine,
			Body:     sb.String(),
		})
	}

	pCtx.InlineComments = comments
	return nil
}
