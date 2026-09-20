package stages

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBusinessLogicAgent struct {
	executeFunc func(ctx context.Context, input BusinessRulesValidationInput) (string, error)
}

func (m *mockBusinessLogicAgent) Execute(ctx context.Context, input BusinessRulesValidationInput) (string, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, input)
	}
	return "No gaps detected. All requirements met.", nil
}

type mockMCPManager struct {
	connections []MCPConnection
	err         error
}

func (m *mockMCPManager) GetConnections(ctx context.Context, orgID, teamID string) ([]MCPConnection, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.connections, nil
}

func TestDeepBusinessLogicValidationStage_SkipConditions(t *testing.T) {
	logger := slog.Default()
	agent := &mockBusinessLogicAgent{}
	mcpMgr := &mockMCPManager{
		connections: []MCPConnection{
			{
				Category:    "task-management",
				AppName:     "Jira Integration",
				IsConnected: true,
			},
		},
	}

	stage := NewDeepBusinessLogicValidationStage(logger, agent, mcpMgr)

	t.Run("Missing Organization", func(t *testing.T) {
		pCtx := &pipeline.PipelineContext{
			PullNumber:   10,
			RepositoryID: uuid.New(),
		}
		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Equal(t, "skipped", pCtx.BusinessLogicOutcome.Kind)
		assert.Equal(t, "missing_org", pCtx.BusinessLogicOutcome.Reason)
	})

	t.Run("Missing Pull Number", func(t *testing.T) {
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:  uuid.New(),
			RepositoryID: uuid.New(),
		}
		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Equal(t, "skipped", pCtx.BusinessLogicOutcome.Kind)
		assert.Equal(t, "missing_pr", pCtx.BusinessLogicOutcome.Reason)
	})

	t.Run("Business Logic Option Off", func(t *testing.T) {
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:  uuid.New(),
			RepositoryID: uuid.New(),
			PullNumber:   42,
			ResolvedConfig: domain.CodeReviewConfig{
				ReviewOptions: domain.ReviewOptions{
					BusinessLogic: false,
				},
			},
		}
		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Equal(t, "skipped", pCtx.BusinessLogicOutcome.Kind)
		assert.Equal(t, "option_off", pCtx.BusinessLogicOutcome.Reason)
	})

	t.Run("No Task MCP Connected", func(t *testing.T) {
		emptyMCPMgr := &mockMCPManager{connections: []MCPConnection{}}
		emptyStage := NewDeepBusinessLogicValidationStage(logger, agent, emptyMCPMgr)
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:  uuid.New(),
			RepositoryID: uuid.New(),
			PullNumber:   42,
			Description:  "Fixing ticket PROJ-101 for authentication",
			ResolvedConfig: domain.CodeReviewConfig{
				ReviewOptions: domain.ReviewOptions{
					BusinessLogic: true,
				},
			},
		}
		err := emptyStage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Equal(t, "skipped", pCtx.BusinessLogicOutcome.Kind)
		assert.Equal(t, "no_task_mcp", pCtx.BusinessLogicOutcome.Reason)
	})

	t.Run("No Relevant Business Signals", func(t *testing.T) {
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:  uuid.New(),
			RepositoryID: uuid.New(),
			PullNumber:   42,
			Description:  "Just clean up imports and format files with gofmt.",
			ResolvedConfig: domain.CodeReviewConfig{
				ReviewOptions: domain.ReviewOptions{
					BusinessLogic: true,
				},
			},
		}
		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Equal(t, "skipped", pCtx.BusinessLogicOutcome.Kind)
		assert.Equal(t, "no_signals", pCtx.BusinessLogicOutcome.Reason)
	})

	t.Run("Unchanged PR Body Skipped", func(t *testing.T) {
		body := "Implements PROJ-999 payment flow."
		bodyHash := stage.computePrBodyHash(body)

		pCtx := &pipeline.PipelineContext{
			WorkspaceID:  uuid.New(),
			RepositoryID: uuid.New(),
			PullNumber:   42,
			Description:  body,
			LastExecution: &pipeline.PreviousExecutionInfo{
				ExecutionID: "exec-1",
			},
			PipelineMetadata: map[string]interface{}{
				"businessLogicHash": bodyHash,
			},
			ResolvedConfig: domain.CodeReviewConfig{
				ReviewOptions: domain.ReviewOptions{
					BusinessLogic: true,
				},
			},
		}
		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Equal(t, "skipped", pCtx.BusinessLogicOutcome.Kind)
		assert.Equal(t, "unchanged_body", pCtx.BusinessLogicOutcome.Reason)
	})

	t.Run("Force Full Rerun Bypasses Unchanged Body", func(t *testing.T) {
		body := "Implements PROJ-999 payment flow."
		bodyHash := stage.computePrBodyHash(body)

		pCtx := &pipeline.PipelineContext{
			WorkspaceID:  uuid.New(),
			RepositoryID: uuid.New(),
			PullNumber:   42,
			Description:  body,
			Origin:       "command-force",
			LastExecution: &pipeline.PreviousExecutionInfo{
				ExecutionID: "exec-1",
			},
			PipelineMetadata: map[string]interface{}{
				"businessLogicHash": bodyHash,
			},
			ResolvedConfig: domain.CodeReviewConfig{
				ReviewOptions: domain.ReviewOptions{
					BusinessLogic: true,
				},
			},
		}
		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Equal(t, "success", pCtx.BusinessLogicOutcome.Kind)
	})
}

func TestDeepBusinessLogicValidationStage_ExecutionOutcomes(t *testing.T) {
	logger := slog.Default()
	mcpMgr := &mockMCPManager{
		connections: []MCPConnection{
			{
				IntegrationID: "scandrix-issues-default",
				Category:      "task-management",
				IsConnected:   true,
			},
			{
				IntegrationID: "linear-default",
				Category:      "task-management",
				IsConnected:   true,
			},
		},
	}

	t.Run("Sentinel No Task MCP Returned", func(t *testing.T) {
		agent := &mockBusinessLogicAgent{
			executeFunc: func(ctx context.Context, input BusinessRulesValidationInput) (string, error) {
				return NoTaskMCPSentinel, nil
			},
		}
		stage := NewDeepBusinessLogicValidationStage(logger, agent, mcpMgr)
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:  uuid.New(),
			RepositoryID: uuid.New(),
			PullNumber:   101,
			Description:  "Refers to ticket LINEAR-420 for billing upgrade.",
			ResolvedConfig: domain.CodeReviewConfig{
				ReviewOptions: domain.ReviewOptions{BusinessLogic: true},
			},
		}

		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Equal(t, "skipped", pCtx.BusinessLogicOutcome.Kind)
		assert.Equal(t, "no_task_mcp", pCtx.BusinessLogicOutcome.Reason)
	})

	t.Run("Weak Task Context Limitation Generates Suggestion", func(t *testing.T) {
		agent := &mockBusinessLogicAgent{
			executeFunc: func(ctx context.Context, input BusinessRulesValidationInput) (string, error) {
				return "The ticket lacks user stories and criteria. " + WeakTaskContextMarker + "\nNeed task information to verify edge cases.", nil
			},
		}
		stage := NewDeepBusinessLogicValidationStage(logger, agent, mcpMgr)
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:  uuid.New(),
			RepositoryID: uuid.New(),
			PullNumber:   102,
			Description:  "Addresses #550 with preliminary patch.",
			ResolvedConfig: domain.CodeReviewConfig{
				ReviewOptions: domain.ReviewOptions{BusinessLogic: true},
			},
		}

		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Equal(t, "skipped", pCtx.BusinessLogicOutcome.Kind)
		assert.Equal(t, "weak_task_context", pCtx.BusinessLogicOutcome.Reason)
		require.Len(t, pCtx.BusinessLogicResults, 1)
		assert.Equal(t, "Task description is insufficient for business logic validation.", pCtx.BusinessLogicResults[0].OneSentenceSummary)
		assert.Equal(t, domain.SeverityMedium, pCtx.BusinessLogicResults[0].Severity)
	})

	t.Run("General Agent Limitation Does Not Generate Suggestion", func(t *testing.T) {
		agent := &mockBusinessLogicAgent{
			executeFunc: func(ctx context.Context, input BusinessRulesValidationInput) (string, error) {
				return "Could not validate due to connection timeout on upstream repository diff.", nil
			},
		}
		stage := NewDeepBusinessLogicValidationStage(logger, agent, mcpMgr)
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:  uuid.New(),
			RepositoryID: uuid.New(),
			PullNumber:   103,
			Description:  "Fixing LINEAR-880 rate limit handling.",
			ResolvedConfig: domain.CodeReviewConfig{
				ReviewOptions: domain.ReviewOptions{BusinessLogic: true},
			},
		}

		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Equal(t, "skipped", pCtx.BusinessLogicOutcome.Kind)
		assert.Equal(t, "agent_limitation", pCtx.BusinessLogicOutcome.Reason)
		assert.Empty(t, pCtx.BusinessLogicResults)
	})

	t.Run("No Gap Outcome (Pass)", func(t *testing.T) {
		agent := &mockBusinessLogicAgent{
			executeFunc: func(ctx context.Context, input BusinessRulesValidationInput) (string, error) {
				return "Verification successful.\nAll requirements met and status: ✅ compliant.", nil
			},
		}
		stage := NewDeepBusinessLogicValidationStage(logger, agent, mcpMgr)
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:  uuid.New(),
			RepositoryID: uuid.New(),
			PullNumber:   104,
			Description:  "Implements Linear-99 requirement for idempotent webhook dispatch.",
			ResolvedConfig: domain.CodeReviewConfig{
				ReviewOptions: domain.ReviewOptions{BusinessLogic: true},
			},
		}

		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Equal(t, "success", pCtx.BusinessLogicOutcome.Kind)
		require.Len(t, pCtx.BusinessLogicResults, 1)
		assert.Equal(t, domain.SeverityLow, pCtx.BusinessLogicResults[0].Severity)
		assert.NotEmpty(t, pCtx.BusinessLogicPrBodyHash)
	})

	t.Run("Gap Found Outcome", func(t *testing.T) {
		agent := &mockBusinessLogicAgent{
			executeFunc: func(ctx context.Context, input BusinessRulesValidationInput) (string, error) {
				return "Missing validation for negative currency amounts in checkout payment payload.", nil
			},
		}
		stage := NewDeepBusinessLogicValidationStage(logger, agent, mcpMgr)
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:  uuid.New(),
			RepositoryID: uuid.New(),
			PullNumber:   105,
			Description:  "Fulfills PROJ-444 checkout requirement.",
			ResolvedConfig: domain.CodeReviewConfig{
				ReviewOptions: domain.ReviewOptions{BusinessLogic: true},
			},
		}

		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Equal(t, "gap_found", pCtx.BusinessLogicOutcome.Kind)
		require.Len(t, pCtx.BusinessLogicResults, 1)
		assert.Equal(t, domain.SeverityMedium, pCtx.BusinessLogicResults[0].Severity)
		assert.Equal(t, "Business logic gap detected based on PR requirements.", pCtx.BusinessLogicResults[0].OneSentenceSummary)
	})

	t.Run("Agent Error Records Partial Pipeline Error", func(t *testing.T) {
		agent := &mockBusinessLogicAgent{
			executeFunc: func(ctx context.Context, input BusinessRulesValidationInput) (string, error) {
				return "", errors.New("upstream LLM quota exceeded")
			},
		}
		stage := NewDeepBusinessLogicValidationStage(logger, agent, mcpMgr)
		pCtx := &pipeline.PipelineContext{
			WorkspaceID:  uuid.New(),
			RepositoryID: uuid.New(),
			PullNumber:   106,
			Description:  "Implements PROJ-555 refund idempotency.",
			ResolvedConfig: domain.CodeReviewConfig{
				ReviewOptions: domain.ReviewOptions{BusinessLogic: true},
			},
		}

		err := stage.Execute(context.Background(), pCtx)
		require.NoError(t, err)
		assert.Equal(t, "error", pCtx.BusinessLogicOutcome.Kind)
		require.Len(t, pCtx.PipelineErrors, 1)
		assert.Equal(t, "partial", pCtx.PipelineErrors[0].Severity)
		assert.Equal(t, "DeepBusinessLogicValidationStage", pCtx.PipelineErrors[0].Stage)
	})
}
