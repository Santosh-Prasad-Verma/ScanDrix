// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package routing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ReviewTaskType categorizes the specific analytical workload in the review pipeline.
type ReviewTaskType string

const (
	TaskFileTriage             ReviewTaskType = "FILE_TRIAGE"
	TaskRuleEvaluation         ReviewTaskType = "RULE_EVALUATION"
	TaskDeepDeliberation       ReviewTaskType = "DEEP_DELIBERATION"
	TaskSuggestionVerification ReviewTaskType = "SUGGESTION_VERIFICATION"
	TaskDeduplication          ReviewTaskType = "DEDUPLICATION"
	TaskSummaryGeneration      ReviewTaskType = "SUMMARY_GENERATION"
)

// ModelTier categorizes the capacity and latency characteristics of a model.
type ModelTier string

const (
	TierFast      ModelTier = "FAST"
	TierStandard  ModelTier = "STANDARD"
	TierReasoning ModelTier = "REASONING"
)

// ProviderType identifies the underlying LLM provider.
type ProviderType string

const (
	ProviderAnthropic ProviderType = "ANTHROPIC"
	ProviderOpenAI    ProviderType = "OPENAI"
	ProviderGemini    ProviderType = "GEMINI"
	ProviderVertex    ProviderType = "VERTEX"
)

// ReviewModelSlot defines the metadata and runtime capabilities for an LLM model slot.
type ReviewModelSlot struct {
	ID               string       `json:"id"`
	Provider         ProviderType `json:"provider"`
	ModelName        string       `json:"model_name"`
	Tier             ModelTier    `json:"tier"`
	MaxContextTokens int          `json:"max_context_tokens"`
	MaxOutputTokens  int          `json:"max_output_tokens"`
	CostPer1kPrompt  float64      `json:"cost_per_1k_prompt"`
	CostPer1kOutput  float64      `json:"cost_per_1k_output"`
	FallbackSlotID   string       `json:"fallback_slot_id,omitempty"`
}

// CircuitState represents the health status of a model provider or slot.
type CircuitState string

const (
	CircuitClosed   CircuitState = "CLOSED"    // Healthy, normal operation
	CircuitHalfOpen CircuitState = "HALF_OPEN" // Trial recovery mode
	CircuitOpen     CircuitState = "OPEN"      // Down, redirecting to fallback
)

// SlotHealth tracks operational metrics and circuit-breaker state for a slot.
type SlotHealth struct {
	mu                  sync.RWMutex
	ConsecutiveFailures int
	TotalCalls          int64
	TotalErrors         int64
	TotalTokensConsumed int64
	CircuitState        CircuitState
	LastFailureTime     time.Time
	LastSuccessTime     time.Time
	CircuitOpenedTime   time.Time
	CoolDownPeriod      time.Duration
}

// DynamicModelRouter dynamically maps review pipeline tasks to optimal LLM slots
// with real-time failover, circuit breaker resilience, and workspace BYOK support.
type DynamicModelRouter struct {
	mu               sync.RWMutex
	slots            map[string]ReviewModelSlot
	health           map[string]*SlotHealth
	taskDefaultSlots map[ReviewTaskType]string
	workspaceConfigs map[string]map[ReviewTaskType]string // workspaceID -> Task -> SlotID
	failureThreshold int
	circuitCooldown  time.Duration
}

// RouterConfig specifies initialization options for the router.
type RouterConfig struct {
	FailureThreshold int
	CircuitCooldown  time.Duration
}

// NewDynamicModelRouter creates and populates a new model router with canonical slots.
func NewDynamicModelRouter(cfg *RouterConfig) *DynamicModelRouter {
	thresh := 3
	cooldown := 30 * time.Second
	if cfg != nil {
		if cfg.FailureThreshold > 0 {
			thresh = cfg.FailureThreshold
		}
		if cfg.CircuitCooldown > 0 {
			cooldown = cfg.CircuitCooldown
		}
	}

	r := &DynamicModelRouter{
		slots:            make(map[string]ReviewModelSlot),
		health:           make(map[string]*SlotHealth),
		taskDefaultSlots: make(map[ReviewTaskType]string),
		workspaceConfigs: make(map[string]map[ReviewTaskType]string),
		failureThreshold: thresh,
		circuitCooldown:  cooldown,
	}

	r.registerDefaultSlots()
	return r
}

func (r *DynamicModelRouter) registerDefaultSlots() {
	// Canonical ScanDrix Model Slots
	fastAnthropic := ReviewModelSlot{
		ID:               "anthropic-haiku",
		Provider:         ProviderAnthropic,
		ModelName:        "claude-3-5-haiku-20241022",
		Tier:             TierFast,
		MaxContextTokens: 200000,
		MaxOutputTokens:  8192,
		CostPer1kPrompt:  0.0008,
		CostPer1kOutput:  0.004,
		FallbackSlotID:   "gemini-flash",
	}
	fastGemini := ReviewModelSlot{
		ID:               "gemini-flash",
		Provider:         ProviderGemini,
		ModelName:        "gemini-2.0-flash",
		Tier:             TierFast,
		MaxContextTokens: 1048576,
		MaxOutputTokens:  8192,
		CostPer1kPrompt:  0.0001,
		CostPer1kOutput:  0.0004,
		FallbackSlotID:   "openai-mini",
	}
	fastOpenAI := ReviewModelSlot{
		ID:               "openai-mini",
		Provider:         ProviderOpenAI,
		ModelName:        "gpt-4o-mini",
		Tier:             TierFast,
		MaxContextTokens: 128000,
		MaxOutputTokens:  16384,
		CostPer1kPrompt:  0.00015,
		CostPer1kOutput:  0.0006,
	}

	stdAnthropic := ReviewModelSlot{
		ID:               "anthropic-sonnet",
		Provider:         ProviderAnthropic,
		ModelName:        "claude-3-5-sonnet-20241022",
		Tier:             TierStandard,
		MaxContextTokens: 200000,
		MaxOutputTokens:  8192,
		CostPer1kPrompt:  0.003,
		CostPer1kOutput:  0.015,
		FallbackSlotID:   "gemini-pro",
	}
	stdGemini := ReviewModelSlot{
		ID:               "gemini-pro",
		Provider:         ProviderGemini,
		ModelName:        "gemini-2.5-pro",
		Tier:             TierStandard,
		MaxContextTokens: 2097152,
		MaxOutputTokens:  65536,
		CostPer1kPrompt:  0.00125,
		CostPer1kOutput:  0.005,
		FallbackSlotID:   "openai-gpt4o",
	}
	stdOpenAI := ReviewModelSlot{
		ID:               "openai-gpt4o",
		Provider:         ProviderOpenAI,
		ModelName:        "gpt-4o",
		Tier:             TierStandard,
		MaxContextTokens: 128000,
		MaxOutputTokens:  16384,
		CostPer1kPrompt:  0.0025,
		CostPer1kOutput:  0.010,
	}

	heavyAnthropic := ReviewModelSlot{
		ID:               "anthropic-sonnet-reasoning",
		Provider:         ProviderAnthropic,
		ModelName:        "claude-3-7-sonnet-thinking",
		Tier:             TierReasoning,
		MaxContextTokens: 200000,
		MaxOutputTokens:  64000,
		CostPer1kPrompt:  0.003,
		CostPer1kOutput:  0.015,
		FallbackSlotID:   "gemini-pro",
	}

	allSlots := []ReviewModelSlot{
		fastAnthropic, fastGemini, fastOpenAI,
		stdAnthropic, stdGemini, stdOpenAI,
		heavyAnthropic,
	}

	for _, slot := range allSlots {
		r.slots[slot.ID] = slot
		r.health[slot.ID] = &SlotHealth{
			CircuitState:   CircuitClosed,
			CoolDownPeriod: r.circuitCooldown,
		}
	}

	// Task Default Mappings
	r.taskDefaultSlots[TaskFileTriage] = "anthropic-haiku"
	r.taskDefaultSlots[TaskRuleEvaluation] = "anthropic-sonnet"
	r.taskDefaultSlots[TaskDeepDeliberation] = "anthropic-sonnet-reasoning"
	r.taskDefaultSlots[TaskSuggestionVerification] = "anthropic-sonnet"
	r.taskDefaultSlots[TaskDeduplication] = "gemini-flash"
	r.taskDefaultSlots[TaskSummaryGeneration] = "gemini-flash"
}

// RegisterSlot adds or updates a slot in the router.
func (r *DynamicModelRouter) RegisterSlot(slot ReviewModelSlot) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.slots[slot.ID] = slot
	if _, exists := r.health[slot.ID]; !exists {
		r.health[slot.ID] = &SlotHealth{
			CircuitState:   CircuitClosed,
			CoolDownPeriod: r.circuitCooldown,
		}
	}
}

// SetWorkspaceTaskSlot configures a custom BYOK slot for a specific workspace and task.
func (r *DynamicModelRouter) SetWorkspaceTaskSlot(workspaceID string, task ReviewTaskType, slotID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.slots[slotID]; !exists {
		return fmt.Errorf("slot %s not registered in router", slotID)
	}

	if r.workspaceConfigs[workspaceID] == nil {
		r.workspaceConfigs[workspaceID] = make(map[ReviewTaskType]string)
	}
	r.workspaceConfigs[workspaceID][task] = slotID
	return nil
}

// ResolveSlot determines the primary eligible slot for a task, respecting circuit health
// and transparently cascading to fallback slots if the primary is down.
func (r *DynamicModelRouter) ResolveSlot(ctx context.Context, task ReviewTaskType, workspaceID string) (ReviewModelSlot, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. Check workspace override
	slotID := ""
	if workspaceID != "" && r.workspaceConfigs[workspaceID] != nil {
		slotID = r.workspaceConfigs[workspaceID][task]
	}

	// 2. Fall back to task default
	if slotID == "" {
		slotID = r.taskDefaultSlots[task]
	}

	if slotID == "" {
		return ReviewModelSlot{}, false, fmt.Errorf("no slot configured for task %s", task)
	}

	primarySlot, exists := r.slots[slotID]
	if !exists {
		return ReviewModelSlot{}, false, fmt.Errorf("primary slot %s not found", slotID)
	}

	// 3. Check health of primary slot
	health := r.health[slotID]
	if r.isHealthy(health) {
		return primarySlot, false, nil
	}

	// 4. Primary is unhealthy: Cascade to fallback chain
	visited := map[string]bool{slotID: true}
	currentFallbackID := primarySlot.FallbackSlotID

	for currentFallbackID != "" && !visited[currentFallbackID] {
		visited[currentFallbackID] = true
		fbSlot, ok := r.slots[currentFallbackID]
		if !ok {
			break
		}

		fbHealth := r.health[currentFallbackID]
		if r.isHealthy(fbHealth) {
			return fbSlot, true, nil
		}

		currentFallbackID = fbSlot.FallbackSlotID
	}

	// If all cascade slots are down, return the primary in half-open trial or fail
	return primarySlot, false, errors.New("all model slots in fallback cascade are currently unavailable")
}

func (r *DynamicModelRouter) isHealthy(h *SlotHealth) bool {
	if h == nil {
		return true
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	switch h.CircuitState {
	case CircuitClosed:
		return true
	case CircuitHalfOpen:
		return true
	case CircuitOpen:
		// Check if cooldown has elapsed to transition to Half-Open
		if time.Since(h.CircuitOpenedTime) >= h.CoolDownPeriod {
			h.CircuitState = CircuitHalfOpen
			return true
		}
		return false
	default:
		return true
	}
}

// RecordSuccess updates metrics when an LLM call succeeds.
func (r *DynamicModelRouter) RecordSuccess(slotID string, tokensConsumed int) {
	r.mu.RLock()
	health, ok := r.health[slotID]
	r.mu.RUnlock()

	if !ok {
		return
	}

	health.mu.Lock()
	defer health.mu.Unlock()

	health.TotalCalls++
	health.ConsecutiveFailures = 0
	health.TotalTokensConsumed += int64(tokensConsumed)
	health.LastSuccessTime = time.Now().UTC()

	// If it was half-open, close it back to healthy
	if health.CircuitState == CircuitHalfOpen {
		health.CircuitState = CircuitClosed
	}
}

// RecordFailure updates metrics when an LLM call fails, evaluating whether to trip the circuit.
// Returns true if the circuit tripped to OPEN.
func (r *DynamicModelRouter) RecordFailure(slotID string, err error) bool {
	r.mu.RLock()
	health, ok := r.health[slotID]
	r.mu.RUnlock()

	if !ok {
		return false
	}

	// Classify error: do not trip on client cancellations or prompt overflow
	if !ShouldTripCircuit(err) {
		return false
	}

	health.mu.Lock()
	defer health.mu.Unlock()

	health.TotalCalls++
	health.TotalErrors++
	health.ConsecutiveFailures++
	health.LastFailureTime = time.Now().UTC()

	if health.CircuitState == CircuitHalfOpen || health.ConsecutiveFailures >= r.failureThreshold {
		health.CircuitState = CircuitOpen
		health.CircuitOpenedTime = time.Now().UTC()
		return true
	}

	return false
}

// ShouldTripCircuit evaluates if an error represents an infrastructure/provider failure.
func ShouldTripCircuit(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())

	// Non-trip errors: client-side budget, token context limit, prompt length
	if strings.Contains(msg, "context_overflow") ||
		strings.Contains(msg, "max tokens exceeded") ||
		strings.Contains(msg, "canceled") ||
		strings.Contains(msg, "context deadline exceeded") {
		return false
	}

	// Trip errors: 5xx, service unavailable, connection reset, authentication invalid
	if strings.Contains(msg, "500") ||
		strings.Contains(msg, "502") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "504") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "429") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "quota exceeded") {
		return true
	}

	return false
}

// GetSlotHealth returns a snapshot of slot health metrics.
func (r *DynamicModelRouter) GetSlotHealth(slotID string) (SlotHealth, bool) {
	r.mu.RLock()
	health, ok := r.health[slotID]
	r.mu.RUnlock()

	if !ok {
		return SlotHealth{}, false
	}

	health.mu.RLock()
	defer health.mu.RUnlock()

	return SlotHealth{
		ConsecutiveFailures: health.ConsecutiveFailures,
		TotalCalls:          health.TotalCalls,
		TotalErrors:         health.TotalErrors,
		TotalTokensConsumed: health.TotalTokensConsumed,
		CircuitState:        health.CircuitState,
		LastFailureTime:     health.LastFailureTime,
		LastSuccessTime:     health.LastSuccessTime,
		CircuitOpenedTime:   health.CircuitOpenedTime,
		CoolDownPeriod:      health.CoolDownPeriod,
	}, true
}
