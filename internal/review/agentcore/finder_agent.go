// Package agentcore provides the deep core agent loop and execution components for code reviews.
package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/domain"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/orchestration"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/policies"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/tools"
)

const (
	FinderDoneTool = "submitResult"

	DefaultHeavyResampleExtraRuns = 2
	DefaultDedupSimilarity        = 0.30 // Calibrated on benchmark suite
)

// FinderSuggestion represents a candidate code review issue found by the finder agent.
type FinderSuggestion struct {
	RelevantFile       string   `json:"relevantFile"`
	Language           string   `json:"language,omitempty"`
	Label              string   `json:"label,omitempty"` // "bug" | "security" | "performance"
	SuggestionContent  string   `json:"suggestionContent"`
	ExistingCode       string   `json:"existingCode"`
	ImprovedCode       string   `json:"improvedCode"`
	OneSentenceSummary string   `json:"oneSentenceSummary,omitempty"`
	RelevantLinesStart int      `json:"relevantLinesStart,omitempty"`
	RelevantLinesEnd   int      `json:"relevantLinesEnd,omitempty"`
	Severity           string   `json:"severity,omitempty"` // "critical" | "high" | "medium" | "low"
	Confidence         float64  `json:"confidence,omitempty"`
	RuleUUID           string   `json:"ruleUuid,omitempty"`
	Tags               []string `json:"tags,omitempty"`
}

// SubmitResultPayload mirrors the structured output of submitResult.
type SubmitResultPayload struct {
	Reasoning   string             `json:"reasoning,omitempty"`
	Suggestions []FinderSuggestion `json:"suggestions"`
}

// SubmitResultSchema is the strict JSON schema required by structured output and function calling.
var SubmitResultSchema = contracts.JSONSchema{
	Type: "object",
	Properties: map[string]contracts.JSONSchema{
		"reasoning": {Type: "string", Description: "Step-by-step audit rationale"},
		"suggestions": {
			Type: "array",
			Items: &contracts.JSONSchema{
				Type: "object",
				Properties: map[string]contracts.JSONSchema{
					"relevantFile":       {Type: "string"},
					"language":           {Type: "string"},
					"label":              {Type: "string", Enum: []string{"bug", "security", "performance"}},
					"suggestionContent":  {Type: "string"},
					"existingCode":       {Type: "string"},
					"improvedCode":       {Type: "string"},
					"oneSentenceSummary": {Type: "string"},
					"relevantLinesStart": {Type: "integer"},
					"relevantLinesEnd":   {Type: "integer"},
					"severity":           {Type: "string", Enum: []string{"critical", "high", "medium", "low"}},
					"confidence":         {Type: "number"},
					"ruleUuid":           {Type: "string"},
				},
				Required: []string{"relevantFile", "suggestionContent", "existingCode", "improvedCode"},
			},
		},
	},
	Required: []string{"suggestions"},
}

// FinderAgentParams holds configuration for the code review finder agent.
type FinderAgentParams struct {
	AgentID               string
	AgentName             string
	SystemPrompt          string
	Tools                 contracts.ToolRegistry
	ProgressLedger        contracts.ProgressLedger
	Compressor            contracts.Compressor
	MaxSteps              int
	MaxTokens             int
	HeavyMode             bool
	HeavyResampleRuns     int
	DedupSimilarityFloor  float64
	EnableCompletionGate  bool
	EnableForceFinalize   bool
	EnableBudgetPolicy    bool
	EnableCompressionGate bool
	SkipSynthesisRescue   bool
	RecoverProse          ProseRecoverer
	MakeResampleSpec      func() contracts.AgentSpec
}

// BuildFinderAgentSpec constructs the production AgentSpec assembled on agent-harness.
func BuildFinderAgentSpec(params FinderAgentParams) contracts.AgentSpec {
	submitTool := &BaseAgentTool{
		ToolName: FinderDoneTool,
		ToolDesc: "Submit your final verified review findings. Once submitted, the agent run terminates.",
		Schema:   SubmitResultSchema,
		IsStrict: true,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			rawJSON, _ := json.Marshal(input)
			return contracts.ToolResult{
				Output: fmt.Sprintf("Recorded %d findings", len(rawJSON)),
			}, nil
		},
	}

	var toolList []contracts.AgentTool
	if params.Tools != nil {
		toolList = params.Tools.List()
	}
	allTools := append(toolList, submitTool)
	registry := tools.NewInMemoryToolRegistry(allTools...)

	activePolicies := make([]contracts.AgentPolicy, 0)

	// 1. Budget policy
	if params.EnableBudgetPolicy {
		activePolicies = append(activePolicies, policies.NewBudgetPolicy(policies.DefaultBudgetPolicyOptions()))
	}

	// 2. Completion gate policy
	if params.EnableCompletionGate && params.ProgressLedger != nil {
		activePolicies = append(activePolicies, policies.NewCompletionGatePolicy(params.ProgressLedger, policies.CompletionGatePolicyOptions{
			DoneToolName: FinderDoneTool,
		}))
	}

	// 3. Force finalize policy
	if params.EnableForceFinalize {
		activePolicies = append(activePolicies, policies.NewForceFinalizePolicy(policies.ForceFinalizePolicyOptions{
			DoneToolName: FinderDoneTool,
		}))
	}

	// 4. Compression policy
	if params.EnableCompressionGate && params.Compressor != nil {
		activePolicies = append(activePolicies, policies.NewCompressionPolicy(params.Compressor))
	}

	maxSteps := params.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 15
		if val := os.Getenv("SCANDRIX_FINDER_MAX_STEPS"); val != "" {
			if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
				maxSteps = parsed
			}
		}
	}

	return contracts.AgentSpec{
		ID:             params.AgentID,
		AgentName:      params.AgentName,
		Phase:          "finder",
		SystemPrompt:   params.SystemPrompt,
		Tools:          registry,
		Policies:       activePolicies,
		MaxSteps:       maxSteps,
		ResultToolName: FinderDoneTool,
	}
}

// ProseRecoverer is an injected capability that restructures prose reasoning into FinderSuggestions.
type ProseRecoverer func(ctx context.Context, reasoning string) ([]FinderSuggestion, error)

var (
	findingVerbRegex     = regexp.MustCompile(`(?i)\b(bug|issue|vulnerabilit|race|leak|npe|null|missing|incorrect|unsafe|injection|overflow|deadlock|toctou|panic)\b`)
	findingSolutionRegex = regexp.MustCompile(`(?i)\b(should|must|fix|instead|because|so that|would|remediate)\b`)
	findingLocationRegex = regexp.MustCompile(`(?i)(\.(go|ts|tsx|js|jsx|py|rb|rs|java|cs|cpp|c|h)\b|:\d+|line\s*\d+)`)
)

// LooksLikeFindings checks whether prose text contains sufficient technical signals of code review findings.
func LooksLikeFindings(text string) bool {
	if len(text) < 80 {
		return false
	}
	signals := 0
	if findingVerbRegex.MatchString(text) {
		signals++
	}
	if findingSolutionRegex.MatchString(text) {
		signals++
	}
	if findingLocationRegex.MatchString(text) {
		signals++
	}
	return signals >= 2
}

// ExtractFindings reads findings from a finished RunState artifacts or fallback text.
func ExtractFindings(state *contracts.RunState) (string, []FinderSuggestion) {
	if state == nil {
		return "", nil
	}

	// 1. Result-tool artifact (submitResult), latest first
	artifact, ok := domain.LastArtifact(state, FinderDoneTool)
	if ok && artifact.Payload != nil {
		payloadBytes, err := json.Marshal(artifact.Payload)
		if err == nil {
			var payload SubmitResultPayload
			if err := json.Unmarshal(payloadBytes, &payload); err == nil && len(payload.Suggestions) > 0 {
				return payload.Reasoning, payload.Suggestions
			}

			// Try direct array
			var direct []FinderSuggestion
			if err := json.Unmarshal(payloadBytes, &direct); err == nil && len(direct) > 0 {
				return "", direct
			}

			// Capture prose reasoning if present
			if payload.Reasoning != "" {
				// Fall back to scanning steps for JSON, or return prose
				if _, textSugg := findingsFromText(state); len(textSugg) > 0 {
					return payload.Reasoning, textSugg
				}
				return payload.Reasoning, nil
			}
		}
	}

	// 2. Fallback to scanning steps from latest to earliest
	return findingsFromText(state)
}

func findingsFromText(state *contracts.RunState) (string, []FinderSuggestion) {
	if state == nil {
		return "", nil
	}

	for i := len(state.Steps) - 1; i >= 0; i-- {
		step := state.Steps[i]
		contentStr, ok := step.Message.Content.(string)
		if !ok || len(contentStr) == 0 {
			continue
		}

		jsonStr := extractJSONBlock(contentStr)
		if jsonStr == "" {
			continue
		}

		var payload SubmitResultPayload
		if err := json.Unmarshal([]byte(jsonStr), &payload); err == nil && len(payload.Suggestions) > 0 {
			return payload.Reasoning, payload.Suggestions
		}

		var direct []FinderSuggestion
		if err := json.Unmarshal([]byte(jsonStr), &direct); err == nil && len(direct) > 0 {
			return "", direct
		}
	}

	return "", nil
}

func extractJSONBlock(text string) string {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "```json") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
		if idx := strings.LastIndex(trimmed, "```"); idx != -1 {
			trimmed = trimmed[:idx]
		}
		return strings.TrimSpace(trimmed)
	}
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```")
		if idx := strings.LastIndex(trimmed, "```"); idx != -1 {
			trimmed = trimmed[:idx]
		}
		return strings.TrimSpace(trimmed)
	}

	// Scan for outermost { ... } or [ ... ]
	startObj := strings.Index(trimmed, "{")
	startArr := strings.Index(trimmed, "[")

	if startObj != -1 && (startArr == -1 || startObj < startArr) {
		endObj := strings.LastIndex(trimmed, "}")
		if endObj > startObj {
			return trimmed[startObj : endObj+1]
		}
	} else if startArr != -1 {
		endArr := strings.LastIndex(trimmed, "]")
		if endArr > startArr {
			return trimmed[startArr : endArr+1]
		}
	}

	return ""
}

// ExtractFindingsWithRecovery extracts findings and triggers prose recovery if structured output was omitted.
func ExtractFindingsWithRecovery(ctx context.Context, state *contracts.RunState, recover ProseRecoverer) (string, []FinderSuggestion, error) {
	reasoning, suggestions := ExtractFindings(state)
	if len(suggestions) > 0 || recover == nil {
		return reasoning, suggestions, nil
	}

	if LooksLikeFindings(reasoning) {
		recovered, err := recover(ctx, reasoning)
		if err == nil && len(recovered) > 0 {
			return reasoning, recovered, nil
		}
	}

	return reasoning, suggestions, nil
}

// StrongFilesFromRun collects normalized paths of files inspected via readFile or checkTypes.
func StrongFilesFromRun(state *contracts.RunState) map[string]struct{} {
	out := make(map[string]struct{})
	if state == nil {
		return out
	}

	for _, step := range state.Steps {
		for _, tc := range step.Message.ToolCalls {
			if tc.Name != "readFile" && tc.Name != "checkTypes" {
				continue
			}

			inputMap, ok := tc.Input.(map[string]any)
			if !ok {
				continue
			}

			path := ""
			for _, key := range []string{"path", "filePath", "file"} {
				if val, ok := inputMap[key].(string); ok && val != "" {
					path = val
					break
				}
			}

			if path != "" {
				out[NormalizeRepoPath(path)] = struct{}{}
			}
		}
	}

	return out
}

// FileWasInvestigated checks whether the given file or a matching suffix was investigated.
func FileWasInvestigated(investigated map[string]struct{}, file string) bool {
	if len(file) == 0 || len(investigated) == 0 {
		return false
	}
	f := NormalizeRepoPath(file)
	for s := range investigated {
		if PathsMatch(s, f) {
			return true
		}
	}
	return false
}

// RunRecallPasses executes synthesis rescue and heavy resample passes.
func RunRecallPasses(
	ctx context.Context,
	runner contracts.AgentRunner,
	params FinderAgentParams,
	input contracts.AgentRunInput,
	toolCtx contracts.ToolContext,
	baseReasoning string,
	baseSuggestions []FinderSuggestion,
	baseState *contracts.RunState,
) (string, []FinderSuggestion, error) {
	findings := baseSuggestions
	reasoning := baseReasoning

	// 1. Synthesis rescue pass: re-think from evidence already gathered
	if !params.SkipSynthesisRescue && baseState != nil {
		inspected := StrongFilesFromRun(baseState)
		toolCalls := collectToolCallsFromState(baseState)
		synthesisPrompt := buildSynthesisPrompt(input.Prompt, inspected, toolCalls, findings)

		spec := BuildFinderAgentSpec(params)
		rescueState, rescueErr := runner.Run(ctx, spec, contracts.AgentRunInput{
			Prompt: synthesisPrompt,
		}, toolCtx)

		if rescueErr == nil && rescueState != nil {
			rescueReasoning, rescueFindings, _ := ExtractFindingsWithRecovery(ctx, rescueState, params.RecoverProse)
			if len(rescueReasoning) > 0 {
				reasoning += "\n\n" + rescueReasoning
			}
			findings = mergeSuggestions(findings, rescueFindings)
		}
	}

	// 2. Heavy mode resampling
	heavyRuns := params.HeavyResampleRuns
	if heavyRuns <= 0 && params.HeavyMode {
		heavyRuns = DefaultHeavyResampleExtraRuns
	}

	if params.HeavyMode && heavyRuns > 0 {
		var wg sync.WaitGroup
		var mu sync.Mutex
		var additionalFindings [][]FinderSuggestion

		for i := 0; i < heavyRuns; i++ {
			wg.Add(1)
			go func(runIndex int) {
				defer wg.Done()
				var spec contracts.AgentSpec
				if params.MakeResampleSpec != nil {
					spec = params.MakeResampleSpec()
				} else {
					spec = BuildFinderAgentSpec(params)
				}

				resampleState, err := runner.Run(ctx, spec, input, toolCtx)
				if err == nil && resampleState != nil {
					_, resampleFindings, _ := ExtractFindingsWithRecovery(ctx, resampleState, params.RecoverProse)
					if len(resampleFindings) > 0 {
						mu.Lock()
						additionalFindings = append(additionalFindings, resampleFindings)
						mu.Unlock()
					}
				}
			}(i)
		}
		wg.Wait()

		for _, extra := range additionalFindings {
			findings = mergeSuggestions(findings, extra)
		}
	}

	return reasoning, findings, nil
}

func collectToolCallsFromState(state *contracts.RunState) []string {
	var calls []string
	if state == nil {
		return calls
	}
	for _, step := range state.Steps {
		for _, tc := range step.Message.ToolCalls {
			argsStr := ""
			if b, err := json.Marshal(tc.Input); err == nil {
				argsStr = string(b)
				if len(argsStr) > 120 {
					argsStr = argsStr[:120] + "..."
				}
			}
			calls = append(calls, fmt.Sprintf("%s(%s)", tc.Name, argsStr))
		}
	}
	return calls
}

func buildSynthesisPrompt(
	userPrompt string,
	inspected map[string]struct{},
	toolCalls []string,
	current []FinderSuggestion,
) string {
	var inspectedList strings.Builder
	if len(inspected) > 0 {
		for f := range inspected {
			inspectedList.WriteString(f + "\n")
		}
	} else {
		inspectedList.WriteString("No files recorded as inspected.\n")
	}

	var investigationList strings.Builder
	if len(toolCalls) > 0 {
		start := 0
		if len(toolCalls) > 20 {
			start = len(toolCalls) - 20
		}
		for _, tc := range toolCalls[start:] {
			investigationList.WriteString(tc + "\n")
		}
	} else {
		investigationList.WriteString("No tool calls captured.\n")
	}

	var currentSummary strings.Builder
	if len(current) > 0 {
		for _, s := range current {
			summary := s.SuggestionContent
			if len(summary) > 120 {
				summary = summary[:120]
			}
			currentSummary.WriteString(fmt.Sprintf("- %s: %s\n", s.RelevantFile, summary))
		}
	} else {
		currentSummary.WriteString("No findings reported yet.\n")
	}

	return fmt.Sprintf(`%s

<AlreadyInspectedFiles>
%s</AlreadyInspectedFiles>

<RecentInvestigation>
%s</RecentInvestigation>

<CurrentFindings>
%s</CurrentFindings>

Your task:
- Re-think the review based on the context above.
- Do not add variants or restatements of existing findings.
- Do not add speculative risks.
- If there are concrete missed bugs, submit them via submitResult.
- If there is no clearly missed bug, submit an empty suggestions array.`,
		userPrompt, inspectedList.String(), investigationList.String(), currentSummary.String(),
	)
}

func mergeSuggestions(base, extra []FinderSuggestion) []FinderSuggestion {
	keyOf := func(s FinderSuggestion) string {
		return fmt.Sprintf("%s::%d::%d::%s", s.RelevantFile, s.RelevantLinesStart, s.RelevantLinesEnd, s.SuggestionContent)
	}
	seen := make(map[string]struct{}, len(base))
	for _, s := range base {
		seen[keyOf(s)] = struct{}{}
	}

	out := append([]FinderSuggestion{}, base...)
	for _, s := range extra {
		k := keyOf(s)
		if _, exists := seen[k]; !exists {
			seen[k] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// RunFinderWithVerification executes the finder agent, performs recall passes,
// collapses near-duplicates, verifies candidates, and applies the evidence gate.
func RunFinderWithVerification(
	ctx context.Context,
	runner contracts.AgentRunner,
	params FinderAgentParams,
	input contracts.AgentRunInput,
	toolCtx contracts.ToolContext,
	verifier contracts.Verifier[FinderSuggestion],
) ([]FinderSuggestion, *contracts.RunState, error) {
	spec := BuildFinderAgentSpec(params)

	state, err := runner.Run(ctx, spec, input, toolCtx)
	if err != nil && state == nil {
		return nil, nil, err
	}

	baseReasoning, baseFindings, _ := ExtractFindingsWithRecovery(ctx, state, params.RecoverProse)

	// Run recall passes: synthesis rescue + heavy resampling
	_, recallFindings, _ := RunRecallPasses(
		ctx, runner, params, input, toolCtx,
		baseReasoning, baseFindings, state,
	)

	// Collapse near duplicates using Jaccard word-overlap
	dedupThreshold := params.DedupSimilarityFloor
	if dedupThreshold <= 0 {
		dedupThreshold = DefaultDedupSimilarity
		if val := os.Getenv("SCANDRIX_DEDUP_SIMILARITY_THRESHOLD"); val != "" {
			if parsed, err := strconv.ParseFloat(val, 64); err == nil && parsed > 0 && parsed <= 1.0 {
				dedupThreshold = parsed
			}
		}
	}
	findings := CollapseNearDuplicates(recallFindings, dedupThreshold)

	// If no verifier supplied, return raw deduplicated findings
	if verifier == nil {
		return findings, state, nil
	}

	// 1. Initial verification pass across all candidate findings
	verificationResults, verErr := orchestration.RunVerificationPass(ctx, orchestration.VerificationPassParams[FinderSuggestion]{
		Candidates:  findings,
		Verifier:    verifier,
		Concurrency: 4,
	}, toolCtx)
	if verErr != nil {
		return findings, state, nil
	}

	kept := verificationResults.Kept

	// 2. EVIDENCE GATE: a finding kept WITHOUT the finder having investigated its file
	// is not trusted blindly — it gets a thorough FULL re-verify
	investigated := StrongFilesFromRun(state)
	var unevidenced []FinderSuggestion
	for _, f := range kept {
		if !FileWasInvestigated(investigated, f.RelevantFile) {
			unevidenced = append(unevidenced, f)
		}
	}

	if len(unevidenced) > 0 {
		fullVerifierSpec := BuildVerifierAgentSpec(BuildVerifierSpecParams{
			ModelID:   "verified-evidence-gate",
			AgentName: "evidence-gate-verifier",
			Tools:     params.Tools,
			MaxSteps:  10,
			ForceFull: true,
		})
		gateVerifier := NewSuggestionVerifier(runner, BuildVerifierSpecParams{
			Tools:     params.Tools,
			MaxSteps:  10,
			ForceFull: true,
		})
		_ = fullVerifierSpec

		gateResults, gateErr := orchestration.RunVerificationPass(ctx, orchestration.VerificationPassParams[FinderSuggestion]{
			Candidates:  unevidenced,
			Verifier:    gateVerifier,
			Concurrency: 4,
		}, toolCtx)

		if gateErr == nil {
			droppedSet := make(map[string]struct{})
			for _, d := range gateResults.Dropped {
				droppedSet[d.Candidate.RelevantFile+"::"+d.Candidate.SuggestionContent] = struct{}{}
			}

			var finalKept []FinderSuggestion
			for _, f := range kept {
				k := f.RelevantFile + "::" + f.SuggestionContent
				if _, dropped := droppedSet[k]; !dropped {
					finalKept = append(finalKept, f)
				}
			}
			kept = finalKept
		}
	}

	return kept, state, nil
}

// ExtractFindingsFromState inspects state artifacts for submitResult submissions (backwards compatible).
func ExtractFindingsFromState(state *contracts.RunState) []FinderSuggestion {
	_, suggestions := ExtractFindings(state)
	return suggestions
}

// CollapseNearDuplicates collapses duplicate findings on the same file and overlapping lines.
func CollapseNearDuplicates(items []FinderSuggestion, threshold float64) []FinderSuggestion {
	if len(items) <= 1 {
		return items
	}

	collapsed := make([]FinderSuggestion, 0, len(items))

	for _, item := range items {
		isDup := false
		for idx, kept := range collapsed {
			if strings.EqualFold(item.RelevantFile, kept.RelevantFile) {
				sim := jaccardSimilarity(item.SuggestionContent, kept.SuggestionContent)
				if sim >= threshold {
					isDup = true
					// Keep higher confidence or higher severity
					if item.Confidence > kept.Confidence {
						collapsed[idx] = item
					}
					break
				}
			}
		}
		if !isDup {
			collapsed = append(collapsed, item)
		}
	}

	return collapsed
}

func jaccardSimilarity(s1, s2 string) float64 {
	w1 := tokenizeWords(s1)
	w2 := tokenizeWords(s2)

	if len(w1) == 0 && len(w2) == 0 {
		return 1.0
	}
	if len(w1) == 0 || len(w2) == 0 {
		return 0.0
	}

	set1 := make(map[string]bool, len(w1))
	for _, w := range w1 {
		set1[w] = true
	}

	intersection := 0
	set2 := make(map[string]bool, len(w2))
	for _, w := range w2 {
		set2[w] = true
		if set1[w] {
			intersection++
		}
	}

	union := len(set1)
	for w := range set2 {
		if !set1[w] {
			union++
		}
	}

	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

func tokenizeWords(s string) []string {
	fields := strings.Fields(strings.ToLower(s))
	words := make([]string, 0, len(fields))
	for _, f := range fields {
		clean := strings.Trim(f, ".,;:!?\"'()[]{}/*-+`")
		if len(clean) > 2 {
			words = append(words, clean)
		}
	}
	return words
}
