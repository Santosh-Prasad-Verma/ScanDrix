// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package agentloop

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/scandrix/backend/internal/llm/structured"
)

// ModelInvoker abstracts invoking a model for a single-shot prompt repair.
type ModelInvoker func(ctx context.Context, prompt string) (string, error)

// RepairInvalidToolInput re-prompts the SAME model to fix invalid tool arguments against the tool's schema.
// Fully fail-soft: any failure to re-ask or parse returns an empty string, allowing the step to fail naturally.
func RepairInvalidToolInput(
	ctx context.Context,
	invoker ModelInvoker,
	toolName string,
	rawInput string,
	validationErr error,
) string {
	if invoker == nil || toolName == "" {
		return ""
	}

	errMsg := ""
	if validationErr != nil {
		errMsg = validationErr.Error()
	}

	prompt := fmt.Sprintf(
		"The tool \"%s\" was called with arguments that failed schema validation.\n"+
			"Invalid arguments: %s\n"+
			"Validation error: %s\n"+
			"Return corrected arguments that satisfy the schema. Keep the original intent; only fix what is malformed. "+
			"Return ONLY the valid JSON object without markdown fences or explanations.",
		toolName, rawInput, errMsg,
	)

	response, err := invoker(ctx, prompt)
	if err != nil {
		return ""
	}

	cleaned := structured.ExtractJSONFromText(response)
	if cleaned == "" {
		return ""
	}

	// Verify that the candidate parses as valid JSON
	var testVal any
	if err := json.Unmarshal([]byte(cleaned), &testVal); err != nil {
		return ""
	}

	return cleaned
}
