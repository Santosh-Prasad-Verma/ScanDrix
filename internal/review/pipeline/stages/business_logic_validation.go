package stages

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

// BusinessLogicValidationStage (Stage 8) verifies diff against ticket acceptance criteria.
type BusinessLogicValidationStage struct{}

// NewBusinessLogicValidationStage constructs Stage 8.
func NewBusinessLogicValidationStage() *BusinessLogicValidationStage {
	return &BusinessLogicValidationStage{}
}

func (s *BusinessLogicValidationStage) Name() string {
	return "BusinessLogicValidationStage"
}

func (s *BusinessLogicValidationStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if pCtx.SkipReview {
		return nil
	}

	// If external ticket context is required but missing
	if pCtx.ResolvedConfig.RequireTicketContext && pCtx.ExternalContext == nil {
		finding := models.CodeFinding{
			ID:          uuid.New(),
			ReviewID:    pCtx.ReviewID,
			WorkspaceID: pCtx.WorkspaceID,
			FilePath:    "PR_METADATA",
			StartLine:   1,
			EndLine:     1,
			Severity:    models.SeverityMedium,
			Category:    "business_logic",
			Title:       "Missing Issue Ticket Association",
			Description: "Repository configuration requires every pull request to link to a tracking issue (Jira/Linear/GitHub Issues).",
			Remediation: "Add ticket reference (e.g. 'SEC-123' or '#123') to pull request title or description.",
			Fingerprint: "rule_req_ticket",
		}
		pCtx.AllFindings = append(pCtx.AllFindings, finding)
		return nil
	}

	if pCtx.ExternalContext == nil {
		return nil
	}

	// Validate title or description alignment with issue context
	ticketKey := pCtx.ExternalContext.IssueKey
	if ticketKey != "" && !strings.Contains(pCtx.Title, ticketKey) && !strings.Contains(pCtx.Description, ticketKey) {
		finding := models.CodeFinding{
			ID:          uuid.New(),
			ReviewID:    pCtx.ReviewID,
			WorkspaceID: pCtx.WorkspaceID,
			FilePath:    "PR_METADATA",
			StartLine:   1,
			EndLine:     1,
			Severity:    models.SeverityInfo,
			Category:    "business_logic",
			Title:       fmt.Sprintf("Issue Key %s Missing From PR Header", ticketKey),
			Description: fmt.Sprintf("External context identified linked ticket '%s', but the key is not in the PR title.", ticketKey),
			Remediation: fmt.Sprintf("Prefix PR title with [%s] for automated release tracking.", ticketKey),
			Fingerprint: "rule_ticket_prefix",
		}
		pCtx.AllFindings = append(pCtx.AllFindings, finding)
	}

	return nil
}
