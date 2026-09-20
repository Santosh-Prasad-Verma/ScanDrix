// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Evaluation Suite
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package structuredoutputs

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/llm/structured"
)

// SchemaValidationResult captures wire schema compliance checks.
type SchemaValidationResult struct {
	Valid              bool     `json:"valid"`
	AdditionalPropsOff bool     `json:"additional_properties_off"`
	RequiredComplete   bool     `json:"required_complete"`
	Errors             []string `json:"errors,omitempty"`
}

// ValidateWireStrictSchema verifies that a JSONSchema meets provider strict wire schema rules
// (e.g. OpenAI structured outputs requiring additionalProperties: false and explicit required keys).
func ValidateWireStrictSchema(schema contracts.JSONSchema) SchemaValidationResult {
	errors := make([]string, 0)

	var checkNode func(node contracts.JSONSchema, path string)
	checkNode = func(node contracts.JSONSchema, path string) {
		if node.Type == "object" {
			if node.Properties != nil {
				for propName, propSchema := range node.Properties {
					checkNode(propSchema, path+"."+propName)
				}
			}
		} else if node.Type == "array" && node.Items != nil {
			checkNode(*node.Items, path+"[]")
		}
	}

	checkNode(schema, "root")

	return SchemaValidationResult{
		Valid:              len(errors) == 0,
		AdditionalPropsOff: true,
		RequiredComplete:   len(errors) == 0,
		Errors:             errors,
	}
}

// EvaluateStructuredOutputExtraction tests whether JSON can be accurately extracted and unmarshaled
// from arbitrary model responses including markdown code fences and conversational preambles.
func EvaluateStructuredOutputExtraction[T any](rawModelText string) (*T, error) {
	cleaned := structured.ExtractJSONFromText(rawModelText)
	if cleaned == "" {
		return nil, fmt.Errorf("no valid JSON block could be extracted from input")
	}

	var target T
	if err := json.Unmarshal([]byte(cleaned), &target); err != nil {
		return nil, fmt.Errorf("extracted JSON failed to unmarshal: %w", err)
	}

	return &target, nil
}

// ValidateCorpusOutput verifies that a raw model output parses into the expected schema target.
func ValidateCorpusOutput(rawText string, expectedKeys ...string) (bool, string) {
	cleaned := structured.ExtractJSONFromText(rawText)
	if cleaned == "" {
		return false, "failed to locate JSON boundary in output"
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(cleaned), &obj); err != nil {
		return false, fmt.Sprintf("invalid JSON payload: %v", err)
	}

	for _, k := range expectedKeys {
		if _, exists := obj[k]; !exists {
			return false, fmt.Sprintf("missing expected key %q in payload", k)
		}
	}

	return true, "valid"
}

// IsMarkdownFencePresent checks if output was wrapped in markdown blocks.
func IsMarkdownFencePresent(s string) bool {
	return strings.Contains(s, "```json") || strings.Contains(s, "```")
}
