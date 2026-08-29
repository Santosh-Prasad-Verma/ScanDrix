package stages

import (
	"context"
	"regexp"

	"github.com/scandrix/backend/internal/review/pipeline"
)

var jiraIssueRegex = regexp.MustCompile(`([A-Z]{2,10}-[0-9]{1,6})`)

// ExternalContextStage discovers linked Jira or Linear issues from pull request metadata.
type ExternalContextStage struct{}

func NewExternalContextStage() *ExternalContextStage {
	return &ExternalContextStage{}
}

func (s *ExternalContextStage) Name() string {
	return "external_context_loader"
}

func (s *ExternalContextStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	matches := jiraIssueRegex.FindStringSubmatch(pCtx.Title)
	if len(matches) > 1 {
		pCtx.ExternalContext = &pipeline.ExternalIssueContext{
			IssueKey:    matches[1],
			Title:       pCtx.Title,
			Description: "Context extracted from pull request reference " + matches[1],
			Status:      "IN_REVIEW",
		}
	}
	return nil
}
