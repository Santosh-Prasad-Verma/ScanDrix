// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package blueprint

import (
	"context"
	"fmt"
	"maps"
)

// ─── Step Types ──────────────────────────────────────────────────────────────

// StepType identifies the execution modality of a blueprint step.
type StepType string

const (
	StepTypeDeterministic StepType = "deterministic"
	StepTypeGate          StepType = "gate"
	StepTypeLLM           StepType = "llm"
	StepTypeFormat        StepType = "format"
	StepTypeParallel      StepType = "parallel"
)

// StepExecutionStatus indicates the outcome of a single step.
type StepExecutionStatus string

const (
	StatusSuccess StepExecutionStatus = "success"
	StatusFailed  StepExecutionStatus = "failed"
	StatusSkipped StepExecutionStatus = "skipped"
)

// ─── Context ─────────────────────────────────────────────────────────────────

// BlueprintContext represents the base context bag passed between blueprint steps.
type BlueprintContext interface {
	GetOrganizationAndTeamData() any
	GetUserLanguage() string
	GetThread() any
	GetPrepareContext() map[string]any
	GetResult() any
	SetResult(res any)
	GetError() string
	SetError(err string)
	Get(key string) (any, bool)
	Set(key string, val any)
	CloneContext() BlueprintContext
}

// DefaultContext is a standard, thread-safe implementation of BlueprintContext.
type DefaultContext struct {
	OrganizationAndTeamData any            `json:"organization_and_team_data,omitempty"`
	UserLanguage            string         `json:"user_language,omitempty"`
	Thread                  any            `json:"thread,omitempty"`
	PrepareContext          map[string]any `json:"prepare_context,omitempty"`
	Result                  any            `json:"result,omitempty"`
	Error                   string         `json:"error,omitempty"`
	Attributes              map[string]any `json:"attributes,omitempty"`
}

// NewDefaultContext constructs an initialized DefaultContext.
func NewDefaultContext(orgTeamData any, userLanguage string) *DefaultContext {
	if userLanguage == "" {
		userLanguage = "en-US"
	}
	return &DefaultContext{
		OrganizationAndTeamData: orgTeamData,
		UserLanguage:            userLanguage,
		PrepareContext:          make(map[string]any),
		Attributes:              make(map[string]any),
	}
}

func (c *DefaultContext) GetOrganizationAndTeamData() any    { return c.OrganizationAndTeamData }
func (c *DefaultContext) GetUserLanguage() string           { return c.UserLanguage }
func (c *DefaultContext) GetThread() any                    { return c.Thread }
func (c *DefaultContext) GetPrepareContext() map[string]any { return c.PrepareContext }
func (c *DefaultContext) GetResult() any                    { return c.Result }
func (c *DefaultContext) SetResult(res any)                 { c.Result = res }
func (c *DefaultContext) GetError() string                  { return c.Error }
func (c *DefaultContext) SetError(err string)               { c.Error = err }

func (c *DefaultContext) Get(key string) (any, bool) {
	if c.Attributes == nil {
		return nil, false
	}
	val, ok := c.Attributes[key]
	return val, ok
}

func (c *DefaultContext) Set(key string, val any) {
	if c.Attributes == nil {
		c.Attributes = make(map[string]any)
	}
	c.Attributes[key] = val
}

func (c *DefaultContext) CloneContext() BlueprintContext {
	clone := &DefaultContext{
		OrganizationAndTeamData: c.OrganizationAndTeamData,
		UserLanguage:            c.UserLanguage,
		Thread:                  c.Thread,
		Result:                  c.Result,
		Error:                   c.Error,
	}
	if c.PrepareContext != nil {
		clone.PrepareContext = make(map[string]any, len(c.PrepareContext))
		maps.Copy(clone.PrepareContext, c.PrepareContext)
	}
	if c.Attributes != nil {
		clone.Attributes = make(map[string]any, len(c.Attributes))
		maps.Copy(clone.Attributes, c.Attributes)
	}
	return clone
}

// ─── Step Contract & Errors ──────────────────────────────────────────────────

// Validator defines a type-safe contract validation check on the context.
type Validator[T any] interface {
	Validate(ctx T) error
}

// ValidatorFunc allows plain functions to act as contracts.
type ValidatorFunc[T any] func(ctx T) error

func (f ValidatorFunc[T]) Validate(ctx T) error {
	return f(ctx)
}

// StepContract encapsulates pre-execution (input) and post-execution (output) validation.
type StepContract[T any] struct {
	Input  Validator[T]
	Output Validator[T]
}

// BlueprintStepContractViolationError represents a failed contract pre/post condition.
type BlueprintStepContractViolationError struct {
	StepName string
	Stage    string // "input" or "output"
	Details  string
}

func (e *BlueprintStepContractViolationError) Error() string {
	return fmt.Sprintf("[blueprint] Step '%s' failed %s contract validation: %s", e.StepName, e.Stage, e.Details)
}

// ─── Step Interface & Implementations ────────────────────────────────────────

// BlueprintStep defines the execution contract for any step in the pipeline.
type BlueprintStep[T any] interface {
	Name() string
	Type() StepType
	GetContract() *StepContract[T]
	Execute(ctx context.Context, c T, opts *RunnerOptions[T]) (T, StepExecutionStatus, error)
}

// DeterministicStep runs a pure function without LLM involvement.
type DeterministicStep[T any] struct {
	StepName     string
	StepContract *StepContract[T]
	Fn           func(ctx context.Context, c T) (T, error)
}

func (s DeterministicStep[T]) Name() string                  { return s.StepName }
func (s DeterministicStep[T]) Type() StepType                { return StepTypeDeterministic }
func (s DeterministicStep[T]) GetContract() *StepContract[T] { return s.StepContract }
func (s DeterministicStep[T]) Execute(ctx context.Context, c T, _ *RunnerOptions[T]) (T, StepExecutionStatus, error) {
	if s.Fn == nil {
		return c, StatusSuccess, nil
	}
	res, err := s.Fn(ctx, c)
	if err != nil {
		return c, StatusFailed, err
	}
	return res, StatusSuccess, nil
}

// GateStep evaluates a condition and short-circuits if false.
type GateStep[T any] struct {
	StepName     string
	StepContract *StepContract[T]
	Condition    func(ctx context.Context, c T) bool
	OnFail       func(ctx context.Context, c T) (T, error)
}

func (s GateStep[T]) Name() string                  { return s.StepName }
func (s GateStep[T]) Type() StepType                { return StepTypeGate }
func (s GateStep[T]) GetContract() *StepContract[T] { return s.StepContract }
func (s GateStep[T]) Execute(ctx context.Context, c T, _ *RunnerOptions[T]) (T, StepExecutionStatus, error) {
	if s.Condition == nil {
		return c, StatusSuccess, nil
	}
	passed := s.Condition(ctx, c)
	if !passed {
		if s.OnFail != nil {
			failCtx, err := s.OnFail(ctx, c)
			if err != nil {
				return c, StatusFailed, err
			}
			return failCtx, StatusSkipped, nil
		}
		return c, StatusSkipped, nil
	}
	return c, StatusSuccess, nil
}

// LLMStep delegates execution to the caller's RunLLMStep handler.
type LLMStep[T any] struct {
	StepName     string
	StepContract *StepContract[T]
	Skill        string
	AgentName    string
}

func (s LLMStep[T]) Name() string                  { return s.StepName }
func (s LLMStep[T]) Type() StepType                { return StepTypeLLM }
func (s LLMStep[T]) GetContract() *StepContract[T] { return s.StepContract }
func (s LLMStep[T]) Execute(ctx context.Context, c T, opts *RunnerOptions[T]) (T, StepExecutionStatus, error) {
	if opts == nil || opts.RunLLMStep == nil {
		return c, StatusFailed, fmt.Errorf("[blueprint] LLM step '%s' encountered but RunLLMStep handler is not configured", s.StepName)
	}
	res, err := opts.RunLLMStep(ctx, s, c)
	if err != nil {
		return c, StatusFailed, err
	}
	return res, StatusSuccess, nil
}

// FormatStep transforms or parses output context after an LLM step.
type FormatStep[T any] struct {
	StepName     string
	StepContract *StepContract[T]
	Fn           func(c T) (T, error)
}

func (s FormatStep[T]) Name() string                  { return s.StepName }
func (s FormatStep[T]) Type() StepType                { return StepTypeFormat }
func (s FormatStep[T]) GetContract() *StepContract[T] { return s.StepContract }
func (s FormatStep[T]) Execute(_ context.Context, c T, _ *RunnerOptions[T]) (T, StepExecutionStatus, error) {
	if s.Fn == nil {
		return c, StatusSuccess, nil
	}
	res, err := s.Fn(c)
	if err != nil {
		return c, StatusFailed, err
	}
	return res, StatusSuccess, nil
}

// ParallelStep delegates concurrent skill execution to opts.RunParallelStep.
type ParallelStep[T any] struct {
	StepName     string
	StepContract *StepContract[T]
	Skills       []string
}

func (s ParallelStep[T]) Name() string                  { return s.StepName }
func (s ParallelStep[T]) Type() StepType                { return StepTypeParallel }
func (s ParallelStep[T]) GetContract() *StepContract[T] { return s.StepContract }
func (s ParallelStep[T]) Execute(ctx context.Context, c T, opts *RunnerOptions[T]) (T, StepExecutionStatus, error) {
	if opts == nil || opts.RunParallelStep == nil {
		return c, StatusFailed, fmt.Errorf("[blueprint] Parallel step '%s' cannot be executed by RunBlueprint directly; RunParallelStep must be supplied", s.StepName)
	}
	res, err := opts.RunParallelStep(ctx, s, c)
	if err != nil {
		return c, StatusFailed, err
	}
	return res, StatusSuccess, nil
}

// ─── Logger & Metrics ────────────────────────────────────────────────────────

// Logger defines a lightweight logging sink for blueprint execution.
type Logger interface {
	Log(msg string, args ...any)
	Error(msg string, err error, args ...any)
}

// StepMetric records timing and outcome metrics per step.
type StepMetric struct {
	StepName     string              `json:"step_name"`
	StepType     StepType            `json:"step_type"`
	Status       StepExecutionStatus `json:"status"`
	DurationMs   int64               `json:"duration_ms"`
	ErrorMessage string              `json:"error_message,omitempty"`
}

// RunnerOptions configures the execution environment of RunBlueprint.
type RunnerOptions[T any] struct {
	Steps           []BlueprintStep[T]
	InitialContext  T
	RunLLMStep      func(ctx context.Context, step LLMStep[T], c T) (T, error)
	RunParallelStep func(ctx context.Context, step ParallelStep[T], c T) (T, error)
	OnStepMetric    func(metric StepMetric)
	Logger          Logger
}

// BlueprintResult contains the final accumulated context, completed steps, and optional skip point.
type BlueprintResult[T any] struct {
	Context        T        `json:"context"`
	CompletedSteps []string `json:"completed_steps"`
	SkippedAt      string   `json:"skipped_at,omitempty"`
}
