package tools

import (
	"encoding/json"
	"fmt"

	"github.com/scandrix/backend/internal/agentharness/contracts"
)

const FinderDoneTool = "submitResult"

// ReviewSuggestion represents an issue found by the code review agent.
type ReviewSuggestion struct {
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

// SubmitResultPayload holds the structured payload emitted by submitResult.
type SubmitResultPayload struct {
	Reasoning   string             `json:"reasoning,omitempty"`
	Suggestions []ReviewSuggestion `json:"suggestions"`
}

// NewSubmitResultTool creates the done tool for review finder agents.
func NewSubmitResultTool() contracts.AgentTool {
	return &BaseTool{
		ToolName: FinderDoneTool,
		ToolDesc: "Submit your final findings and end the review. Call this once you have thoroughly investigated the changed code.",
		Schema: contracts.JSONSchema{
			Type: "object",
			Properties: map[string]contracts.JSONSchema{
				"reasoning": {
					Type:        "string",
					Description: "Comprehensive step-by-step audit rationale and analysis synthesis",
				},
				"suggestions": {
					Type: "array",
					Items: &contracts.JSONSchema{
						Type: "object",
						Properties: map[string]contracts.JSONSchema{
							"relevantFile": {
								Type:        "string",
								Description: "Path to the file containing the issue",
							},
							"language": {
								Type:        "string",
								Description: "Programming language of the file",
							},
							"label": {
								Type:        "string",
								Enum:        []string{"bug", "security", "performance"},
								Description: "Primary category of the issue",
							},
							"suggestionContent": {
								Type:        "string",
								Description: "Detailed markdown explanation following WHAT/WHY/HOW format",
							},
							"existingCode": {
								Type:        "string",
								Description: "Precise snippet of the problematic existing code",
							},
							"improvedCode": {
								Type:        "string",
								Description: "Drop-in replacement fix resolving the problem",
							},
							"oneSentenceSummary": {
								Type:        "string",
								Description: "Short, impactful summary for notification title",
							},
							"relevantLinesStart": {
								Type:        "integer",
								Description: "Starting line number in the current version of the file",
							},
							"relevantLinesEnd": {
								Type:        "integer",
								Description: "Ending line number in the current version of the file",
							},
							"severity": {
								Type:        "string",
								Enum:        []string{"critical", "high", "medium", "low"},
								Description: "Impact severity rating",
							},
							"confidence": {
								Type:        "number",
								Description: "Confidence score between 0.0 and 1.0",
							},
							"ruleUuid": {
								Type:        "string",
								Description: "UUID of the custom rule violated, if applicable",
							},
						},
						Required: []string{"relevantFile", "suggestionContent", "existingCode", "improvedCode"},
					},
				},
			},
			Required: []string{"suggestions"},
		},
		IsStrict: true,
		ExecHandler: func(ctx contracts.ToolContext, input any) (contracts.ToolResult, error) {
			rawJSON, err := json.Marshal(input)
			if err != nil {
				return contracts.ToolResult{Output: "Error serializing findings", IsError: true}, nil
			}

			var payload SubmitResultPayload
			if err := json.Unmarshal(rawJSON, &payload); err != nil {
				return contracts.ToolResult{Output: fmt.Sprintf("Error parsing findings payload: %v", err), IsError: true}, nil
			}

			return contracts.ToolResult{
				Output: fmt.Sprintf("Recorded %d verified findings. Review loop finalized.", len(payload.Suggestions)),
				Meta: map[string]any{
					"findings_count": len(payload.Suggestions),
					"has_reasoning":  payload.Reasoning != "",
				},
			}, nil
		},
	}
}
