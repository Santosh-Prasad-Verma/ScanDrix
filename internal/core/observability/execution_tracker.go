package observability

import (
	"fmt"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/core/idgen"
)

// ExecutionStepType mirrors ExecutionStep['type'] from ScanDrix.
type ExecutionStepType string

const (
	StepTypeStart      ExecutionStepType = "start"
	StepTypeLLMCall    ExecutionStepType = "llm_call"
	StepTypeToolCall   ExecutionStepType = "tool_call"
	StepTypeReasoning  ExecutionStepType = "reasoning"
	StepTypeValidation ExecutionStepType = "validation"
	StepTypeError      ExecutionStepType = "error"
	StepTypeComplete   ExecutionStepType = "complete"
)

// ExecutionStep mirrors ScanDrix ExecutionStep.
type ExecutionStep struct {
	StepID    string            `json:"stepId"`
	Type      ExecutionStepType `json:"type"`
	Timestamp time.Time         `json:"timestamp"`
	Duration  int64             `json:"duration,omitempty"` // ms
	Component string            `json:"component"`
	Data      map[string]any    `json:"data"`
}

// ExecutionCycleMetadata mirrors ScanDrix ExecutionCycle metadata.
type ExecutionCycleMetadata struct {
	TenantID  string `json:"tenantId,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	ThreadID  string `json:"threadId,omitempty"`
	UserID    string `json:"userId,omitempty"`
}

// ExecutionCycle mirrors ScanDrix ExecutionCycle.
type ExecutionCycle struct {
	ExecutionID   string                 `json:"executionId"`
	AgentName     string                 `json:"agentName"`
	CorrelationID string                 `json:"correlationId"`
	StartTime     time.Time              `json:"startTime"`
	EndTime       *time.Time             `json:"endTime,omitempty"`
	Duration      int64                  `json:"duration,omitempty"` // ms
	Steps         []ExecutionStep        `json:"steps"`
	Status        string                 `json:"status"` // "running" | "completed" | "failed"
	Metadata      ExecutionCycleMetadata `json:"metadata"`
	Input         any                    `json:"input,omitempty"`
	Output        any                    `json:"output,omitempty"`
	Error         string                 `json:"error,omitempty"`
}

// ExecutionTracker mirrors ScanDrix ExecutionTracker: centralized agent execution lifecycle monitoring.
type ExecutionTracker struct {
	mu        sync.RWMutex
	cycles    map[string]*ExecutionCycle
	maxCycles int
	idGen     idgen.IdGenerator
}

// NewExecutionTracker instantiates an execution tracker.
func NewExecutionTracker(maxCycles int) *ExecutionTracker {
	if maxCycles <= 0 {
		maxCycles = 1000
	}
	return &ExecutionTracker{
		cycles:    make(map[string]*ExecutionCycle),
		maxCycles: maxCycles,
		idGen:     idgen.IdGenerator{},
	}
}

// StartExecution starts tracking a new execution cycle.
func (t *ExecutionTracker) StartExecution(
	agentName string,
	correlationID string,
	meta ExecutionCycleMetadata,
	input any,
) string {
	executionID := t.idGen.ExecutionID()

	t.mu.Lock()
	if len(t.cycles) >= t.maxCycles {
		t.cleanupOldCyclesLocked()
	}

	cycle := &ExecutionCycle{
		ExecutionID:   executionID,
		AgentName:     agentName,
		CorrelationID: correlationID,
		StartTime:     time.Now().UTC(),
		Status:        "running",
		Metadata:      meta,
		Input:         input,
		Steps:         make([]ExecutionStep, 0, 8),
	}

	t.cycles[executionID] = cycle
	t.mu.Unlock()

	// Initial start step
	t.AddStep(executionID, StepTypeStart, "execution-tracker", map[string]any{
		"agentName":     agentName,
		"correlationId": correlationID,
	}, 0)

	return executionID
}

// AddStep appends an execution step to an active cycle.
func (t *ExecutionTracker) AddStep(
	executionID string,
	stepType ExecutionStepType,
	component string,
	data map[string]any,
	durationMs int64,
) {
	t.mu.Lock()
	defer t.mu.Unlock()

	cycle, exists := t.cycles[executionID]
	if !exists {
		return
	}

	step := ExecutionStep{
		StepID:    fmt.Sprintf("step_%d_%s", len(cycle.Steps)+1, t.idGen.GenerateSpanID()),
		Type:      stepType,
		Timestamp: time.Now().UTC(),
		Duration:  durationMs,
		Component: component,
		Data:      data,
	}

	cycle.Steps = append(cycle.Steps, step)
}

// CompleteExecution marks cycle as completed.
func (t *ExecutionTracker) CompleteExecution(executionID string, output any) {
	now := time.Now().UTC()

	t.mu.Lock()
	cycle, exists := t.cycles[executionID]
	if !exists {
		t.mu.Unlock()
		return
	}

	cycle.Status = "completed"
	cycle.EndTime = &now
	cycle.Duration = now.Sub(cycle.StartTime).Milliseconds()
	cycle.Output = output
	t.mu.Unlock()

	t.AddStep(executionID, StepTypeComplete, "execution-tracker", map[string]any{
		"durationMs": cycle.Duration,
	}, 0)
}

// FailExecution marks cycle as failed with an error.
func (t *ExecutionTracker) FailExecution(executionID string, err error) {
	now := time.Now().UTC()
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	t.mu.Lock()
	cycle, exists := t.cycles[executionID]
	if !exists {
		t.mu.Unlock()
		return
	}

	cycle.Status = "failed"
	cycle.EndTime = &now
	cycle.Duration = now.Sub(cycle.StartTime).Milliseconds()
	cycle.Error = errMsg
	t.mu.Unlock()

	t.AddStep(executionID, StepTypeError, "execution-tracker", map[string]any{
		"error":      errMsg,
		"durationMs": cycle.Duration,
	}, 0)
}

// GetExecution retrieves an execution cycle by ID.
func (t *ExecutionTracker) GetExecution(executionID string) (*ExecutionCycle, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	cycle, exists := t.cycles[executionID]
	if !exists {
		return nil, false
	}
	return cycle, true
}

func (t *ExecutionTracker) cleanupOldCyclesLocked() {
	var oldestID string
	var oldestTime time.Time

	for id, c := range t.cycles {
		if oldestID == "" || c.StartTime.Before(oldestTime) {
			oldestID = id
			oldestTime = c.StartTime
		}
	}

	if oldestID != "" {
		delete(t.cycles, oldestID)
	}
}
