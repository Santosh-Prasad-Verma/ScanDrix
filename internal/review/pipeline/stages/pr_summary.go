package stages

import (
	"context"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/drixy"
	"github.com/scandrix/backend/internal/llm"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

// PRSummaryStage generates an executive pull request review summary markdown report.
type PRSummaryStage struct{}

func NewPRSummaryStage() *PRSummaryStage {
	return &PRSummaryStage{}
}

func (s *PRSummaryStage) Name() string {
	return "pr_summary_generator"
}

func (s *PRSummaryStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	var criticalCount, highCount, mediumCount, lowCount int
	for _, f := range pCtx.AllFindings {
		switch f.Severity {
		case models.SeverityCritical:
			criticalCount++
		case models.SeverityHigh:
			highCount++
		case models.SeverityMedium:
			mediumCount++
		case models.SeverityLow:
			lowCount++
		}
	}

	pCtx.PassedReview = criticalCount == 0 && highCount == 0

	var sb strings.Builder
	sb.WriteString("## ⚡ Drixy Code Review Summary\n\n")

	if pCtx.PassedReview {
		sb.WriteString("✅ **All Security & Quality Gates Passed** — No blocking issues found.\n\n")
	} else {
		sb.WriteString("⚠️ **Action Required** — High or critical security vulnerabilities detected.\n\n")
	}

	sb.WriteString("| Severity | Count | Status |\n")
	sb.WriteString("| :--- | :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| 🔴 Critical | %d | %s |\n", criticalCount, statusBadge(criticalCount)))
	sb.WriteString(fmt.Sprintf("| 🟠 High | %d | %s |\n", highCount, statusBadge(highCount)))
	sb.WriteString(fmt.Sprintf("| 🟡 Medium | %d | %s |\n", mediumCount, "Warning"))
	sb.WriteString(fmt.Sprintf("| 🔵 Low | %d | %s |\n\n", lowCount, "Informational"))

	if len(pCtx.AllFindings) > 0 {
		sb.WriteString("### Detailed Findings\n\n")
		for i, f := range pCtx.AllFindings {
			sb.WriteString(fmt.Sprintf("%d. **[%s]** `%s:%d` — %s\n", i+1, f.Severity, f.FilePath, f.StartLine, f.Title))
		}
		sb.WriteString("\n")
	}

	if pCtx.LastReviewError != nil {
		diag := llm.ReviewErrorDiagnostics{
			FriendlyMessage: pCtx.LastReviewError.FriendlyMessage,
			Provider:        pCtx.LastReviewError.Provider,
			Model:           pCtx.LastReviewError.Model,
			HTTPStatus:      pCtx.LastReviewError.HTTPStatus,
			ProviderMessage: pCtx.LastReviewError.ProviderMessage,
			AgentName:       pCtx.LastReviewError.AgentName,
		}
		sb.WriteString("### ⚠️ Review Diagnostics\n\n")
		sb.WriteString(llm.BuildReviewErrorMessage(diag) + "\n\n")
	}

	if pCtx.ExternalContext != nil {
		sb.WriteString(fmt.Sprintf("*Linked Issue: [%s]*\n", pCtx.ExternalContext.IssueKey))
	}

	sb.WriteString(drixy.PRCommentFooter())

	pCtx.PRSummaryBody = sb.String()
	return nil
}

func statusBadge(count int) string {
	if count > 0 {
		return "❌ Blocking"
	}
	return "✅ Clean"
}
