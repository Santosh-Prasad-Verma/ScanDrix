// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/invocation"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
	"github.com/scandrix/backend/internal/llm/structured"
	"github.com/scandrix/backend/internal/llm/systemcache"
	"github.com/scandrix/backend/internal/llm/tokens"
)

const (
	DefaultReviewCallTimeout = 10 * time.Minute
	MaxTransientRetries      = 1 // D-00c latency guard: single re-issue on transient blip
)

// BaseReviewCallParams holds shared inputs for review LLM calls.
type BaseReviewCallParams struct {
	Slot                 *byok.NormalizedModel
	System               string
	User                 string
	RunName              string
	OrganizationID       string
	Timeout              time.Duration
	DefaultModelOverride string
	Temperature          *float64
	MaxOutputTokens      int
	ProviderOptions      map[string]any
}

// StructuredReviewCallParams extends BaseReviewCallParams for schema-enforced output.
type StructuredReviewCallParams struct {
	BaseReviewCallParams
	Schema map[string]any
	Target any // Pointer to destination struct for deserialization
}

// TextReviewCallParams specifies plain text generation parameters.
type TextReviewCallParams struct {
	BaseReviewCallParams
}

// ReviewCallResult represents the outcome of a structured or text review call.
type ReviewCallResult struct {
	Text      string
	Usage     tokens.TokenUsage
	Raw       any
	Identity  invocation.ModelIdentity
	SlotUsed  byok.NormalizedModel
}

// RunStructuredReviewCall executes a structured LLM call with schema enforcement,
// automatic JSON repair, and transient-error retry.
func RunStructuredReviewCall(ctx context.Context, params StructuredReviewCallParams) (*ReviewCallResult, error) {
	if params.Target == nil {
		return nil, errors.New("target pointer must not be nil for structured review call")
	}

	timeout := params.Timeout
	if timeout <= 0 {
		timeout = DefaultReviewCallTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	inv := invocation.ResolveModelConfig(params.Slot, invocation.ResolveModelInvocationOptions{
		RunName:              params.RunName,
		DefaultModelOverride: params.DefaultModelOverride,
	})

	var slot byok.NormalizedModel
	if inv.Slot != nil {
		slot = *inv.Slot
	} else {
		res := byok.ResolveManagedSlot(params.DefaultModelOverride, byok.ByokModelOptions{})
		managed := res.SlotFromResolution()
		if managed == nil {
			return nil, errors.New("no active model slot or managed environment configuration available")
		}
		slot = *managed
	}

	providerID := string(slot.Provider)
	if !kernel.DefaultRegistry.Has(providerID) {
		return nil, fmt.Errorf("provider %q not registered", providerID)
	}
	module, _ := kernel.DefaultRegistry.Get(providerID)

	traits := module.ReasoningTraits(slot)
	plan := structured.ResolveStructuredPlan(
		providerID,
		slot.Model,
		traits.ThinksByDefault || traits.CanDisableThinking,
		traits.ThinksByDefault,
		traits.RejectsThinkingWhenToolsForced,
	)

	// Build messages
	systemPrompt := params.System
	var schemaBytes []byte
	if params.Schema != nil {
		schemaBytes, _ = json.MarshalIndent(params.Schema, "", "  ")
	}

	if plan == structured.PlanRerouteJSON && len(schemaBytes) > 0 {
		systemPrompt = structured.FormatPromptWithSchema(systemPrompt, string(schemaBytes))
	}

	messages := make([]kernel.ChatMessage, 0, 2)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, kernel.ChatMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	messages = append(messages, kernel.ChatMessage{
		Role:    "user",
		Content: params.User,
	})

	// Execution request
	execReq := kernel.ExecutionRequest{
		Messages:        messages,
		Temperature:     inv.CallOptions.Temperature,
		MaxTokens:       inv.CallOptions.MaxOutputTokens,
		SystemCacheHint: systemcache.SystemCacheControl(systemcache.SystemCacheControlInput{
			Provider: providerID,
			Model:    slot.Model,
		}) != nil,
	}
	if params.Temperature != nil {
		execReq.Temperature = params.Temperature
	}
	if params.MaxOutputTokens > 0 {
		execReq.MaxTokens = params.MaxOutputTokens
	}
	if plan == structured.PlanAsIs && params.Schema != nil {
		if structured.MayUseJSONSchema(providerID, slot.Model, slot.BaseURL) {
			execReq.ResponseSchema = params.Schema
		} else {
			// Proactively downgrade to prompt-injected JSON mode if previously marked unsupported
			systemPrompt = structured.FormatPromptWithSchema(systemPrompt, string(schemaBytes))
			if len(messages) > 0 && messages[0].Role == "system" {
				messages[0].Content = systemPrompt
			}
		}
	}

	// Single re-issue loop on transient network/5xx blips or schema unsupported errors
	var lastErr error
	for attempt := 1; attempt <= 1+MaxTransientRetries; attempt++ {
		if attempt > 1 {
			backoff := JitteredBackoff(attempt, RetryBaseDelayMs, RetryMaxDelayMs)
			slog.Info("re-issuing structured review call after transient failure", "attempt", attempt, "backoff", backoff)
			if err := SleepContext(callCtx, backoff); err != nil {
				return nil, err
			}
		}

		res, err := module.Execute(callCtx, slot, execReq)
		if err != nil {
			lastErr = err
			// Check if upstream provider rejected response_format json_schema
			if execReq.ResponseSchema != nil && structured.IsJSONSchemaUnsupportedError(err) {
				slog.Warn("provider rejected json_schema; caching and re-issuing in prompt-injected mode", "provider", providerID, "model", slot.Model)
				structured.MarkJSONSchemaUnsupported(providerID, slot.Model, slot.BaseURL)
				execReq.ResponseSchema = nil
				systemPrompt = structured.FormatPromptWithSchema(systemPrompt, string(schemaBytes))
				if len(messages) > 0 && messages[0].Role == "system" {
					messages[0].Content = systemPrompt
				}
				continue
			}
			if IsRetryableForReissue(err) {
				continue
			}
			return nil, err
		}

		// Extract & parse JSON
		rawText := res.Text
		extracted := structured.ExtractJSONFromText(rawText)
		if extracted == "" {
			extracted = rawText
		}

		if err := json.Unmarshal([]byte(extracted), params.Target); err != nil {
			lastErr = fmt.Errorf("structured schema parse failure: %w (raw output snippet: %s)", err, truncateSnippet(rawText, 160))
			// If first attempt failed JSON parsing, try one more time if within budget
			continue
		}

		usageId := invocation.AgentModelIdentity(inv.Slot)
		usageId.Model = inv.ModelName

		return &ReviewCallResult{
			Text: rawText,
			Usage: tokens.TokenUsage{
				InputTokens:           res.Usage.InputTokens,
				OutputTokens:          res.Usage.OutputTokens,
				TotalTokens:           res.Usage.TotalTokens,
				OutputReasoningTokens: res.Usage.ReasoningTokens,
				Model:                 inv.ModelName,
				RunName:               params.RunName,
			},
			Raw:      res.Raw,
			Identity: usageId,
			SlotUsed: slot,
		}, nil
	}

	return nil, fmt.Errorf("structured review call failed after retries: %w", lastErr)
}

// RunTextReviewCall executes a plain text generation call without schema constraints.
func RunTextReviewCall(ctx context.Context, params TextReviewCallParams) (*ReviewCallResult, error) {
	timeout := params.Timeout
	if timeout <= 0 {
		timeout = DefaultReviewCallTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	inv := invocation.ResolveModelConfig(params.Slot, invocation.ResolveModelInvocationOptions{
		RunName:              params.RunName,
		DefaultModelOverride: params.DefaultModelOverride,
	})

	var slot byok.NormalizedModel
	if inv.Slot != nil {
		slot = *inv.Slot
	} else {
		res := byok.ResolveManagedSlot(params.DefaultModelOverride, byok.ByokModelOptions{})
		managed := res.SlotFromResolution()
		if managed == nil {
			return nil, errors.New("no active model slot or managed environment configuration available")
		}
		slot = *managed
	}

	providerID := string(slot.Provider)
	if !kernel.DefaultRegistry.Has(providerID) {
		return nil, fmt.Errorf("provider %q not registered", providerID)
	}
	module, _ := kernel.DefaultRegistry.Get(providerID)

	messages := make([]kernel.ChatMessage, 0, 2)
	if strings.TrimSpace(params.System) != "" {
		messages = append(messages, kernel.ChatMessage{
			Role:    "system",
			Content: params.System,
		})
	}
	messages = append(messages, kernel.ChatMessage{
		Role:    "user",
		Content: params.User,
	})

	execReq := kernel.ExecutionRequest{
		Messages:    messages,
		Temperature: inv.CallOptions.Temperature,
		MaxTokens:   inv.CallOptions.MaxOutputTokens,
	}
	if params.Temperature != nil {
		execReq.Temperature = params.Temperature
	}
	if params.MaxOutputTokens > 0 {
		execReq.MaxTokens = params.MaxOutputTokens
	}

	res, err := module.Execute(callCtx, slot, execReq)
	if err != nil {
		return nil, err
	}

	usageId := invocation.AgentModelIdentity(inv.Slot)
	usageId.Model = inv.ModelName

	return &ReviewCallResult{
		Text: res.Text,
		Usage: tokens.TokenUsage{
			InputTokens:           res.Usage.InputTokens,
			OutputTokens:          res.Usage.OutputTokens,
			TotalTokens:           res.Usage.TotalTokens,
			OutputReasoningTokens: res.Usage.ReasoningTokens,
			Model:                 inv.ModelName,
			RunName:               params.RunName,
		},
		Raw:      res.Raw,
		Identity: usageId,
		SlotUsed: slot,
	}, nil
}

func truncateSnippet(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
