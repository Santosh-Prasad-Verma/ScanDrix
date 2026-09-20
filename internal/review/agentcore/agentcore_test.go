// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.

package agentcore_test

import (
	"context"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/runner"
	"github.com/scandrix/backend/internal/review/agentcore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockFS struct {
	files map[string]string
}

func (m *mockFS) ReadFile(ctx context.Context, path string, startLine, endLine int) (string, error) {
	content, ok := m.files[path]
	if !ok {
		return "", assert.AnError
	}
	return content, nil
}

func (m *mockFS) ListDir(ctx context.Context, path string) ([]string, error) {
	var list []string
	for k := range m.files {
		list = append(list, k)
	}
	return list, nil
}

func (m *mockFS) Grep(ctx context.Context, query, path string) (string, error) {
	return "match: main.go:10", nil
}

func (m *mockFS) GetCallers(ctx context.Context, symbol, file string) (string, error) {
	return "caller: handler.go:25", nil
}

func (m *mockFS) GitDiff(ctx context.Context, path string) (string, error) {
	return "+ auth.ValidateToken()", nil
}

func TestDiffCoverageLedger(t *testing.T) {
	files := []agentcore.ChangedFile{
		{
			Filename: "pkg/auth/token.go",
			Hunks: []agentcore.DiffHunk{
				{NewStart: 10, NewLines: 20},
			},
		},
		{
			Filename: "internal/ui/view.go",
			Hunks: []agentcore.DiffHunk{
				{NewStart: 1, NewLines: 5},
			},
		},
	}

	ledger := agentcore.NewDiffCoverageLedger(agentcore.DiffCoverageLedgerParams{
		ChangedFiles: files,
	})

	sum := ledger.Summary()
	assert.Equal(t, 2, sum.TotalTargets)
	assert.Equal(t, 2, sum.PendingTargets)
	assert.Equal(t, 1, sum.CriticalTotal, "token.go matches critical keyword auth")
	assert.Equal(t, 1, sum.CriticalPending)
	assert.False(t, ledger.IsSatisfied())

	// Mark tool call on auth file
	ledger.MarkFromToolCall("readFile", map[string]any{
		"path":      "pkg/auth/token.go",
		"startLine": 10,
		"endLine":   25,
	}, 1)

	sumAfter := ledger.Summary()
	assert.Equal(t, 1, sumAfter.PendingTargets)
	assert.Equal(t, 0, sumAfter.CriticalPending)
	assert.True(t, ledger.IsSatisfied(), "Critical targets are now satisfied")
}

func TestFinderTools_CachingAndOutline(t *testing.T) {
	fs := &mockFS{
		files: map[string]string{
			"small.go": "package main\nfunc main() {}",
			"large.go": strings.Repeat("func handler() {}\n", 600),
		},
	}

	registry, cache := agentcore.BuildFinderToolRegistry(agentcore.FinderToolRegistryOptions{
		FS:               fs,
		EnableOutline:    true,
		OutlineThreshold: 500,
	})

	assert.NotNil(t, cache)
	assert.Len(t, registry.List(), 5)

	tool, found := registry.Get("readFile")
	require.True(t, found)

	toolCtx := contracts.ToolContext{
		RunID:   "test-run",
		Context: context.Background(),
	}

	// 1. Read large file -> triggers outline
	res, err := tool.Execute(toolCtx, map[string]any{"path": "large.go"})
	require.NoError(t, err)
	assert.Contains(t, res.Output, "[OUTLINE-FIRST: File large.go has")

	// 2. Read small file -> triggers normal read
	resSmall, err := tool.Execute(toolCtx, map[string]any{"path": "small.go"})
	require.NoError(t, err)
	assert.Contains(t, resSmall.Output, "package main")

	// 3. Cache verification
	assert.True(t, cache.Stats().Size > 0)
}

func TestCollapseNearDuplicates(t *testing.T) {
	findings := []agentcore.FinderSuggestion{
		{
			RelevantFile:      "auth.go",
			SuggestionContent: "Missing JWT expiration check on token validation endpoint",
			Confidence:        0.8,
		},
		{
			RelevantFile:      "auth.go",
			SuggestionContent: "Missing JWT expiration check on the token validation endpoint here",
			Confidence:        0.95, // Higher confidence
		},
		{
			RelevantFile:      "user.go",
			SuggestionContent: "SQL injection vulnerability in query",
			Confidence:        0.9,
		},
	}

	collapsed := agentcore.CollapseNearDuplicates(findings, 0.3)
	assert.Len(t, collapsed, 2, "Near duplicate on auth.go should be collapsed")
	assert.Equal(t, 0.95, collapsed[0].Confidence, "Should retain higher confidence duplicate")
}

func TestOverflowRecoveringRunner(t *testing.T) {
	attempts := 0
	tightened := false

	mockRunner := &mockAgentRunner{
		runFn: func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
			attempts++
			if attempts == 1 {
				return &contracts.RunState{
					Status: contracts.StatusError,
					Trace: []contracts.TraceEvent{
						{
							Kind: "error",
							Detail: map[string]any{
								"message": "context_length_exceeded: token limit exceeded",
							},
						},
					},
				}, nil
			}
			return &contracts.RunState{
				Status: contracts.StatusCompleted,
			}, nil
		},
	}

	tightener := func(spec contracts.AgentSpec, scale float64) contracts.AgentSpec {
		tightened = true
		assert.Equal(t, 0.60, scale)
		return spec
	}

	recovering := agentcore.NewOverflowRecoveringRunner(mockRunner, tightener)
	state, err := recovering.Run(context.Background(), contracts.AgentSpec{}, contracts.AgentRunInput{}, contracts.ToolContext{})
	require.NoError(t, err)
	assert.Equal(t, contracts.StatusCompleted, state.Status)
	assert.Equal(t, 2, attempts, "Should have retried once")
	assert.True(t, tightened, "Tightener should have been invoked")
}

type mockAgentRunner struct {
	runFn func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error)
}

func (m *mockAgentRunner) Run(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
	return m.runFn(ctx, spec, input, toolCtx)
}

func TestRunAgentLoopViaCore_EndToEnd(t *testing.T) {
	ctx := context.Background()

	turn := 0
	invoker := func(ctx context.Context, req runner.ModelTurnRequest) (*runner.ModelTurnResult, error) {
		turn++
		if turn == 1 {
			return &runner.ModelTurnResult{
				Text: "Inspecting auth.go",
				ToolCalls: []contracts.ToolCallRecord{
					{
						ID:   "call-read",
						Name: "readFile",
						Input: map[string]any{
							"path": "auth.go",
						},
					},
				},
				Usage: &contracts.TokenUsage{InputTokens: 100, OutputTokens: 20},
			}, nil
		}

		return &runner.ModelTurnResult{
			Text: "Analysis finished",
			ToolCalls: []contracts.ToolCallRecord{
				{
					ID:   "call-1",
					Name: agentcore.FinderDoneTool,
					Input: map[string]any{
						"suggestions": []map[string]any{
							{
								"relevantFile":       "auth.go",
								"suggestionContent":  "Hardcoded credentials",
								"existingCode":       "const password = \"123456\"",
								"improvedCode":       "password := os.Getenv(\"SECRET\")",
								"severity":           "critical",
								"confidence":         0.98,
								"relevantLinesStart": 10,
								"relevantLinesEnd":   12,
							},
						},
					},
				},
			},
			Usage: &contracts.TokenUsage{InputTokens: 150, OutputTokens: 40},
		}, nil
	}

	agentRunner := runner.NewGoAgentRunner(invoker)

	input := agentcore.ReviewAgentInput{
		PRNumber:       101,
		RepositoryName: "org/repo",
		AgentName:      "security",
		SystemPrompt:   "Find security vulnerabilities",
		UserPrompt:     "Review changes in auth.go",
		ChangedFiles: []agentcore.ChangedFile{
			{Filename: "auth.go"},
		},
		FS: &mockFS{
			files: map[string]string{
				"auth.go": "const password = \"123456\"",
			},
		},
		MaxSteps: 5,
	}

	out, err := agentcore.RunAgentLoopViaCore(ctx, agentRunner, input, contracts.ToolContext{
		RunID:   "core-review-run",
		Context: ctx,
	})

	require.NoError(t, err)
	assert.Equal(t, "security", out.AgentName)
	assert.Equal(t, 1, len(out.VerifiedFindings))
	assert.Equal(t, "Hardcoded credentials", out.VerifiedFindings[0].SuggestionContent)
	assert.Equal(t, 250, out.Usage.InputTokens)
}

func TestSpecializedAgentProviders_InitializationAndPrompting(t *testing.T) {
	bugProv := agentcore.NewBugAgentProvider()
	assert.Equal(t, agentcore.CategoryBug, bugProv.Category())
	assert.Equal(t, "scandrix-bug-review-agent", bugProv.Identity().Name)
	assert.Contains(t, bugProv.SystemPrompt([]string{"No raw panics allowed"}), "No raw panics allowed")

	secProv := agentcore.NewSecurityAgentProvider()
	assert.Equal(t, agentcore.CategorySecurity, secProv.Category())
	assert.Contains(t, secProv.SystemPrompt(nil), "OWASP Top 10")

	perfProv := agentcore.NewPerformanceAgentProvider()
	assert.Equal(t, agentcore.CategoryPerformance, perfProv.Category())
	assert.Contains(t, perfProv.SystemPrompt(nil), "N+1")

	rulesProv := agentcore.NewDrixyRulesAgentProvider()
	assert.Equal(t, agentcore.CategoryRules, rulesProv.Category())

	genProv := agentcore.NewGeneralistAgentProvider()
	assert.Equal(t, agentcore.CategoryGeneralist, genProv.Category())

	multiOrch := agentcore.NewMultiAgentReviewOrchestrator(agentcore.MultiAgentReviewConfig{
		EnableBugAgent:         true,
		EnableSecurityAgent:    true,
		EnablePerformanceAgent: false,
		EnableRulesAgent:       false,
		EnableGeneralistAgent:  false,
		MaxConcurrentAgents:    2,
		DedupSimilarityFloor:   0.30,
	})
	assert.NotNil(t, multiOrch)
}

