// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/agentharness/infrastructure/runner"
	"github.com/scandrix/backend/internal/llm/agentloop"
	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/scandrix/backend/internal/llm/contextwin"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
	"github.com/scandrix/backend/internal/llm/structured"
)

// Engine is the unified production AI orchestration engine for ScanDrix.
type Engine struct {
	registry        *kernel.ProviderRegistry
	mu              sync.RWMutex
	limiters        map[string]*byok.BYOKConcurrencyLimiter
	breakerRegistry *ProviderBreakerRegistry
}

// NewEngine constructs a production Engine instance.
func NewEngine() *Engine {
	return &Engine{
		registry:        kernel.DefaultRegistry,
		limiters:        make(map[string]*byok.BYOKConcurrencyLimiter),
		breakerRegistry: NewProviderBreakerRegistry(5, 60*time.Second),
	}
}

// CallOption configures execution options.
type CallOption func(*kernel.ExecutionRequest)

// WithTemperature sets the temperature for sampling.
func WithTemperature(temp float64) CallOption {
	return func(r *kernel.ExecutionRequest) {
		r.Temperature = &temp
	}
}

// WithMaxTokens sets the maximum generation tokens.
func WithMaxTokens(max int) CallOption {
	return func(r *kernel.ExecutionRequest) {
		r.MaxTokens = max
	}
}

// WithReasoningEffort sets canonical reasoning effort.
func WithReasoningEffort(effort kernel.ReasoningEffort) CallOption {
	return func(r *kernel.ExecutionRequest) {
		r.ReasoningEffort = effort
	}
}

// WithSystemCacheHint toggles prompt caching for the system prompt.
func WithSystemCacheHint(enable bool) CallOption {
	return func(r *kernel.ExecutionRequest) {
		r.SystemCacheHint = enable
	}
}

// RunText executes a text generation request through the designated provider slot.
func (e *Engine) RunText(ctx context.Context, slot byok.NormalizedModel, msgs []kernel.ChatMessage, opts ...CallOption) (*kernel.ExecutionResult, error) {
	return e.executeSlot(ctx, slot, msgs, nil, nil, opts...)
}

// RunStructured executes a structured JSON generation request, repairing invalid output deterministically.
func (e *Engine) RunStructured(ctx context.Context, slot byok.NormalizedModel, msgs []kernel.ChatMessage, schema map[string]any, target any, opts ...CallOption) (*kernel.ExecutionResult, error) {
	// Preflight context checks
	var sysPrompt, userPrompt string
	for _, m := range msgs {
		if m.Role == "system" {
			sysPrompt += m.Content + "\n"
		} else {
			userPrompt += m.Content + "\n"
		}
	}

	maxTokens := slot.MaxInputTokens
	if maxTokens <= 0 {
		maxTokens = 128000
	}
	if err := contextwin.AssertPromptFitsInContext(sysPrompt, userPrompt, maxTokens, slot.Model); err != nil {
		return nil, fmt.Errorf("context window preflight failed: %w", err)
	}

	strictWireSchema := structured.ToStrictWireSchema(schema)
	schemaBytes, _ := json.Marshal(strictWireSchema)

	// Format system prompt with schema guidelines as defense-in-depth
	msgsCopy := make([]kernel.ChatMessage, len(msgs))
	copy(msgsCopy, msgs)
	if len(msgsCopy) > 0 && msgsCopy[0].Role == "system" {
		msgsCopy[0].Content = structured.FormatPromptWithSchema(msgsCopy[0].Content, string(schemaBytes))
	}

	res, err := e.executeSlot(ctx, slot, msgsCopy, strictWireSchema, nil, opts...)
	if err != nil {
		return nil, err
	}

	// Deterministic JSON extraction and repair
	cleanJSON := structured.ExtractJSONFromText(res.Text)
	if err := json.Unmarshal([]byte(cleanJSON), target); err != nil {
		// Attempt repair pass
		repaired := structured.RepairJSONText(cleanJSON)
		if retryErr := json.Unmarshal([]byte(repaired), target); retryErr != nil {
			return nil, fmt.Errorf("failed to parse structured response after repair: %w (raw: %s)", err, res.Text)
		}
		res.Text = repaired
	} else {
		res.Text = cleanJSON
	}

	return res, nil
}

// LoopFailoverControl tracks whether agent tool actions have made failover unsafe.
type LoopFailoverControl struct {
	unsafe bool
}

func (c *LoopFailoverControl) MarkUnsafeToRetry() {
	c.unsafe = true
}

func (c *LoopFailoverControl) CanAttemptFallback() bool {
	return !c.unsafe
}

// RunLoop executes an agentic multi-turn loop with tools.
func (e *Engine) RunLoop(
	ctx context.Context,
	slot byok.NormalizedModel,
	initialMessages []map[string]any,
	tools []kernel.ToolDefinition,
	toolExecutor func(ctx context.Context, call agentloop.ToolCall) (agentloop.ToolResult, error),
	maxSteps int,
	opts ...CallOption,
) (*agentloop.AgentLoopResult, error) {
	mod, ok := e.registry.Get(string(slot.Provider))
	if !ok {
		return nil, fmt.Errorf("unknown provider %s", slot.Provider)
	}

	control := &LoopFailoverControl{}

	runner := func(c context.Context, stepNum int, history []map[string]any) (*agentloop.StepEvent, error) {
		var chatMsgs []kernel.ChatMessage
		for _, m := range history {
			role, _ := m["role"].(string)
			content, _ := m["content"].(string)
			chatMsgs = append(chatMsgs, kernel.ChatMessage{
				Role:    role,
				Content: content,
			})
		}

		req := kernel.ExecutionRequest{
			Messages:        chatMsgs,
			Tools:           tools,
			ToolChoice:      "auto",
			SystemCacheHint: true,
		}
		for _, opt := range opts {
			opt(&req)
		}

		res, err := mod.Execute(c, slot, req)
		if err != nil {
			return nil, err
		}

		event := &agentloop.StepEvent{
			StepNumber:  stepNum,
			TextContent: res.Text,
			TokensUsed:  res.Usage.TotalTokens,
		}
		for _, tc := range res.ToolCalls {
			event.ToolCalls = append(event.ToolCalls, agentloop.ToolCall{
				ID:        tc.ID,
				ToolName:  tc.Name,
				Arguments: tc.Arguments,
			})
		}
		return event, nil
	}

	seams := agentloop.AgentLoopSeams{
		MaxSteps:     maxSteps,
		ToolExecutor: toolExecutor,
	}

	return agentloop.ExecuteLoop(ctx, runner, seams, control, initialMessages)
}

// BuildAgentRunner constructs an enterprise AgentRunner bound to a model slot.
func (e *Engine) BuildAgentRunner(slot byok.NormalizedModel) (contracts.AgentRunner, error) {
	mod, ok := e.registry.Get(string(slot.Provider))
	if !ok {
		return nil, fmt.Errorf("unknown provider %s", slot.Provider)
	}

	invoker := func(c context.Context, req runner.ModelTurnRequest) (*runner.ModelTurnResult, error) {
		var chatMsgs []kernel.ChatMessage
		if req.SystemPrompt != "" {
			chatMsgs = append(chatMsgs, kernel.ChatMessage{
				Role:    "system",
				Content: req.SystemPrompt,
			})
		}

		for _, m := range req.Messages {
			contentStr := ""
			if s, ok := m.Content.(string); ok {
				contentStr = s
			} else if b, err := json.Marshal(m.Content); err == nil {
				contentStr = string(b)
			}

			var toolCalls []kernel.ToolCall
			for _, tc := range m.ToolCalls {
				argStr := ""
				if s, ok := tc.Input.(string); ok {
					argStr = s
				} else if b, err := json.Marshal(tc.Input); err == nil {
					argStr = string(b)
				}
				toolCalls = append(toolCalls, kernel.ToolCall{
					ID:        tc.ID,
					Name:      tc.Name,
					Arguments: argStr,
				})
			}

			chatMsgs = append(chatMsgs, kernel.ChatMessage{
				Role:       string(m.Role),
				Content:    contentStr,
				ToolCallID: m.ToolCallID,
				Name:       m.Name,
				ToolCalls:  toolCalls,
			})
		}

		kReq := kernel.ExecutionRequest{
			Messages:        chatMsgs,
			Tools:           req.Tools,
			ToolChoice:      "auto",
			SystemCacheHint: true,
			Temperature:     req.Temperature,
		}
		if req.MaxTokens != nil {
			kReq.MaxTokens = *req.MaxTokens
		}

		res, err := mod.Execute(c, slot, kReq)
		if err != nil {
			return nil, err
		}

		var toolCalls []contracts.ToolCallRecord
		for _, tc := range res.ToolCalls {
			toolCalls = append(toolCalls, contracts.ToolCallRecord{
				ID:    tc.ID,
				Name:  tc.Name,
				Input: tc.Arguments,
			})
		}

		var usage *contracts.TokenUsage
		if res.Usage.TotalTokens > 0 {
			usage = &contracts.TokenUsage{
				InputTokens:     res.Usage.InputTokens,
				OutputTokens:    res.Usage.OutputTokens,
				ReasoningTokens: res.Usage.ReasoningTokens,
			}
		}

		return &runner.ModelTurnResult{
			Text:      res.Text,
			ToolCalls: toolCalls,
			Usage:     usage,
		}, nil
	}

	return runner.NewGoAgentRunner(invoker), nil
}

// RunAgentSpec executes an AgentSpec declaration using the enterprise AgentRunner.
func (e *Engine) RunAgentSpec(
	ctx context.Context,
	slot byok.NormalizedModel,
	spec contracts.AgentSpec,
	input contracts.AgentRunInput,
	toolCtx contracts.ToolContext,
) (*contracts.RunState, error) {
	agentRunner, err := e.BuildAgentRunner(slot)
	if err != nil {
		return nil, err
	}
	return agentRunner.Run(ctx, spec, input, toolCtx)
}

// RunWithFailover executes an operation with automatic fallback slot cascading.
func (e *Engine) RunWithFailover(
	ctx context.Context,
	slot byok.NormalizedModel,
	fallbackSlot *byok.NormalizedModel,
	fn func(s byok.NormalizedModel) (*kernel.ExecutionResult, error),
) (*kernel.ExecutionResult, error) {
	res, err := fn(slot)
	if err == nil {
		return res, nil
	}

	if fallbackSlot == nil {
		return nil, AttachAttemptedSlot(err, slot.Model, string(slot.Provider))
	}

	fallbackCopy := *fallbackSlot
	fallbackCopy.UsedFallback = true
	res, fallbackErr := fn(fallbackCopy)
	if fallbackErr != nil {
		stampedFallbackErr := AttachAttemptedSlot(fallbackErr, fallbackCopy.Model, string(fallbackCopy.Provider))
		return nil, fmt.Errorf("primary model failed (%v); fallback model also failed: %w", err, stampedFallbackErr)
	}

	return res, nil
}

func (e *Engine) getLimiter(slot byok.NormalizedModel) *byok.BYOKConcurrencyLimiter {
	key := slot.CredentialID
	if key == "" {
		key = string(slot.Provider) + ":" + slot.Model
	}

	e.mu.RLock()
	lim, ok := e.limiters[key]
	e.mu.RUnlock()
	if ok {
		return lim
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if lim, ok = e.limiters[key]; ok {
		return lim
	}

	lim = byok.NewBYOKConcurrencyLimiter(
		slot.MaxConcurrentRequests,
		slot.RPM,
		slot.TPM,
		string(slot.Provider),
		slot.Model,
	)
	e.limiters[key] = lim
	return lim
}

func (e *Engine) executeSlot(
	ctx context.Context,
	slot byok.NormalizedModel,
	msgs []kernel.ChatMessage,
	schema map[string]any,
	tools []kernel.ToolDefinition,
	opts ...CallOption,
) (*kernel.ExecutionResult, error) {
	mod, ok := e.registry.Get(string(slot.Provider))
	if !ok {
		return nil, fmt.Errorf("provider module not found for %s", slot.Provider)
	}

	req := kernel.ExecutionRequest{
		Messages:        msgs,
		ResponseSchema:  schema,
		Tools:           tools,
		SystemCacheHint: true,
	}

	if slot.Temperature != nil {
		req.Temperature = slot.Temperature
	}
	if slot.MaxOutputTokens > 0 {
		req.MaxTokens = slot.MaxOutputTokens
	}
	if slot.ReasoningEffort != "" {
		req.ReasoningEffort = kernel.ReasoningEffort(slot.ReasoningEffort)
	}

	for _, opt := range opts {
		opt(&req)
	}

	// Rate limiter gate
	var estimatedTokens int
	for _, m := range msgs {
		estimatedTokens += len(m.Content) / 4
	}

	limiter := e.getLimiter(slot)
	if err := limiter.Acquire(ctx, estimatedTokens); err != nil {
		return nil, fmt.Errorf("rate limiter acquire error: %w", err)
	}

	var actualTokens int
	defer func() {
		limiter.Release(estimatedTokens, actualTokens)
	}()

	// Circuit breaker gate — fail-fast during provider outages
	breaker := e.breakerRegistry.GetOrCreate(string(slot.Provider))
	if !breaker.Allow() {
		return nil, fmt.Errorf("%w for provider '%s'", ErrCircuitOpen, string(slot.Provider))
	}

	res, err := mod.Execute(ctx, slot, req)
	if err != nil {
		breaker.RecordFailure()

		// Only arm cooldown on rate-limit errors, not auth/model/timeout errors
		classified := ClassifyLLMError(err, 0)
		if classified.Category == CategoryRateLimit {
			cooldown := 30 * time.Second
			if slot.CooldownMs > 0 {
				cooldown = time.Duration(slot.CooldownMs) * time.Millisecond
			}
			limiter.ArmCooldown(cooldown)
		}
		return nil, err
	}

	breaker.RecordSuccess()
	actualTokens = res.Usage.TotalTokens
	return res, nil
}
