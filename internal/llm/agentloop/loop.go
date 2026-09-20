// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package agentloop

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// ToolCall represents an invocation of an MCP or native tool emitted by the model.
type ToolCall struct {
	ID        string `json:"id"`
	ToolName  string `json:"tool_name"`
	Arguments string `json:"arguments"`
}

// ToolResult represents the output of a tool execution fed back into the model.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

// StepEvent captures an individual reasoning and tool-dispatch step.
type StepEvent struct {
	StepNumber   int          `json:"step_number"`
	TextContent  string       `json:"text_content,omitempty"`
	ToolCalls    []ToolCall   `json:"tool_calls,omitempty"`
	ToolResults  []ToolResult `json:"tool_results,omitempty"`
	FinishReason string       `json:"finish_reason,omitempty"`
	TokensUsed   int          `json:"tokens_used,omitempty"`
}

// FailoverAttemptControl allows an active loop attempt to veto fallback restarts
// when side-effecting tools have executed.
type FailoverAttemptControl interface {
	MarkUnsafeToRetry()
}

// AgentLoopSeams encapsulates runner policies and hooks for the multi-step agent loop.
type AgentLoopSeams struct {
	MaxSteps     int
	ToolExecutor func(ctx context.Context, call ToolCall) (ToolResult, error)
	OnStepFinish func(event StepEvent)
	StopWhen     func(event StepEvent) bool
	PrepareStep  func(stepNumber int, history []map[string]any) ([]map[string]any, []string, error)
}

// AgentLoopResult aggregates the full multi-step agent execution.
type AgentLoopResult struct {
	Steps       []StepEvent `json:"steps"`
	FinalAnswer string      `json:"final_answer"`
	TotalTokens int         `json:"total_tokens"`
	StopReason  string      `json:"stop_reason,omitempty"`
}

// StepRunner represents the low-level provider call that generates one turn with tools.
type StepRunner func(ctx context.Context, stepNum int, history []map[string]any) (*StepEvent, error)

// AgentLoopParams encapsulates inputs for running the agent loop call via the LLM gateway.
type AgentLoopParams struct {
	SystemPrompt      string
	InitialMessages   []map[string]any
	Tools             []map[string]any
	Provider          string
	Model             string
	Runner            StepRunner
	Seams             AgentLoopSeams
	Control           FailoverAttemptControl
	Config            AgentLoopConfig
	Invoker           ModelInvoker
	TelemetryMetadata map[string]any
}

// RunAgentLoopCall drives the multi-step agent loop with full enterprise resilience:
// - Automatic exponential backoff retry for transient step failures.
// - Multi-breakpoint prompt caching for cache-capable providers.
// - Tool-call argument self-healing repair.
// - Failover attempt marking when side-effecting tools run.
func RunAgentLoopCall(ctx context.Context, params AgentLoopParams) (*AgentLoopResult, error) {
	cfg := params.Config
	if cfg.MaxSteps <= 0 {
		cfg = LoadAgentLoopConfig()
	}

	maxSteps := params.Seams.MaxSteps
	if maxSteps <= 0 {
		maxSteps = cfg.MaxSteps
	}

	result := &AgentLoopResult{
		Steps: make([]StepEvent, 0),
	}

	// 1. Prepare initial conversation history
	history := make([]map[string]any, len(params.InitialMessages))
	copy(history, params.InitialMessages)

	// 2. Apply prompt caching breakpoints if enabled
	if cfg.PromptCacheEnabled {
		_, cachedMessages, _ := ApplyCacheBreakpoints(
			params.SystemPrompt,
			history,
			params.Tools,
			maxSteps,
			params.Provider,
			params.Model,
		)
		history = cachedMessages
	}

	for step := 1; step <= maxSteps; step++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Optional per-step preparation hook
		if params.Seams.PrepareStep != nil {
			modifiedHistory, _, prepErr := params.Seams.PrepareStep(step, history)
			if prepErr == nil && modifiedHistory != nil {
				history = modifiedHistory
			}
		}

		// Step execution with transient retries (exponential backoff + jitter)
		var stepEvent *StepEvent
		var stepErr error

		maxAttempts := cfg.MaxRetries + 1
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			stepEvent, stepErr = params.Runner(ctx, step, history)
			if stepErr == nil && stepEvent != nil {
				break
			}

			if attempt < maxAttempts {
				backoff := cfg.RetryBaseDelay * time.Duration(1<<(attempt-1))
				jitter := time.Duration(rand.Int63n(int64(backoff / 4)))
				sleepDuration := backoff + jitter

				select {
				case <-time.After(sleepDuration):
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
		}

		if stepErr != nil {
			return nil, fmt.Errorf("agent step %d failed after %d attempts: %w", step, maxAttempts, stepErr)
		}

		stepEvent.StepNumber = step
		result.TotalTokens += stepEvent.TokensUsed

		// If tools were called, mark unsafe to retry externally and execute tools
		if len(stepEvent.ToolCalls) > 0 {
			if params.Control != nil {
				params.Control.MarkUnsafeToRetry()
			}

			if params.Seams.ToolExecutor != nil {
				for _, call := range stepEvent.ToolCalls {
					res, execErr := params.Seams.ToolExecutor(ctx, call)
					if execErr != nil {
						// Attempt tool argument repair if an invoker is supplied
						if params.Invoker != nil {
							repaired := RepairInvalidToolInput(ctx, params.Invoker, call.ToolName, call.Arguments, execErr)
							if repaired != "" && repaired != call.Arguments {
								call.Arguments = repaired
								res, execErr = params.Seams.ToolExecutor(ctx, call)
							}
						}
					}

					if execErr != nil {
						res = ToolResult{
							ToolCallID: call.ID,
							Content:    fmt.Sprintf("Tool error: %v", execErr),
							IsError:    true,
						}
					}
					stepEvent.ToolResults = append(stepEvent.ToolResults, res)
				}
			}
		}

		result.Steps = append(result.Steps, *stepEvent)
		result.FinalAnswer = stepEvent.TextContent

		// Record step turn into conversation history
		stepTurn := map[string]any{
			"role":    "assistant",
			"content": stepEvent.TextContent,
		}
		if len(stepEvent.ToolCalls) > 0 {
			stepTurn["tool_calls"] = stepEvent.ToolCalls
		}
		history = append(history, stepTurn)

		if len(stepEvent.ToolResults) > 0 {
			resultTurn := map[string]any{
				"role":         "tool",
				"tool_results": stepEvent.ToolResults,
			}
			history = append(history, resultTurn)
		}

		// Run step finish callback
		if params.Seams.OnStepFinish != nil {
			params.Seams.OnStepFinish(*stepEvent)
		}

		// Check policy stop condition
		if params.Seams.StopWhen != nil && params.Seams.StopWhen(*stepEvent) {
			result.StopReason = "stop_condition_met"
			break
		}

		// Natural stop: no further tool calls requested
		if len(stepEvent.ToolCalls) == 0 {
			result.StopReason = "natural_completion"
			break
		}
	}

	if result.StopReason == "" {
		result.StopReason = "step_limit_reached"
	}

	return result, nil
}

// ExecuteLoop maintains backward compatibility for existing callers while delegating
// to the environment-configured loop engine.
func ExecuteLoop(
	ctx context.Context,
	runner StepRunner,
	seams AgentLoopSeams,
	control FailoverAttemptControl,
	initialMessages []map[string]any,
) (*AgentLoopResult, error) {
	cfg := LoadAgentLoopConfig()
	if seams.MaxSteps <= 0 {
		seams.MaxSteps = cfg.MaxSteps
	}

	params := AgentLoopParams{
		InitialMessages: initialMessages,
		Runner:          runner,
		Seams:           seams,
		Control:         control,
		Config:          cfg,
	}

	return RunAgentLoopCall(ctx, params)
}
