// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package verify

import (
	"encoding/json"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/tools"
)

// VerifyDoneTool is the name of the result tool a verifier agent invokes to emit its verdict.
const VerifyDoneTool = "submitVerdict"

// VerdictSchema defines the JSON Schema for submitVerdict.
var VerdictSchema = contracts.JSONSchema{
	Type: "object",
	Properties: map[string]contracts.JSONSchema{
		"keep": {
			Type:        "boolean",
			Description: "Set to false ONLY if you can definitively refute this finding with code evidence.",
		},
		"rationale": {
			Type:        "string",
			Description: "Clear reasoning for the verdict based on code evidence.",
		},
		"confidence": {
			Type: "string",
			Enum: []string{"high", "medium", "low"},
		},
	},
	Required: []string{"keep", "rationale"},
}

// submitVerdictToolImpl implements the verdict capture tool.
type submitVerdictToolImpl struct{}

func (t *submitVerdictToolImpl) Name() string {
	return VerifyDoneTool
}

func (t *submitVerdictToolImpl) Description() string {
	return "Submit your verdict for the candidate (keep=true unless you can REFUTE it)."
}

func (t *submitVerdictToolImpl) InputSchema() contracts.JSONSchema {
	return VerdictSchema
}

func (t *submitVerdictToolImpl) Strict() bool {
	return true
}

func (t *submitVerdictToolImpl) Execute(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
	return contracts.ToolResult{
		Output: "verdict recorded",
	}, nil
}

// SubmitVerdictTool is the singleton tool used by verifier agents.
var SubmitVerdictTool contracts.AgentTool = &submitVerdictToolImpl{}

// BuildVerifierAgentSpecParams carries parameters for constructing a verifier AgentSpec.
type BuildVerifierAgentSpecParams struct {
	ID              string
	SystemPrompt    string
	Tools           contracts.ToolRegistry
	MaxSteps        int
	Policies        []contracts.AgentPolicy
	ProviderOptions map[string]any
}

// BuildVerifierAgentSpec constructs an AgentSpec tailored for objective verification.
// It automatically appends the submitVerdict tool and configures ResultToolName for artifact capture.
func BuildVerifierAgentSpec(params BuildVerifierAgentSpecParams) contracts.AgentSpec {
	var toolList []contracts.AgentTool
	if params.Tools != nil {
		toolList = append(toolList, params.Tools.List()...)
	}
	toolList = append(toolList, SubmitVerdictTool)
	registry := tools.NewInMemoryToolRegistry(toolList...)

	id := params.ID
	if id == "" {
		id = "verifier"
	}

	maxSteps := params.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 6
	}

	return contracts.AgentSpec{
		ID:              id,
		SystemPrompt:    params.SystemPrompt,
		Tools:           registry,
		Policies:        params.Policies,
		MaxSteps:        maxSteps,
		ResultToolName:  VerifyDoneTool,
		ProviderOptions: params.ProviderOptions,
	}
}

// CollectVerifierToolCalls flattens a verifier run's tool calls into generic VerifierToolCallRecords,
// excluding the submitVerdict result tool.
func CollectVerifierToolCalls(state *contracts.RunState) []contracts.VerifierToolCallRecord {
	if state == nil {
		return nil
	}

	var out []contracts.VerifierToolCallRecord
	for _, step := range state.Steps {
		for _, tc := range step.Message.ToolCalls {
			if tc.Name == VerifyDoneTool {
				continue
			}
			argsMap, _ := tc.Input.(map[string]any)
			out = append(out, contracts.VerifierToolCallRecord{
				Name:   tc.Name,
				Args:   argsMap,
				Result: tc.Output,
			})
		}
	}
	return out
}

// ExtractVerdict extracts the structured Verdict from a verifier run's materialized artifacts.
// Fail-Open Semantics: defaults to keep=true so that any parsing failure or incomplete run
// never silently drops a candidate. Only an explicit keep:false drops a candidate.
func ExtractVerdict(state *contracts.RunState) contracts.Verdict {
	toolCalls := CollectVerifierToolCalls(state)

	if state != nil {
		for i := len(state.Artifacts) - 1; i >= 0; i-- {
			artifact := state.Artifacts[i]
			if artifact.Type != VerifyDoneTool {
				continue
			}

			// Parse payload
			var payloadMap map[string]any
			if m, ok := artifact.Payload.(map[string]any); ok {
				payloadMap = m
			} else if str, ok := artifact.Payload.(string); ok {
				_ = json.Unmarshal([]byte(str), &payloadMap)
			} else {
				// Re-marshal generic object
				if b, err := json.Marshal(artifact.Payload); err == nil {
					_ = json.Unmarshal(b, &payloadMap)
				}
			}

			if payloadMap != nil {
				if keepVal, ok := payloadMap["keep"].(bool); ok {
					rationale, _ := payloadMap["rationale"].(string)
					confidence, _ := payloadMap["confidence"].(string)

					return contracts.Verdict{
						Keep:       keepVal,
						Rationale:  rationale,
						Confidence: confidence,
						ToolCalls:  toolCalls,
					}
				}
			}
		}
	}

	// Fail-open default
	return contracts.Verdict{
		Keep:      true,
		Rationale: "no parseable verdict — kept by default",
		ToolCalls: toolCalls,
	}
}
