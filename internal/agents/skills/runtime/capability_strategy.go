package runtime

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

const (
	minPromotionSuccesses   = 3
	minPromotionSuccessRate = 0.7
)

// ToolStrategyStats holds historical success and latency metrics for a tool.
type ToolStrategyStats struct {
	SuccessCount   int       `json:"successCount"`
	FailedCount    int       `json:"failedCount"`
	SkippedCount   int       `json:"skippedCount"`
	TotalLatencyMs int64     `json:"totalLatencyMs"`
	LastUsedAt     time.Time `json:"lastUsedAt"`
}

// CapabilityStrategyState tracks state for a specific scope.
type CapabilityStrategyState struct {
	Scope         CapabilityStrategyScope       `json:"scope"`
	PreferredTool string                        `json:"preferredTool,omitempty"`
	Promoted      bool                          `json:"promoted"`
	ToolStats     map[string]*ToolStrategyStats `json:"toolStats"`
	UpdatedAt     time.Time                     `json:"updatedAt"`
}

// CapabilityStrategyService manages tool selection heuristics based on execution traces.
type CapabilityStrategyService struct {
	mu          sync.RWMutex
	memoryStore *BoundedMap[string, *CapabilityStrategyState]
}

// NewCapabilityStrategyService creates an initialized strategy service.
func NewCapabilityStrategyService() *CapabilityStrategyService {
	return &CapabilityStrategyService{
		memoryStore: NewBoundedMap[string, *CapabilityStrategyState](512),
	}
}

func scopeKey(scope CapabilityStrategyScope) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s",
		scope.OrganizationID, scope.TeamID, scope.SkillName, scope.Capability, scope.Provider)
}

// GetPreferredTool identifies the highest scoring tool among candidates.
func (s *CapabilityStrategyService) GetPreferredTool(
	scope CapabilityStrategyScope,
	candidateTools []string,
) (string, bool) {
	if len(candidateTools) == 0 {
		return "", false
	}

	key := scopeKey(scope)
	state, found := s.memoryStore.Get(key)
	if !found || state == nil {
		return "", false
	}

	if state.Promoted && state.PreferredTool != "" {
		for _, tool := range candidateTools {
			if tool == state.PreferredTool {
				return state.PreferredTool, true
			}
		}
	}

	type scoredTool struct {
		tool  string
		score float64
	}

	var scored []scoredTool
	for _, tool := range candidateTools {
		stats := state.ToolStats[tool]
		score := s.computeToolScore(stats)
		scored = append(scored, scoredTool{tool: tool, score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	if len(scored) > 0 && scored[0].score > 0 {
		return scored[0].tool, true
	}

	return "", false
}

// RecordExecution updates performance telemetry for a tool within its scope.
func (s *CapabilityStrategyService) RecordExecution(trace CapabilityExecutionTrace) {
	if trace.ToolName == "" {
		return
	}

	scope := CapabilityStrategyScope{
		OrganizationID: trace.OrganizationID,
		TeamID:         trace.TeamID,
		SkillName:      trace.SkillName,
		Capability:     trace.Capability,
		Provider:       trace.Provider,
	}

	key := scopeKey(scope)
	s.mu.Lock()
	defer s.mu.Unlock()

	state, found := s.memoryStore.Get(key)
	if !found || state == nil {
		state = &CapabilityStrategyState{
			Scope:     scope,
			ToolStats: make(map[string]*ToolStrategyStats),
			UpdatedAt: trace.OccurredAt,
		}
	}

	stats, exists := state.ToolStats[trace.ToolName]
	if !exists || stats == nil {
		stats = &ToolStrategyStats{}
		state.ToolStats[trace.ToolName] = stats
	}

	switch trace.Status {
	case StatusSuccess:
		stats.SuccessCount++
	case StatusFailed:
		stats.FailedCount++
	case StatusSkipped:
		stats.SkippedCount++
	}

	stats.TotalLatencyMs += trace.LatencyMs
	stats.LastUsedAt = trace.OccurredAt
	state.UpdatedAt = trace.OccurredAt

	totalRuns := stats.SuccessCount + stats.FailedCount
	if totalRuns >= minPromotionSuccesses {
		successRate := float64(stats.SuccessCount) / float64(totalRuns)
		if successRate >= minPromotionSuccessRate {
			state.Promoted = true
			state.PreferredTool = trace.ToolName
		}
	}

	s.memoryStore.Set(key, state)
}

func (s *CapabilityStrategyService) computeToolScore(stats *ToolStrategyStats) float64 {
	if stats == nil {
		return 0
	}
	total := stats.SuccessCount + stats.FailedCount
	if total == 0 {
		return 0
	}

	successRate := float64(stats.SuccessCount) / float64(total)
	avgLatency := float64(stats.TotalLatencyMs) / float64(total)

	latencyPenalty := avgLatency / 10000.0
	score := successRate - latencyPenalty
	if score < 0 {
		return 0
	}
	return score
}
