package stages_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/runner"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/orchestrator"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/review/pipeline/stages"
	"github.com/scandrix/backend/internal/rules"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentDeliberationStage_EmptyPatches(t *testing.T) {
	evaluator, err := rules.NewEvaluator(rules.DefaultCatalog())
	require.NoError(t, err)

	stage := stages.NewAgentDeliberationStage(evaluator)
	pCtx := &pipeline.PipelineContext{
		ReviewID:        uuid.New(),
		WorkspaceID:     uuid.New(),
		FilteredPatches: nil,
	}

	err = stage.Execute(context.Background(), pCtx)
	require.NoError(t, err)
	assert.Empty(t, pCtx.AgentFindings)
}

func TestAgentDeliberationStage_LegacyFallback(t *testing.T) {
	evaluator, err := rules.NewEvaluator(rules.DefaultCatalog())
	require.NoError(t, err)

	stage := stages.NewAgentDeliberationStage(evaluator)
	patch := &diff.FilePatch{
		OldPath: "pkg/auth.go",
		NewPath: "pkg/auth.go",
		Hunks: []diff.Hunk{
			{
				OldStart: 1,
				OldLines: 1,
				NewStart: 1,
				NewLines: 1,
				Lines: []diff.DiffLine{
					{Type: diff.LineAddition, NewLineNo: 1, Content: "apiKey := \"AKIAIOSFODNN7EXAMPLE\""},
				},
			},
		},
	}

	pCtx := &pipeline.PipelineContext{
		ReviewID:        uuid.New(),
		WorkspaceID:     uuid.New(),
		FilteredPatches: []*diff.FilePatch{patch},
	}

	err = stage.Execute(context.Background(), pCtx)
	require.NoError(t, err)
	// Evaluator identifies the AWS key rule
	assert.NotEmpty(t, pCtx.AgentFindings)
	assert.NotEmpty(t, pCtx.AllFindings)
}

func TestAgentDeliberationStage_CoreEngine(t *testing.T) {
	ctx := context.Background()

	stepCount := 0
	invoker := func(ctx context.Context, req runner.ModelTurnRequest) (*runner.ModelTurnResult, error) {
		stepCount++
		if stepCount == 1 {
			// First step: read file
			return &runner.ModelTurnResult{
				Text: "Reading auth.go",
				ToolCalls: []contracts.ToolCallRecord{
					{
						ID:   "call_1",
						Name: "readFile",
						Input: map[string]any{
							"path": "auth.go",
						},
					},
				},
				Usage: &contracts.TokenUsage{InputTokens: 100, OutputTokens: 20},
			}, nil
		}

		// Second step: submit result
		return &runner.ModelTurnResult{
			Text: "Analysis finished",
			ToolCalls: []contracts.ToolCallRecord{
				{
					ID:   "call_2",
					Name: "submitResult",
					Input: map[string]any{
						"reasoning": "Identified hardcoded secret in auth.go",
						"suggestions": []map[string]any{
							{
								"relevantFile":       "auth.go",
								"language":           "go",
								"label":              "security",
								"suggestionContent":  "Hardcoded token in auth.go",
								"existingCode":       "token := \"secret\"",
								"improvedCode":       "token := os.Getenv(\"AUTH_TOKEN\")",
								"oneSentenceSummary": "Move token to environment variable",
								"relevantLinesStart": 5,
								"relevantLinesEnd":   6,
								"severity":           "critical",
								"confidence":         0.98,
							},
						},
					},
				},
			},
			Usage: &contracts.TokenUsage{InputTokens: 120, OutputTokens: 30},
		}, nil
	}

	agentRunner := runner.NewGoAgentRunner(invoker)

	evaluator, err := rules.NewEvaluator(rules.DefaultCatalog())
	require.NoError(t, err)

	stage := stages.NewAgentDeliberationStage(
		evaluator,
		stages.WithAgentRunner(agentRunner),
		stages.WithEngineMode("core"),
	)

	patch := &diff.FilePatch{
		OldPath: "auth.go",
		NewPath: "auth.go",
		Hunks: []diff.Hunk{
			{
				OldStart: 1,
				OldLines: 10,
				NewStart: 1,
				NewLines: 10,
				Lines: []diff.DiffLine{
					{Type: diff.LineAddition, NewLineNo: 5, Content: "token := \"secret\""},
				},
			},
		},
	}

	reviewID := uuid.New()
	wsID := uuid.New()
	pCtx := &pipeline.PipelineContext{
		ReviewID:        reviewID,
		WorkspaceID:     wsID,
		PullNumber:      42,
		Title:           "Add auth handler",
		Author:          "dev",
		FilteredPatches: []*diff.FilePatch{patch},
	}

	err = stage.Execute(ctx, pCtx)
	require.NoError(t, err)

	require.Len(t, pCtx.AgentFindings, 1)
	finding := pCtx.AgentFindings[0]
	assert.Equal(t, "auth.go", finding.FilePath)
	assert.Equal(t, 5, finding.StartLine)
	assert.Equal(t, 6, finding.EndLine)
	assert.Equal(t, models.SeverityCritical, finding.Severity)
	assert.Equal(t, "security", finding.Category)
	assert.Equal(t, "Move token to environment variable", finding.Title)
	assert.Equal(t, "Hardcoded token in auth.go", finding.Description)
	assert.Equal(t, "token := os.Getenv(\"AUTH_TOKEN\")", finding.Remediation)
	assert.Equal(t, reviewID, finding.ReviewID)
	assert.Equal(t, wsID, finding.WorkspaceID)
	assert.NotEmpty(t, finding.Fingerprint)
}

type mockPipelineSpecialist struct {
	name     string
	category string
	findings []orchestrator.AgentFinding
}

func (m *mockPipelineSpecialist) Name() string     { return m.name }
func (m *mockPipelineSpecialist) Category() string { return m.category }
func (m *mockPipelineSpecialist) Review(ctx context.Context, input orchestrator.ReviewAgentInput) (*orchestrator.ReviewAgentOutput, error) {
	return &orchestrator.ReviewAgentOutput{
		AgentName:      m.name,
		Category:       m.category,
		Findings:       m.findings,
		FinishReason:   "completed",
		TokensConsumed: 1500,
		DurationMs:     45,
	}, nil
}

func TestAgentDeliberationStage_OrchestratorEngine(t *testing.T) {
	findingID := uuid.New()
	finding := orchestrator.AgentFinding{
		ID:            findingID,
		AgentName:     "security",
		FilePath:      "pkg/auth/jwt.go",
		StartLine:     20,
		EndLine:       25,
		Severity:      models.SeverityCritical,
		Category:      "security",
		Title:         "Insecure JWT Secret",
		Description:   "Hardcoded default secret key used in JWT signing.",
		Remediation:   "Read secret from environment variable.",
		SuggestedDiff: "@@ -20,2 +20,2 @@\n-secret := \"default\"\n+secret := os.Getenv(\"JWT_SECRET\")\n",
		Blocking:      true,
	}

	mockSpec := &mockPipelineSpecialist{
		name:     "generalist",
		category: "generalist",
		findings: []orchestrator.AgentFinding{finding},
	}

	orchestratorSvc := orchestrator.NewReviewOrchestratorService(nil, nil, nil)
	orchestratorSvc.RegisterSpecialist(mockSpec)

	evaluator, err := rules.NewEvaluator(rules.DefaultCatalog())
	require.NoError(t, err)

	stage := stages.NewAgentDeliberationStage(
		evaluator,
		stages.WithOrchestratorService(orchestratorSvc),
		stages.WithEngineMode("orchestrator"),
	)

	patch := &diff.FilePatch{
		OldPath: "pkg/auth/jwt.go",
		NewPath: "pkg/auth/jwt.go",
		Hunks: []diff.Hunk{
			{
				OldStart: 20,
				OldLines: 5,
				NewStart: 20,
				NewLines: 5,
				Lines: []diff.DiffLine{
					{Type: diff.LineAddition, NewLineNo: 20, Content: "func Sign() {"},
					{Type: diff.LineAddition, NewLineNo: 21, Content: "secret := \"default\""},
					{Type: diff.LineAddition, NewLineNo: 22, Content: "return secret"},
					{Type: diff.LineAddition, NewLineNo: 23, Content: "}"},
				},
			},
		},
	}

	reviewID := uuid.New()
	wsID := uuid.New()
	pCtx := &pipeline.PipelineContext{
		ReviewID:        reviewID,
		WorkspaceID:     wsID,
		PullNumber:      99,
		Title:           "Add JWT auth",
		Author:          "alice",
		FilteredPatches: []*diff.FilePatch{patch},
	}

	err = stage.Execute(context.Background(), pCtx)
	require.NoError(t, err)

	require.Len(t, pCtx.AgentFindings, 1)
	f := pCtx.AgentFindings[0]
	assert.Equal(t, findingID, f.ID)
	assert.Equal(t, "pkg/auth/jwt.go", f.FilePath)
	assert.Equal(t, 20, f.StartLine)
	assert.Equal(t, 24, f.EndLine)
	assert.Equal(t, models.SeverityCritical, f.Severity)
	assert.Equal(t, "Insecure JWT Secret", f.Title)
	assert.NotEmpty(t, f.Fingerprint)
	assert.NotEmpty(t, pCtx.PRSummaryBody)
	assert.Contains(t, pCtx.PRSummaryBody, "ScanDrix AI Review: CHANGES REQUESTED")
}

