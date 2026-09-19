package runtime

import (
	"context"
	"strings"
)

// DeterministicFallbackReason indicates why a fallback was chosen.
type DeterministicFallbackReason string

const (
	FallbackToolUnavailable      DeterministicFallbackReason = "tool_unavailable"
	FallbackPreconditionFailed   DeterministicFallbackReason = "precondition_failed"
	FallbackMissingResult        DeterministicFallbackReason = "missing_result"
	FallbackExecutionError       DeterministicFallbackReason = "execution_error"
)

// ExecuteDeterministicToolParams parameters for deterministic execution.
type ExecuteDeterministicToolParams[T any] struct {
	ToolName   string
	Args       map[string]any
	CallTool   func(ctx context.Context, toolName string, args map[string]any) (*ToolExecutionResponse, error)
	Extract    func(payload any) T
	Fallback   T
	Validate   func() (DeterministicFallbackReason, bool)
	OnError    string // "throw" or "fallback"
	OnFallback func(reason DeterministicFallbackReason, err error)
}

// ExecuteDeterministicTool executes an MCP tool with robust precondition checks and fallback handling.
func ExecuteDeterministicTool[T any](
	ctx context.Context,
	params ExecuteDeterministicToolParams[T],
) (T, error) {
	if strings.TrimSpace(params.ToolName) == "" {
		if params.OnFallback != nil {
			params.OnFallback(FallbackToolUnavailable, nil)
		}
		return params.Fallback, nil
	}

	if params.Validate != nil {
		if reason, failed := params.Validate(); failed {
			if params.OnFallback != nil {
				params.OnFallback(reason, nil)
			}
			return params.Fallback, nil
		}
	}

	if params.CallTool == nil {
		if params.OnFallback != nil {
			params.OnFallback(FallbackToolUnavailable, nil)
		}
		return params.Fallback, nil
	}

	toolResult, err := params.CallTool(ctx, params.ToolName, params.Args)
	if err != nil {
		if params.OnError == "fallback" || params.OnError == "" {
			if params.OnFallback != nil {
				params.OnFallback(FallbackExecutionError, err)
			}
			return params.Fallback, nil
		}
		return params.Fallback, err
	}

	if toolResult == nil || toolResult.Result == nil {
		if params.OnFallback != nil {
			params.OnFallback(FallbackMissingResult, nil)
		}
		return params.Fallback, nil
	}

	if params.Extract != nil {
		return params.Extract(toolResult.Result), nil
	}
	return params.Fallback, nil
}
