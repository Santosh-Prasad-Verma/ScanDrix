package agentcore_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/review/agentcore"
)

type mockRecallRunner struct {
	states   []*contracts.RunState
	runIndex int64
}

func (m *mockRecallRunner) Run(
	ctx context.Context,
	spec contracts.AgentSpec,
	input contracts.AgentRunInput,
	toolCtx contracts.ToolContext,
) (*contracts.RunState, error) {
	idx := atomic.AddInt64(&m.runIndex, 1) - 1
	if int(idx) < len(m.states) {
		return m.states[idx], nil
	}
	if len(m.states) > 0 {
		return m.states[len(m.states)-1], nil
	}
	return &contracts.RunState{Status: contracts.StatusCompleted}, nil
}

func (m *mockRecallRunner) GetRunCount() int {
	return int(atomic.LoadInt64(&m.runIndex))
}

func makeRunStateWithSuggestions(suggestions []agentcore.FinderSuggestion) *contracts.RunState {
	payload := agentcore.SubmitResultPayload{
		Reasoning:   "mock reasoning",
		Suggestions: suggestions,
	}

	return &contracts.RunState{
		Status: contracts.StatusCompleted,
		Artifacts: []contracts.Artifact{
			{
				Type:    agentcore.FinderDoneTool,
				Payload: payload,
			},
		},
		Steps: []contracts.RunStep{
			{
				Index: 0,
				Message: contracts.AgentMessage{
					Role: contracts.RoleAssistant,
					ToolCalls: []contracts.ToolCallRecord{
						{
							Name:  "readFile",
							Input: map[string]any{"path": "a.ts"},
						},
					},
				},
			},
		},
		Usage: contracts.TokenUsage{
			InputTokens:  10,
			OutputTokens: 4,
		},
	}
}

func TestRunRecallPasses_SkipsWhenSynthesisRescueDisabled(t *testing.T) {
	runner := &mockRecallRunner{
		states: []*contracts.RunState{
			makeRunStateWithSuggestions([]agentcore.FinderSuggestion{
				{RelevantFile: "b.ts", SuggestionContent: "missed bug"},
			}),
		},
	}

	baseSuggestions := []agentcore.FinderSuggestion{
		{RelevantFile: "a.ts", SuggestionContent: "bug1"},
	}

	params := agentcore.FinderAgentParams{
		SkipSynthesisRescue: true,
	}

	baseState := makeRunStateWithSuggestions(baseSuggestions)

	reasoning, findings, err := agentcore.RunRecallPasses(
		context.Background(),
		runner,
		params,
		contracts.AgentRunInput{Prompt: "review this"},
		contracts.ToolContext{RunID: "recall-test"},
		"base reasoning",
		baseSuggestions,
		baseState,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runner.GetRunCount() != 0 {
		t.Fatalf("expected 0 runs when SkipSynthesisRescue is true, got %d", runner.GetRunCount())
	}

	if len(findings) != 1 || findings[0].RelevantFile != "a.ts" {
		t.Fatalf("expected original findings preserved, got: %+v", findings)
	}

	if reasoning != "base reasoning" {
		t.Fatalf("expected original reasoning preserved, got: %s", reasoning)
	}
}

func TestRunRecallPasses_RunsSynthesisRescueAndMergesNewFindings(t *testing.T) {
	runner := &mockRecallRunner{
		states: []*contracts.RunState{
			makeRunStateWithSuggestions([]agentcore.FinderSuggestion{
				{RelevantFile: "e.ts", SuggestionContent: "missed bug in e"},
			}),
		},
	}

	baseSuggestions := []agentcore.FinderSuggestion{
		{RelevantFile: "a.ts", SuggestionContent: "bug in a"},
	}

	params := agentcore.FinderAgentParams{
		SkipSynthesisRescue: false,
	}

	baseState := makeRunStateWithSuggestions(baseSuggestions)

	_, findings, err := agentcore.RunRecallPasses(
		context.Background(),
		runner,
		params,
		contracts.AgentRunInput{Prompt: "review this"},
		contracts.ToolContext{RunID: "recall-test"},
		"base reasoning",
		baseSuggestions,
		baseState,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runner.GetRunCount() != 1 {
		t.Fatalf("expected exactly 1 synthesis run, got %d", runner.GetRunCount())
	}

	if len(findings) != 2 {
		t.Fatalf("expected 2 merged findings, got %d: %+v", len(findings), findings)
	}

	files := []string{findings[0].RelevantFile, findings[1].RelevantFile}
	if files[0] != "a.ts" || files[1] != "e.ts" {
		t.Fatalf("expected files [a.ts, e.ts], got %+v", files)
	}
}

func TestRunRecallPasses_DedupsIdenticalFindings(t *testing.T) {
	runner := &mockRecallRunner{
		states: []*contracts.RunState{
			makeRunStateWithSuggestions([]agentcore.FinderSuggestion{
				{RelevantFile: "a.ts", SuggestionContent: "bug in a"}, // identical to base
			}),
		},
	}

	baseSuggestions := []agentcore.FinderSuggestion{
		{RelevantFile: "a.ts", SuggestionContent: "bug in a"},
	}

	params := agentcore.FinderAgentParams{
		SkipSynthesisRescue: false,
	}

	baseState := makeRunStateWithSuggestions(baseSuggestions)

	_, findings, err := agentcore.RunRecallPasses(
		context.Background(),
		runner,
		params,
		contracts.AgentRunInput{Prompt: "review this"},
		contracts.ToolContext{RunID: "recall-test"},
		"base reasoning",
		baseSuggestions,
		baseState,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runner.GetRunCount() != 1 {
		t.Fatalf("expected 1 run, got %d", runner.GetRunCount())
	}

	if len(findings) != 1 {
		t.Fatalf("expected deduplication to keep 1 finding, got %d", len(findings))
	}
}

func TestRunRecallPasses_HeavyResampleMode(t *testing.T) {
	runner := &mockRecallRunner{
		states: []*contracts.RunState{
			makeRunStateWithSuggestions([]agentcore.FinderSuggestion{
				{RelevantFile: "rescue.ts", SuggestionContent: "rescue finding"},
			}),
			makeRunStateWithSuggestions([]agentcore.FinderSuggestion{
				{RelevantFile: "resample1.ts", SuggestionContent: "resample 1 finding"},
			}),
			makeRunStateWithSuggestions([]agentcore.FinderSuggestion{
				{RelevantFile: "resample2.ts", SuggestionContent: "resample 2 finding"},
			}),
		},
	}

	baseSuggestions := []agentcore.FinderSuggestion{
		{RelevantFile: "base.ts", SuggestionContent: "base finding"},
	}

	params := agentcore.FinderAgentParams{
		HeavyMode:           true,
		HeavyResampleRuns:   2,
		SkipSynthesisRescue: false,
		MakeResampleSpec: func() contracts.AgentSpec {
			return agentcore.BuildFinderAgentSpec(agentcore.FinderAgentParams{})
		},
	}

	baseState := makeRunStateWithSuggestions(baseSuggestions)

	_, findings, err := agentcore.RunRecallPasses(
		context.Background(),
		runner,
		params,
		contracts.AgentRunInput{Prompt: "heavy review"},
		contracts.ToolContext{RunID: "heavy-test"},
		"base reasoning",
		baseSuggestions,
		baseState,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1 synthesis rescue + 2 heavy resample runs = 3 runs total
	if runner.GetRunCount() != 3 {
		t.Fatalf("expected 3 runs (1 rescue + 2 resamples), got %d", runner.GetRunCount())
	}

	// 1 base + 1 rescue + 2 resamples = 4 findings
	if len(findings) != 4 {
		t.Fatalf("expected 4 findings merged from heavy passes, got %d: %+v", len(findings), findings)
	}
}
