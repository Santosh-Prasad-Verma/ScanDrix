// Package agentcore provides the deep core agent loop and execution components for code reviews.
package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/tools"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/verify"
)

const (
	VerifyDoneTool = "submitVerdict"
)

// VerdictSchema defines the structured output schema for the verifier agent.
var VerdictSchema = contracts.JSONSchema{
	Type: "object",
	Properties: map[string]contracts.JSONSchema{
		"keep":       {Type: "boolean", Description: "true to keep the finding, false to refute as false-positive"},
		"rationale":  {Type: "string", Description: "Technical proof confirming or refuting the issue"},
		"confidence": {Type: "string", Enum: []string{"high", "medium", "low"}},
	},
	Required: []string{"keep", "rationale"},
}

// BuildVerifierSpecParams contains parameters to configure the verifier agent spec.
type BuildVerifierSpecParams struct {
	ModelID        string
	FallbackModel  string
	AgentName      string
	RunName        string
	Tools          contracts.ToolRegistry
	MaxSteps       int
	LightMaxSteps  int
	FullMaxSteps   int
	ForceFull      bool
	SystemPrompt   string
}

// VerifyUsage tracks token usage across verification runs.
type VerifyUsage struct {
	InputTokens     int64 `json:"inputTokens"`
	OutputTokens    int64 `json:"outputTokens"`
	ReasoningTokens int64 `json:"reasoningTokens"`
	CacheReadTokens int64 `json:"cacheReadTokens"`
}

// BuildVerifierPrompt generates the surgical HV2 verifier prompt (refute-to-drop).
func BuildVerifierPrompt(evidenceBundle string, index int) (system string, prompt string) {
	system = `You are a surgical code review verifier.

Your task is to verify ONE candidate finding: confirm or REFUTE its technical claim.
You are NOT re-deciding whether it is "worth reporting" — the finder already promoted it.
Your job is correctness, not taste. The bar to remove a finding is a REFUTATION, not a doubt.

Rules:
- You may use only a few tool calls. Be surgical.
- Use tools to confirm or REFUTE the candidate finding.
- Treat call graph hints as fast navigation hints, not as final proof.
- You must NOT create a new finding unrelated to the candidate.
- Do NOT rewrite the finding text, summary, severity, or suggested fix.

DROP the finding ONLY if you can actively REFUTE it — concrete evidence that it is wrong or cannot happen:
- The root cause described is factually wrong (e.g. claims something is not imported when it is; claims a value can be null when it provably cannot).
- The failure path is impossible given the actual code: a guard upstream prevents it, the branch is unreachable, or the value is already validated before use.
- It is pure code style, naming, documentation, or formatting — not a behavior bug.
- It is a generic "missing X" suggestion (missing rate limit / validation / CSRF / auth) with NO concrete code path where the omission produces a wrong outcome.

KEEP the finding (this is the DEFAULT) whenever you cannot refute it. Do NOT drop a finding merely because:
- the trigger is concurrent, adversarial, or an edge condition — race conditions, SSRF, auth/FIPS bypass, and injection are REAL bugs, not "speculative" or "extreme";
- the root cause is reached from a caller in another file — cross-file bugs are real; trace the path before judging;
- the bug is not literally on a changed line, as long as the PR's change activates, exposes, or fails to guard it.

When in doubt, KEEP — a human reviewer makes the final call. Recall of real defects matters more here than trimming the last few low-value findings.

Return JSON only at the end.`

	prompt = fmt.Sprintf(`%s

Recommended approach:
1. Read the cited file/range if needed.
2. Search for the key symbol or caller if the claim depends on flow.
3. Read one relevant caller/callee file if needed.
4. Return a final JSON verdict via submitVerdict.

Submit verdict with:
- keep: true | false
- rationale: why the evidence supports keep or refute
- confidence: high | medium | low`, evidenceBundle)

	return system, prompt
}

// VerifierPromptFor formats a single candidate finding into an evidence bundle for the verifier.
func VerifierPromptFor(finding FinderSuggestion) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("File: %s", finding.RelevantFile))
	if finding.RelevantLinesStart > 0 {
		end := finding.RelevantLinesEnd
		if end <= 0 {
			end = finding.RelevantLinesStart
		}
		lines = append(lines, fmt.Sprintf("Lines: %d-%d", finding.RelevantLinesStart, end))
	}
	if finding.Severity != "" {
		lines = append(lines, fmt.Sprintf("Severity: %s", finding.Severity))
	}
	lines = append(lines, fmt.Sprintf("Claim: %s", finding.SuggestionContent))
	if finding.ExistingCode != "" {
		lines = append(lines, fmt.Sprintf("Existing Code:\n```\n%s\n```", finding.ExistingCode))
	}
	if finding.ImprovedCode != "" {
		lines = append(lines, fmt.Sprintf("Proposed Fix:\n```\n%s\n```", finding.ImprovedCode))
	}
	return strings.Join(lines, "\n")
}

// BuildVerifierAgentSpec constructs an AgentSpec for the finding verifier.
func BuildVerifierAgentSpec(params BuildVerifierSpecParams) contracts.AgentSpec {
	submitVerdictTool := &BaseAgentTool{
		ToolName: VerifyDoneTool,
		ToolDesc: "Submit your verdict for the candidate finding (keep=true unless you can mathematically or structurally REFUTE it).",
		Schema:   VerdictSchema,
		IsStrict: true,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			return contracts.ToolResult{Output: "verdict recorded"}, nil
		},
	}

	var toolList []contracts.AgentTool
	if params.Tools != nil {
		toolList = append(toolList, params.Tools.List()...)
	}
	toolList = append(toolList, submitVerdictTool)
	registry := tools.NewInMemoryToolRegistry(toolList...)

	systemPrompt := params.SystemPrompt
	if systemPrompt == "" {
		sys, _ := BuildVerifierPrompt("", 0)
		systemPrompt = sys
	}

	maxSteps := params.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 5
		if val := os.Getenv("SCANDRIX_VERIFIER_MAX_STEPS"); val != "" {
			if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
				maxSteps = parsed
			}
		}
	}

	runName := params.RunName
	if runName == "" {
		runName = "code-review-verifier"
	}

	return contracts.AgentSpec{
		ID:             "verifier",
		RunName:        runName,
		AgentName:      params.AgentName,
		Phase:          "verify",
		SystemPrompt:   systemPrompt,
		Tools:          registry,
		MaxSteps:       maxSteps,
		ResultToolName: VerifyDoneTool,
	}
}

// SuggestionVerifier adapts the agent harness runner into contracts.Verifier[FinderSuggestion].
// It implements the confidence split: high-confidence -> light verify, low-confidence -> full verify.
type SuggestionVerifier struct {
	runner contracts.AgentRunner
	params BuildVerifierSpecParams

	mu    sync.RWMutex
	usage VerifyUsage

	inputTokensAcc     atomic.Int64
	outputTokensAcc    atomic.Int64
	reasoningTokensAcc atomic.Int64
	cacheReadTokensAcc atomic.Int64
}

// NewSuggestionVerifier creates an enterprise verifier for review candidate suggestions.
func NewSuggestionVerifier(runner contracts.AgentRunner, params BuildVerifierSpecParams) *SuggestionVerifier {
	if params.LightMaxSteps <= 0 {
		params.LightMaxSteps = 5
	}
	if params.FullMaxSteps <= 0 {
		params.FullMaxSteps = 10
	}

	return &SuggestionVerifier{
		runner: runner,
		params: params,
	}
}

// Usage returns the accumulated token usage across all verify calls.
func (s *SuggestionVerifier) Usage() VerifyUsage {
	return VerifyUsage{
		InputTokens:     s.inputTokensAcc.Load(),
		OutputTokens:    s.outputTokensAcc.Load(),
		ReasoningTokens: s.reasoningTokensAcc.Load(),
		CacheReadTokens: s.cacheReadTokensAcc.Load(),
	}
}

// Verify implements contracts.Verifier[FinderSuggestion].
func (s *SuggestionVerifier) Verify(ctx context.Context, item FinderSuggestion, toolCtx contracts.ToolContext) (contracts.Verdict, error) {
	// Confidence split: high-confidence (>= 5 or >= 0.70) gets light verify; low gets full verify.
	useFull := s.params.ForceFull
	if !useFull {
		if item.Confidence > 0 && item.Confidence < 1.0 {
			useFull = item.Confidence < 0.70
		} else if item.Confidence > 0 {
			useFull = item.Confidence < 5.0
		}
	}

	maxSteps := s.params.LightMaxSteps
	if useFull {
		maxSteps = s.params.FullMaxSteps
	}

	specParams := s.params
	specParams.MaxSteps = maxSteps
	spec := BuildVerifierAgentSpec(specParams)

	evidenceBundle := VerifierPromptFor(item)
	_, userPrompt := BuildVerifierPrompt(evidenceBundle, 0)

	state, err := s.runner.Run(ctx, spec, contracts.AgentRunInput{
		Prompt: userPrompt,
	}, toolCtx)

	if state != nil {
		s.inputTokensAcc.Add(int64(state.Usage.InputTokens))
		s.outputTokensAcc.Add(int64(state.Usage.OutputTokens))
		s.reasoningTokensAcc.Add(int64(state.Usage.ReasoningTokens))
		s.cacheReadTokensAcc.Add(int64(state.Usage.CacheReadTokens))
	}

	if err != nil && state == nil {
		// Fail open: default to KEEP if verification could not execute
		return contracts.Verdict{
			Keep:      true,
			Rationale: fmt.Sprintf("Verification pass encountered runner error, kept candidate: %v", err),
		}, nil
	}

	// Extract verdict from state artifacts
	for i := len(state.Artifacts) - 1; i >= 0; i-- {
		art := state.Artifacts[i]
		if art.Type == VerifyDoneTool && art.Payload != nil {
			payloadBytes, err := json.Marshal(art.Payload)
			if err == nil {
				var verdict contracts.Verdict
				if err := json.Unmarshal(payloadBytes, &verdict); err == nil {
					return verdict, nil
				}
			}
		}
	}

	// Fail-open fallback using verify utility
	return verify.ExtractVerdict(state), nil
}
