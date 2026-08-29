package stages

import (
	"context"

	"github.com/scandrix/backend/internal/agents"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/rules"
)

// AgentDeliberationStage performs multi-turn AI reasoning on complex logic diffs.
type AgentDeliberationStage struct {
	reviewer *agents.AutonomousReviewer
}

func NewAgentDeliberationStage(evaluator *rules.Evaluator) *AgentDeliberationStage {
	return &AgentDeliberationStage{
		reviewer: agents.NewAutonomousReviewer(evaluator),
	}
}

func (s *AgentDeliberationStage) Name() string {
	return "agent_deliberation"
}

func (s *AgentDeliberationStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	if s.reviewer != nil && len(pCtx.FilteredPatches) > 0 {
		findings, thoughts, err := s.reviewer.ExecuteAgenticReview(ctx, pCtx.ReviewID, pCtx.WorkspaceID, pCtx.FilteredPatches)
		if err == nil {
			pCtx.AgentFindings = findings
			pCtx.AddMetric("agent_thoughts", 0, true, nil, len(thoughts))
		}
	}

	// Combine static findings and agent findings into AllFindings
	pCtx.AllFindings = append(pCtx.StaticFindings, pCtx.AgentFindings...)
	return nil
}
