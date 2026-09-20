// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Harness Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package contracts

import (
	"context"
)

// StepView is a read-only snapshot handed to policies at each decision point.
type StepView struct {
	RunID       string
	AgentID     string
	StepNumber  int
	MaxSteps    int
	Steps       []RunStep
	Messages    []AgentMessage
	ActiveTools []string
}

// InjectNote represents a user-turn guidance note injected mid-conversation.
// Mid-conversation notes MUST have role "user" because providers like Gemini
// reject system messages outside the initial index.
type InjectNote struct {
	Role    AgentRole `json:"role"`
	Content string    `json:"content"`
}

// StepDirectives specifies instructions a policy can return to steer the next step.
// Directives are merged in the order policies are declared.
type StepDirectives struct {
	// Messages replaces the current message history (e.g. for context compression).
	Messages []AgentMessage
	// ActiveTools restricts the tool names allowed for this step.
	ActiveTools []string
	// ModelID requests a model swap for this step.
	ModelID string
	// InjectNote appends an escalating guidance note before the model call.
	InjectNote *InjectNote
	// Emit appends observability events to the run trace.
	Emit []TraceEvent
}

// AgentPolicy defines a composable interceptor over the agent loop lifecycle.
type AgentPolicy interface {
	// Name returns the unique policy identifier for tracing.
	Name() string

	// OnRunStart runs once before the loop begins.
	OnRunStart(ctx context.Context, view StepView) error

	// PrepareStep runs before each model turn to steer tools, messages, or notes.
	PrepareStep(ctx context.Context, view StepView) (StepDirectives, error)

	// ShouldStop evaluates whether the loop should terminate early (OR semantics).
	ShouldStop(ctx context.Context, view StepView) (bool, error)

	// OnStepFinish runs after each turn completes (e.g. tracking progress).
	OnStepFinish(ctx context.Context, view StepView) error

	// OnRunFinish runs once after the loop finishes or encounters an error.
	OnRunFinish(ctx context.Context, view StepView) error
}

// BasePolicy provides default no-op implementations for AgentPolicy hooks.
type BasePolicy struct {
	PolicyName string
}

func (b *BasePolicy) Name() string {
	return b.PolicyName
}

func (b *BasePolicy) OnRunStart(ctx context.Context, view StepView) error {
	return nil
}

func (b *BasePolicy) PrepareStep(ctx context.Context, view StepView) (StepDirectives, error) {
	return StepDirectives{}, nil
}

func (b *BasePolicy) ShouldStop(ctx context.Context, view StepView) (bool, error) {
	return false, nil
}

func (b *BasePolicy) OnStepFinish(ctx context.Context, view StepView) error {
	return nil
}

func (b *BasePolicy) OnRunFinish(ctx context.Context, view StepView) error {
	return nil
}
