package agentcore

import (
	"context"
	"strings"
	"testing"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/tools"
)

func TestLooksLikeFindings(t *testing.T) {
	// True: mentions bug/vulnerability, fix/should, and file/line
	proseWithFindings := `
During code review of src/auth/token.go:42, I noticed a critical null pointer dereference bug.
The function does not validate claims before access, which causes a panic under high concurrency.
You must add a nil-check guard before invoking claims.Valid() so that unauthenticated requests are rejected.
`
	if !LooksLikeFindings(proseWithFindings) {
		t.Errorf("expected true for text with strong finding signals")
	}

	// False: general investigation notes
	proseInvestigation := `
I ran grep across the repository and found 15 references to the token service.
Let me list the files and look through the repository structure.
`
	if LooksLikeFindings(proseInvestigation) {
		t.Errorf("expected false for general investigation notes without finding signals")
	}

	// False: too short
	if LooksLikeFindings("bug in token.go") {
		t.Errorf("expected false for string under 80 characters")
	}
}

func TestExtractFindingsWithRecovery(t *testing.T) {
	// Case 1: Structured artifact already present
	stateWithArtifact := &contracts.RunState{
		Status: contracts.StatusCompleted,
		Artifacts: []contracts.Artifact{
			{
				Type: FinderDoneTool,
				Payload: map[string]any{
					"reasoning": "Audit completed.",
					"suggestions": []map[string]any{
						{
							"relevantFile":      "src/auth.go",
							"suggestionContent": "Missing validation",
							"existingCode":      "a := b",
							"improvedCode":      "if b != nil { a := b }",
						},
					},
				},
			},
		},
	}

	reasoning, suggestions, err := ExtractFindingsWithRecovery(context.Background(), stateWithArtifact, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suggestions) != 1 || suggestions[0].RelevantFile != "src/auth.go" {
		t.Errorf("unexpected suggestions from artifact: %+v", suggestions)
	}
	if reasoning != "Audit completed." {
		t.Errorf("unexpected reasoning: %s", reasoning)
	}

	// Case 2: Prose in reasoning, suggestions empty -> triggers recoverer
	stateWithProse := &contracts.RunState{
		Status: contracts.StatusCompleted,
		Artifacts: []contracts.Artifact{
			{
				Type: FinderDoneTool,
				Payload: map[string]any{
					"reasoning": "Found a critical security vulnerability in src/api/handler.go:88. Unsafe SQL injection in query building! You must fix this by using parameterized queries.",
					// suggestions omitted
				},
			},
		},
	}

	recovererCalled := false
	recoverer := func(ctx context.Context, prose string) ([]FinderSuggestion, error) {
		recovererCalled = true
		return []FinderSuggestion{
			{
				RelevantFile:      "src/api/handler.go",
				SuggestionContent: "SQL injection vulnerability",
				ExistingCode:      "db.Query(fmt.Sprintf(...))",
				ImprovedCode:      "db.Query(query, param)",
			},
		}, nil
	}

	_, recoveredSuggestions, err := ExtractFindingsWithRecovery(context.Background(), stateWithProse, recoverer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !recovererCalled {
		t.Errorf("expected recoverer to be called for prose findings")
	}
	if len(recoveredSuggestions) != 1 || recoveredSuggestions[0].RelevantFile != "src/api/handler.go" {
		t.Errorf("expected recovered suggestion, got: %+v", recoveredSuggestions)
	}
}

func TestStrongFilesFromRunAndInvestigation(t *testing.T) {
	state := &contracts.RunState{
		Steps: []contracts.RunStep{
			{
				Index: 1,
				Message: contracts.AgentMessage{
					Role: contracts.RoleAssistant,
					ToolCalls: []contracts.ToolCallRecord{
						{
							Name: "readFile",
							Input: map[string]any{
								"path": "internal/auth/token.go",
							},
						},
						{
							Name: "grep",
							Input: map[string]any{
								"query": "TokenService",
							},
						},
					},
				},
			},
			{
				Index: 2,
				Message: contracts.AgentMessage{
					Role: contracts.RoleAssistant,
					ToolCalls: []contracts.ToolCallRecord{
						{
							Name: "checkTypes",
							Input: map[string]any{
								"file": "internal/billing/stripe.go",
							},
						},
					},
				},
			},
		},
	}

	strong := StrongFilesFromRun(state)
	if len(strong) != 2 {
		t.Fatalf("expected 2 strong files, got %d", len(strong))
	}

	if !FileWasInvestigated(strong, "internal/auth/token.go") {
		t.Errorf("expected internal/auth/token.go to be investigated")
	}
	if !FileWasInvestigated(strong, "/repo/internal/billing/stripe.go") {
		t.Errorf("expected suffix match on internal/billing/stripe.go")
	}
	if FileWasInvestigated(strong, "internal/random/file.go") {
		t.Errorf("uninvestigated file should return false")
	}
}

func TestSuggestionVerifier_ConfidenceSplitAndVerdict(t *testing.T) {
	var executedMaxSteps int
	mock := &mockRunner{
		runFn: func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
			executedMaxSteps = spec.MaxSteps
			return &contracts.RunState{
				Status: contracts.StatusCompleted,
				Usage: contracts.TokenUsage{
					InputTokens:  100,
					OutputTokens: 50,
				},
				Artifacts: []contracts.Artifact{
					{
						Type: VerifyDoneTool,
						Payload: map[string]any{
							"keep":       true,
							"rationale":  "Verified bug exists in cited file.",
							"confidence": "high",
						},
					},
				},
			}, nil
		},
	}

	v := NewSuggestionVerifier(mock, BuildVerifierSpecParams{
		LightMaxSteps: 4,
		FullMaxSteps:  9,
		Tools:         tools.NewInMemoryToolRegistry(),
	})

	// Test 1: High confidence finding -> uses light steps (4)
	highConfFinding := FinderSuggestion{
		RelevantFile:       "src/token.go",
		SuggestionContent:  "Memory leak in goroutine",
		Confidence:         0.85,
		RelevantLinesStart: 20,
	}
	verdict, err := v.Verify(context.Background(), highConfFinding, contracts.ToolContext{})
	if err != nil {
		t.Fatalf("unexpected verify error: %v", err)
	}
	if !verdict.Keep {
		t.Errorf("expected keep=true")
	}
	if executedMaxSteps != 4 {
		t.Errorf("expected light max steps (4) for high confidence, got %d", executedMaxSteps)
	}

	// Test 2: Low confidence finding -> uses full steps (9)
	lowConfFinding := FinderSuggestion{
		RelevantFile:       "src/token.go",
		SuggestionContent:  "Possible deadlock",
		Confidence:         0.40,
		RelevantLinesStart: 50,
	}
	_, _ = v.Verify(context.Background(), lowConfFinding, contracts.ToolContext{})
	if executedMaxSteps != 9 {
		t.Errorf("expected full max steps (9) for low confidence, got %d", executedMaxSteps)
	}

	// Check token usage tracking
	usage := v.Usage()
	if usage.InputTokens != 200 || usage.OutputTokens != 100 {
		t.Errorf("unexpected accumulated usage: %+v", usage)
	}
}

func TestRunFinderWithVerification_EvidenceGate(t *testing.T) {
	// Runner simulates:
	// 1. Initial finder run finds 2 findings:
	//    - finding 1 on "src/auth.go" (which the finder inspected via readFile)
	//    - finding 2 on "src/uninspected.go" (which the finder DID NOT inspect)
	// 2. Initial verify passes both
	// 3. Evidence gate runs full-verify on "src/uninspected.go" and DROPS it (keep=false)
	mock := &mockRunner{
		runFn: func(ctx context.Context, spec contracts.AgentSpec, input contracts.AgentRunInput, toolCtx contracts.ToolContext) (*contracts.RunState, error) {
			if spec.Phase == "finder" {
				return &contracts.RunState{
					Status: contracts.StatusCompleted,
					Steps: []contracts.RunStep{
						{
							Index: 1,
							Message: contracts.AgentMessage{
								Role: contracts.RoleAssistant,
								ToolCalls: []contracts.ToolCallRecord{
									{
										Name:  "readFile",
										Input: map[string]any{"path": "src/auth.go"},
									},
								},
							},
						},
					},
					Artifacts: []contracts.Artifact{
						{
							Type: FinderDoneTool,
							Payload: map[string]any{
								"suggestions": []map[string]any{
									{
										"relevantFile":      "src/auth.go",
										"suggestionContent": "Auth bypass",
										"existingCode":      "old1",
										"improvedCode":      "new1",
										"confidence":        0.9,
									},
									{
										"relevantFile":      "src/uninspected.go",
										"suggestionContent": "Hallucinated issue",
										"existingCode":      "old2",
										"improvedCode":      "new2",
										"confidence":        0.9,
									},
								},
							},
						},
					},
				}, nil
			}

			// Verifier phase:
			// If prompt contains "uninspected.go" and spec is from evidence-gate (forceFull), drop it
			isUninspected := strings.Contains(input.Prompt, "src/uninspected.go")
			if isUninspected && spec.MaxSteps == 10 {
				return &contracts.RunState{
					Status: contracts.StatusCompleted,
					Artifacts: []contracts.Artifact{
						{
							Type: VerifyDoneTool,
							Payload: map[string]any{
								"keep":      false,
								"rationale": "Refuted: code does not contain claimed issue",
							},
						},
					},
				}, nil
			}

			// Keep otherwise
			return &contracts.RunState{
				Status: contracts.StatusCompleted,
				Artifacts: []contracts.Artifact{
					{
						Type: VerifyDoneTool,
						Payload: map[string]any{
							"keep":      true,
							"rationale": "Confirmed bug",
						},
					},
				},
			}, nil
		},
	}

	verifier := NewSuggestionVerifier(mock, BuildVerifierSpecParams{
		LightMaxSteps: 5,
		FullMaxSteps:  10,
		Tools:         tools.NewInMemoryToolRegistry(),
	})

	kept, _, err := RunFinderWithVerification(
		context.Background(),
		mock,
		FinderAgentParams{
			AgentID:             "finder-test",
			SkipSynthesisRescue: true, // skip recall to focus on evidence gate
			Tools:               tools.NewInMemoryToolRegistry(),
		},
		contracts.AgentRunInput{Prompt: "Review PR"},
		contracts.ToolContext{},
		verifier,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only finding on "src/auth.go" should survive; "src/uninspected.go" dropped by evidence gate
	if len(kept) != 1 || kept[0].RelevantFile != "src/auth.go" {
		t.Errorf("expected 1 kept finding on src/auth.go, got: %+v", kept)
	}
}
