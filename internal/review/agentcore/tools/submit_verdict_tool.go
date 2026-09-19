package tools

import (
	"encoding/json"
	"fmt"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

const VerifyDoneTool = "submitVerdict"

// VerdictPayload represents the output emitted by submitVerdict.
type VerdictPayload struct {
	Keep       bool   `json:"keep"`
	Rationale  string `json:"rationale"`
	Confidence string `json:"confidence,omitempty"` // "high" | "medium" | "low"
}

// NewSubmitVerdictTool creates the done tool for verifier agents.
func NewSubmitVerdictTool() contracts.AgentTool {
	return &BaseTool{
		ToolName: VerifyDoneTool,
		ToolDesc: "Submit your verdict for the candidate finding (keep=true unless you can REFUTE it with concrete evidence).",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"keep": {
					Type:        "boolean",
					Description: "Set to false ONLY if you can prove the finding is an invalid false positive. Defaults to true.",
				},
				"rationale": {
					Type:        "string",
					Description: "Evidence-based rationale justifying why the finding was kept or dropped",
				},
				"confidence": {
					Type:        "string",
					Enum:        []string{"high", "medium", "low"},
					Description: "Confidence in this verdict",
				},
			},
			Required: []string{"keep", "rationale"},
		},
		IsStrict: true,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			rawJSON, err := json.Marshal(input)
			if err != nil {
				return contracts.ToolResult{Output: "Error serializing verdict", IsError: true}, nil
			}

			var payload VerdictPayload
			if err := json.Unmarshal(rawJSON, &payload); err != nil {
				return contracts.ToolResult{Output: fmt.Sprintf("Error parsing verdict: %v", err), IsError: true}, nil
			}

			action := "kept"
			if !payload.Keep {
				action = "dropped"
			}

			return contracts.ToolResult{
				Output: fmt.Sprintf("Verdict recorded: candidate %s (confidence: %s).", action, payload.Confidence),
				Meta: map[string]any{
					"keep":       payload.Keep,
					"rationale":  payload.Rationale,
					"confidence": payload.Confidence,
				},
			}, nil
		},
	}
}
