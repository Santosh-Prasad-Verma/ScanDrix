// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/agentharness/contracts"
	"github.com/scandrix/backend/internal/llm/providers/kernel"
)

// ModelTurnRequest bundles all parameters for an individual model turn.
type ModelTurnRequest struct {
	SystemPrompt    string
	Messages        []contracts.AgentMessage
	Tools           []kernel.ToolDefinition
	ModelID         string
	Temperature     *float64
	MaxTokens       *int
	ProviderOptions map[string]any
}

// ModelTurnResult represents the raw output of one turn from the model.
type ModelTurnResult struct {
	Text         string
	ToolCalls    []contracts.ToolCallRecord
	Usage        *contracts.TokenUsage
	FinishReason string
}

// ModelTurnInvoker abstracts the low-level provider call for one reasoning step.
type ModelTurnInvoker func(ctx context.Context, req ModelTurnRequest) (*ModelTurnResult, error)

// AgentRunnerOptions configures runtime behavior for the runner.
type AgentRunnerOptions struct {
	DefaultModelID string
}

// GoAgentRunner is the production implementation of contracts.AgentRunner.
// It maps the composable policy seams (prepareStep, shouldStop, onStepFinish, onRunFinish)
// onto an observable, multi-step agent loop.
type GoAgentRunner struct {
	invoker ModelTurnInvoker
	opts    AgentRunnerOptions
}

// NewGoAgentRunner constructs a GoAgentRunner with the given model invoker.
func NewGoAgentRunner(invoker ModelTurnInvoker, opts ...AgentRunnerOptions) *GoAgentRunner {
	var opt AgentRunnerOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	return &GoAgentRunner{
		invoker: invoker,
		opts:    opt,
	}
}

// Run executes an agent specification until completion, early stop, or maxSteps.
func (r *GoAgentRunner) Run(
	ctx context.Context,
	spec contracts.AgentSpec,
	input contracts.AgentRunInput,
	toolCtx contracts.ToolContext,
) (*contracts.RunState, error) {
	startTime := time.Now()
	steps := make([]contracts.RunStep, 0)
	trace := make([]contracts.TraceEvent, 0)

	emit := func(source, kind string, detail map[string]any) {
		trace = append(trace, contracts.TraceEvent{
			At:     time.Now(),
			Source: source,
			Kind:   kind,
			Detail: detail,
		})
	}

	allTools := spec.Tools.List()
	allToolNames := make([]string, len(allTools))
	for i, t := range allTools {
		allToolNames[i] = t.Name()
	}

	// Seed messages
	messages := make([]contracts.AgentMessage, 0, len(input.SeedMessages)+1)
	messages = append(messages, input.SeedMessages...)
	if input.Prompt != "" {
		messages = append(messages, contracts.AgentMessage{
			Role:    contracts.RoleUser,
			Content: input.Prompt,
		})
	}
	messages = sanitizeNoSystem(messages)

	buildView := func(stepNum int, msgs []contracts.AgentMessage, active []string) contracts.StepView {
		return contracts.StepView{
			RunID:       toolCtx.RunID,
			AgentID:     spec.ID,
			StepNumber:  stepNum,
			MaxSteps:    spec.MaxSteps,
			Steps:       steps,
			Messages:    msgs,
			ActiveTools: active,
		}
	}

	// Lifecycle: OnRunStart
	initialView := buildView(0, messages, allToolNames)
	for _, p := range spec.Policies {
		if err := p.OnRunStart(ctx, initialView); err != nil {
			emit(p.Name(), "policy.error", map[string]any{"hook": "OnRunStart", "error": err.Error()})
		}
	}

	var stopReason string

	for step := 1; step <= spec.MaxSteps; step++ {
		// Check context cancellation / deadline check
		select {
		case <-ctx.Done():
			emit("runner", "abort", map[string]any{"reason": ctx.Err().Error(), "step": step})
			errView := buildView(step, messages, allToolNames)
			for _, p := range spec.Policies {
				_ = p.OnRunFinish(ctx, errView)
			}
			return &contracts.RunState{
				RunID:      toolCtx.RunID,
				AgentID:    spec.ID,
				Status:     contracts.StatusError,
				Steps:      steps,
				Artifacts:  materializeArtifacts(steps, spec.ResultToolName, "error"),
				StopReason: "context_canceled",
				Usage:      aggregateUsage(steps),
				Trace:      trace,
			}, ctx.Err()
		default:
		}

		view := buildView(step, messages, allToolNames)

		// 1. PrepareStep: merge directives
		directives := r.mergeDirectives(ctx, spec.Policies, view, emit)
		if len(directives.Messages) > 0 {
			messages = directives.Messages
		}

		activeTools := allToolNames
		if len(directives.ActiveTools) > 0 {
			activeTools = directives.ActiveTools
		}

		// Filter active tools for model definition
		var activeToolDefs []kernel.ToolDefinition
		for _, t := range allTools {
			if contains(activeTools, t.Name()) {
				activeToolDefs = append(activeToolDefs, ToKernelToolDefinition(t))
			}
		}

		// Assemble messages with trailing injectNote if provided
		stepMessages := make([]contracts.AgentMessage, len(messages))
		copy(stepMessages, messages)
		if directives.InjectNote != nil {
			stepMessages = append(stepMessages, contracts.AgentMessage{
				Role:    contracts.RoleUser,
				Content: directives.InjectNote.Content,
			})
		}
		stepMessages = sanitizeNoSystem(stepMessages)

		// 3. Execute Model Turn
		req := ModelTurnRequest{
			SystemPrompt:    spec.SystemPrompt,
			Messages:        stepMessages,
			Tools:           activeToolDefs,
			ModelID:         directives.ModelID,
			Temperature:     spec.Temperature,
			MaxTokens:       spec.MaxOutputTokens,
			ProviderOptions: spec.ProviderOptions,
		}

		turnRes, err := r.invoker(ctx, req)
		if err != nil {
			emit("runner", "error", map[string]any{
				"message": err.Error(),
				"step":    step,
			})
			errView := buildView(step, messages, activeTools)
			for _, p := range spec.Policies {
				_ = p.OnRunFinish(ctx, errView)
			}
			return &contracts.RunState{
				RunID:      toolCtx.RunID,
				AgentID:    spec.ID,
				Status:     contracts.StatusError,
				Steps:      steps,
				Artifacts:  materializeArtifacts(steps, spec.ResultToolName, "error"),
				StopReason: "error",
				Usage:      aggregateUsage(steps),
				Trace:      trace,
			}, err
		}

		// 4. Execute tool calls requested by assistant
		executedToolCalls := make([]contracts.ToolCallRecord, 0, len(turnRes.ToolCalls))
		for _, tc := range turnRes.ToolCalls {
			toolRec := tc
			// Check if tool is allowed under current activeTools
			if !contains(activeTools, tc.Name) {
				toolRec.Output = fmt.Sprintf("Tool %s is currently inactive or restricted", tc.Name)
				toolRec.IsError = true
				executedToolCalls = append(executedToolCalls, toolRec)
				continue
			}

			targetTool, found := spec.Tools.Get(tc.Name)
			if !found {
				toolRec.Output = fmt.Sprintf("Tool %s not found in registry", tc.Name)
				toolRec.IsError = true
				executedToolCalls = append(executedToolCalls, toolRec)
				continue
			}

			start := time.Now()
			parsedInput := normalizeToolInput(tc.Input)
			res, execErr := targetTool.Execute(toolCtx, parsedInput)
			toolRec.DurationMs = time.Since(start).Milliseconds()

			if execErr != nil {
				toolRec.Output = fmt.Sprintf("ERROR: %v", execErr)
				toolRec.IsError = true
			} else {
				toolRec.Output = res.Output
				toolRec.IsError = res.IsError
			}
			executedToolCalls = append(executedToolCalls, toolRec)
		}

		// 5. Record Step
		currentStep := contracts.RunStep{
			Index: len(steps),
			Message: contracts.AgentMessage{
				Role:      contracts.RoleAssistant,
				Content:   turnRes.Text,
				ToolCalls: executedToolCalls,
			},
			Usage: turnRes.Usage,
		}
		steps = append(steps, currentStep)

		if turnRes.Usage != nil {
			emit("runner", "telemetry.span", map[string]any{
				"type":          "llm_turn",
				"step":          step,
				"input_tokens":  turnRes.Usage.InputTokens,
				"output_tokens": turnRes.Usage.OutputTokens,
				"total_tokens":  turnRes.Usage.Total(),
				"span_name":     spec.SpanName,
				"phase":         spec.Phase,
			})
		}

		// Append assistant turn and tool results to running history
		messages = append(messages, currentStep.Message)
		for _, executed := range executedToolCalls {
			messages = append(messages, contracts.AgentMessage{
				Role:       contracts.RoleTool,
				Content:    executed.Output,
				ToolCallID: executed.ID,
				Name:       executed.Name,
			})
		}

		// 6. Policy onStepFinish
		finishView := buildView(step, messages, activeTools)
		for _, p := range spec.Policies {
			if err := p.OnStepFinish(ctx, finishView); err != nil {
				emit(p.Name(), "policy.error", map[string]any{"hook": "OnStepFinish", "error": err.Error()})
			}
		}

		// 7. Check ShouldStop across all policies (OR semantics)
		stopped := false
		policyVeto := false
		for _, p := range spec.Policies {
			should, err := p.ShouldStop(ctx, finishView)
			if err != nil {
				emit(p.Name(), "policy.error", map[string]any{"hook": "ShouldStop", "error": err.Error()})
				continue
			}
			if should {
				stopReason = p.Name()
				emit(p.Name(), "stop", map[string]any{"step": step})
				stopped = true
				break
			} else {
				policyVeto = true
			}
		}
		if stopped {
			break
		}

		// If result tool was called and no policy vetoed it, complete
		resultToolCalled := false
		if spec.ResultToolName != "" {
			for _, tc := range executedToolCalls {
				if tc.Name == spec.ResultToolName {
					resultToolCalled = true
					break
				}
			}
		}
		if resultToolCalled && (!policyVeto || stopReason != "") {
			break
		}

		// Stop if assistant produced text and requested no further tools
		if len(turnRes.ToolCalls) == 0 {
			break
		}
	}

	// Finalize: OnRunFinish
	finalView := buildView(len(steps), messages, allToolNames)
	for _, p := range spec.Policies {
		if err := p.OnRunFinish(ctx, finalView); err != nil {
			emit(p.Name(), "policy.error", map[string]any{"hook": "OnRunFinish", "error": err.Error()})
		}
	}

	status := contracts.StatusCompleted
	if stopReason != "" {
		status = contracts.StatusStopped
	} else if len(steps) >= spec.MaxSteps {
		status = contracts.StatusBudgetExhausted
	}

	stage := stopReason
	if stage == "" {
		stage = "result"
	}

	totalUsage := aggregateUsage(steps)
	emit("runner", "telemetry.billing_span", map[string]any{
		"run_id":        toolCtx.RunID,
		"agent_id":      spec.ID,
		"agent_name":    spec.AgentName,
		"total_tokens":  totalUsage.Total(),
		"input_tokens":  totalUsage.InputTokens,
		"output_tokens": totalUsage.OutputTokens,
		"status":        status,
		"steps_count":   len(steps),
		"duration_ms":   time.Since(startTime).Milliseconds(),
	})

	return &contracts.RunState{
		RunID:      toolCtx.RunID,
		AgentID:    spec.ID,
		Status:     status,
		Steps:      steps,
		Artifacts:  materializeArtifacts(steps, spec.ResultToolName, stage),
		StopReason: stopReason,
		Usage:      totalUsage,
		Trace:      trace,
	}, nil
}

func (r *GoAgentRunner) mergeDirectives(
	ctx context.Context,
	policies []contracts.AgentPolicy,
	view contracts.StepView,
	emit func(source, kind string, detail map[string]any),
) contracts.StepDirectives {
	var merged contracts.StepDirectives
	var notes []string
	var activeToolsSource string
	var modelIDSource string

	for _, p := range policies {
		d, err := p.PrepareStep(ctx, view)
		if err != nil {
			emit(p.Name(), "policy.error", map[string]any{"hook": "PrepareStep", "error": err.Error()})
			continue
		}

		if len(d.Messages) > 0 {
			merged.Messages = d.Messages
		}

		if len(d.ActiveTools) > 0 {
			if activeToolsSource != "" && activeToolsSource != p.Name() {
				emit(p.Name(), "policy.conflict", map[string]any{
					"directive": "activeTools",
					"overrides": activeToolsSource,
				})
			}
			merged.ActiveTools = d.ActiveTools
			activeToolsSource = p.Name()
		}

		if d.ModelID != "" {
			if modelIDSource != "" && modelIDSource != p.Name() && merged.ModelID != d.ModelID {
				emit(p.Name(), "policy.conflict", map[string]any{
					"directive": "modelId",
					"from":      merged.ModelID,
					"to":        d.ModelID,
					"overrides": modelIDSource,
				})
			}
			merged.ModelID = d.ModelID
			modelIDSource = p.Name()
		}

		if d.InjectNote != nil && d.InjectNote.Content != "" {
			notes = append(notes, d.InjectNote.Content)
		}

		for _, e := range d.Emit {
			emit(p.Name(), e.Kind, e.Detail)
		}
	}

	if len(notes) > 0 {
		merged.InjectNote = &contracts.InjectNote{
			Role:    contracts.RoleUser,
			Content: strings.Join(notes, "\n\n"),
		}
	}

	return merged
}

func sanitizeNoSystem(messages []contracts.AgentMessage) []contracts.AgentMessage {
	sanitized := make([]contracts.AgentMessage, len(messages))
	for i, m := range messages {
		if i > 0 && m.Role == contracts.RoleSystem {
			sanitized[i] = contracts.AgentMessage{
				Role:      contracts.RoleUser,
				Content:   m.Content,
				ToolCalls: m.ToolCalls,
			}
		} else {
			sanitized[i] = m
		}
	}
	return sanitized
}

func materializeArtifacts(steps []contracts.RunStep, resultToolName, stage string) []contracts.Artifact {
	if resultToolName == "" {
		return nil
	}

	var artifacts []contracts.Artifact
	for _, s := range steps {
		for _, tc := range s.Message.ToolCalls {
			if tc.Name == resultToolName {
				artifacts = append(artifacts, contracts.Artifact{
					Type:     resultToolName,
					Payload:  normalizeToolInput(tc.Input),
					Location: fmt.Sprintf("step:%d", s.Index),
					Stage:    stage,
				})
			}
		}
	}
	return artifacts
}

func normalizeToolInput(input any) any {
	if str, ok := input.(string); ok {
		trimmed := strings.TrimSpace(str)
		// 1. Strip markdown code fences if model wrapped JSON in ```json ... ```
		if strings.HasPrefix(trimmed, "```") {
			lines := strings.Split(trimmed, "\n")
			if len(lines) >= 2 {
				// strip first line (```json) and last line (```)
				if strings.HasPrefix(lines[len(lines)-1], "```") {
					trimmed = strings.Join(lines[1:len(lines)-1], "\n")
				} else {
					trimmed = strings.Join(lines[1:], "\n")
				}
				trimmed = strings.TrimSpace(trimmed)
			}
		}

		var parsed any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			return parsed
		}

		// 2. Self-healing / repair: if model produced prose with embedded JSON, isolate and extract {...} or [...]
		if startIdx := strings.Index(trimmed, "{"); startIdx >= 0 {
			if endIdx := strings.LastIndex(trimmed, "}"); endIdx > startIdx {
				candidate := trimmed[startIdx : endIdx+1]
				if err := json.Unmarshal([]byte(candidate), &parsed); err == nil {
					return parsed
				}
			}
		} else if startIdx := strings.Index(trimmed, "["); startIdx >= 0 {
			if endIdx := strings.LastIndex(trimmed, "]"); endIdx > startIdx {
				candidate := trimmed[startIdx : endIdx+1]
				if err := json.Unmarshal([]byte(candidate), &parsed); err == nil {
					return parsed
				}
			}
		}
	}
	return input
}

func aggregateUsage(steps []contracts.RunStep) contracts.TokenUsage {
	var total contracts.TokenUsage
	for _, s := range steps {
		if s.Usage != nil {
			total = total.Add(*s.Usage)
		}
	}
	return total
}

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
