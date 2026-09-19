// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package llm

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/llm/agentloop"
	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
	"github.com/scandrix/backend/internal/llm/routing"
)

// LLMRequest encapsulates the input parameters for invoking an LLM call.
type LLMRequest struct {
	// Pre-resolved slot — caller already routed task -> slot
	ByokConfig *byok.NormalizedModel
	// OR route here: org's stored BYOK config + task
	Config *byok.BYOKConfig
	Task   byok.LlmTask
	Ctx    *routing.RequestContext

	// Prompts
	System   string
	User     string
	Messages []kernel.ChatMessage

	// Structured output
	Schema map[string]any
	Target any // Target pointer to deserialize JSON into

	// Agent loop mode
	Tools        []kernel.ToolDefinition
	ToolExecutor func(ctx context.Context, call agentloop.ToolCall) (agentloop.ToolResult, error)
	MaxSteps     int

	// Tuning overrides
	Temperature     *float64
	MaxOutputTokens int
	ReasoningEffort kernel.ReasoningEffort
	OrganizationID  string
}

// LLMResult represents the outcome of an LLM call.
type LLMResult struct {
	Text      string
	ToolCalls []kernel.ToolCall
	Usage     kernel.TokenUsage
	Raw       any
	SlotUsed  byok.NormalizedModel
}

// ResolveSlot determines the active slot: explicit slot wins, then task routing, then system managed default.
func ResolveSlot(req LLMRequest) *byok.NormalizedModel {
	if req.ByokConfig != nil {
		return req.ByokConfig
	}
	if req.Config != nil && req.Task != "" {
		overrideID := ""
		overrideName := ""
		if req.Ctx != nil {
			overrideID = req.Ctx.OverrideModelID
			overrideName = req.Ctx.OverrideModelName
		}
		slot, _, _ := byok.ResolveTaskSlot(req.Config, req.Task, overrideID, overrideName)
		if slot != nil {
			return slot
		}
	}
	// System managed fallback from environment variables
	res := byok.ResolveManagedSlot("", byok.ByokModelOptions{})
	if res != nil {
		return res.SlotFromResolution()
	}
	return nil
}

var defaultEngine = NewEngine()

// Run is the unified entrypoint for executing all LLM operations in ScanDrix.
func Run(ctx context.Context, req LLMRequest) (*LLMResult, error) {
	slot := ResolveSlot(req)
	if slot == nil {
		return nil, fmt.Errorf("no AI model slot could be resolved: configure BYOK credentials or set environment API keys")
	}

	attempts := []*byok.NormalizedModel{slot}
	if slot.Fallback != nil {
		attempts = append(attempts, slot.Fallback.FallbackToSlot())
	}

	var lastErr error
	for i, attemptSlot := range attempts {
		if attemptSlot == nil {
			continue
		}

		s := *attemptSlot
		if i > 0 {
			s.UsedFallback = true
		}

		// Apply request-level tuning overrides
		if req.Temperature != nil {
			s.Temperature = req.Temperature
		}
		if req.MaxOutputTokens > 0 {
			s.MaxOutputTokens = req.MaxOutputTokens
		}
		if req.ReasoningEffort != "" {
			s.ReasoningEffort = string(req.ReasoningEffort)
		}

		// 1. Agent-loop execution mode
		if req.ToolExecutor != nil && len(req.Tools) > 0 {
			initialMessages := make([]map[string]any, 0, len(req.Messages)+2)
			if req.System != "" {
				initialMessages = append(initialMessages, map[string]any{
					"role":    "system",
					"content": req.System,
				})
			}
			for _, m := range req.Messages {
				initialMessages = append(initialMessages, map[string]any{
					"role":    m.Role,
					"content": m.Content,
				})
			}
			if req.User != "" {
				initialMessages = append(initialMessages, map[string]any{
					"role":    "user",
					"content": req.User,
				})
			}

			maxSteps := req.MaxSteps
			if maxSteps <= 0 {
				maxSteps = 10
			}

			loopRes, err := defaultEngine.RunLoop(ctx, s, initialMessages, req.Tools, req.ToolExecutor, maxSteps)
			if err != nil {
				stamped := AttachAttemptedSlot(err, s.Model, string(s.Provider))
				lastErr = stamped
				classified := ClassifyLLMError(stamped, 0, string(s.Provider))
				if ShouldFailover(classified) && i < len(attempts)-1 {
					continue
				}
				return nil, stamped
			}

			return &LLMResult{
				Text:     loopRes.FinalAnswer,
				SlotUsed: s,
			}, nil
		}

		// Build conversation messages for one-shot call
		msgs := make([]kernel.ChatMessage, 0, len(req.Messages)+2)
		if req.System != "" {
			msgs = append(msgs, kernel.ChatMessage{
				Role:    "system",
				Content: req.System,
			})
		}
		msgs = append(msgs, req.Messages...)
		if req.User != "" {
			msgs = append(msgs, kernel.ChatMessage{
				Role:    "user",
				Content: req.User,
			})
		}

		// 2. Structured review call mode
		if req.Schema != nil {
			var callRes *kernel.ExecutionResult
			var callErr error

			// Attempt 1
			callRes, callErr = defaultEngine.RunStructured(ctx, s, msgs, req.Schema, req.Target)
			if callErr != nil && IsRetryableForReissue(callErr) {
				// Re-issue with jittered backoff
				backoff := JitteredBackoff(1, RetryBaseDelayMs, RetryMaxDelayMs)
				if sleepErr := SleepContext(ctx, backoff); sleepErr == nil {
					callRes, callErr = defaultEngine.RunStructured(ctx, s, msgs, req.Schema, req.Target)
				}
			}

			if callErr != nil {
				stamped := AttachAttemptedSlot(callErr, s.Model, string(s.Provider))
				lastErr = stamped
				classified := ClassifyLLMError(stamped, 0, string(s.Provider))
				if ShouldFailover(classified) && i < len(attempts)-1 {
					continue
				}
				return nil, stamped
			}

			return &LLMResult{
				Text:      callRes.Text,
				ToolCalls: callRes.ToolCalls,
				Usage:     callRes.Usage,
				Raw:       callRes.Raw,
				SlotUsed:  s,
			}, nil
		}

		// 3. Plain text review call mode
		var callRes *kernel.ExecutionResult
		var callErr error

		callRes, callErr = defaultEngine.RunText(ctx, s, msgs)
		if callErr != nil && IsRetryableForReissue(callErr) {
			backoff := JitteredBackoff(1, RetryBaseDelayMs, RetryMaxDelayMs)
			if sleepErr := SleepContext(ctx, backoff); sleepErr == nil {
				callRes, callErr = defaultEngine.RunText(ctx, s, msgs)
			}
		}

		if callErr != nil {
			stamped := AttachAttemptedSlot(callErr, s.Model, string(s.Provider))
			lastErr = stamped
			classified := ClassifyLLMError(stamped, 0, string(s.Provider))
			if ShouldFailover(classified) && i < len(attempts)-1 {
				continue
			}
			return nil, stamped
		}

		return &LLMResult{
			Text:      callRes.Text,
			ToolCalls: callRes.ToolCalls,
			Usage:     callRes.Usage,
			Raw:       callRes.Raw,
			SlotUsed:  s,
		}, nil
	}

	return nil, lastErr
}
