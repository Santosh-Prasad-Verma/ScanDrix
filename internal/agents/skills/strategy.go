// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Dynamic Skills Capability Strategy
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package skills

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	MinPromotionSuccesses   = 3
	MinPromotionSuccessRate = 0.70
)

// CapabilityStrategyScope identifies the tenancy and skill boundary for strategy caching.
type CapabilityStrategyScope struct {
	OrganizationID string `json:"organization_id,omitempty"`
	TeamID         string `json:"team_id,omitempty"`
	RepositoryID   string `json:"repository_id,omitempty"`
	SkillName      string `json:"skill_name,omitempty"`
	Capability     string `json:"capability,omitempty"`
	Provider       string `json:"provider,omitempty"`
}

func (s CapabilityStrategyScope) Key() string {
	return fmt.Sprintf("strategy:%s:%s:%s:%s:%s:%s",
		s.OrganizationID, s.TeamID, s.RepositoryID, s.SkillName, s.Capability, s.Provider)
}

// ToolStrategyStats records operational performance metrics for a specific tool.
type ToolStrategyStats struct {
	SuccessCount   int       `json:"success_count"`
	FailedCount    int       `json:"failed_count"`
	SkippedCount   int       `json:"skipped_count"`
	TotalLatencyMs int64     `json:"total_latency_ms"`
	LastUsedAt     time.Time `json:"last_used_at"`
}

// CapabilityExecutionTrace captures the outcome of a single tool execution attempt.
type CapabilityExecutionTrace struct {
	Scope      CapabilityStrategyScope
	ToolName   string
	Status     string // "success" | "failed" | "skipped"
	LatencyMs  int64
	OccurredAt time.Time
}

// CapabilityStrategyState stores the learned rankings and preferred tool for a scope.
type CapabilityStrategyState struct {
	Scope         CapabilityStrategyScope       `json:"scope"`
	PreferredTool string                        `json:"preferred_tool,omitempty"`
	Promoted      bool                          `json:"promoted"`
	ToolStats     map[string]*ToolStrategyStats `json:"tool_stats"`
	CachedTools   []string                      `json:"cached_tools,omitempty"`
	UpdatedAt     time.Time                     `json:"updated_at"`
}

// CapabilityStrategyService learns and caches the most reliable tools for dynamic capabilities.
type CapabilityStrategyService struct {
	mu     sync.RWMutex
	states map[string]*CapabilityStrategyState
}

// NewCapabilityStrategyService constructs a new strategy learning service.
func NewCapabilityStrategyService() *CapabilityStrategyService {
	return &CapabilityStrategyService{
		states: make(map[string]*CapabilityStrategyState),
	}
}

// GetPreferredTool returns the highest scoring or promoted tool among candidates.
func (s *CapabilityStrategyService) GetPreferredTool(scope CapabilityStrategyScope, candidateTools []string) (string, bool) {
	if len(candidateTools) == 0 {
		return "", false
	}

	s.mu.RLock()
	state, exists := s.states[scope.Key()]
	s.mu.RUnlock()

	if !exists || state == nil {
		return "", false
	}

	// 1. If a tool has been formally promoted, check if it's in candidates
	if state.Promoted && state.PreferredTool != "" {
		for _, c := range candidateTools {
			if strings.EqualFold(c, state.PreferredTool) {
				return state.PreferredTool, true
			}
		}
	}

	// 2. Score candidate tools
	type scoredTool struct {
		name  string
		score int
	}

	var scored []scoredTool
	for _, c := range candidateTools {
		stats, ok := state.ToolStats[c]
		if ok {
			score := s.computeToolScore(stats)
			scored = append(scored, scoredTool{name: c, score: score})
		}
	}

	if len(scored) == 0 {
		return "", false
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	if scored[0].score > 0 {
		return scored[0].name, true
	}

	return "", false
}

// RecordExecution updates performance metrics for a tool and evaluates promotion.
func (s *CapabilityStrategyService) RecordExecution(trace CapabilityExecutionTrace) {
	if trace.ToolName == "" {
		return
	}

	key := trace.Scope.Key()

	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.states[key]
	if !exists {
		state = &CapabilityStrategyState{
			Scope:     trace.Scope,
			ToolStats: make(map[string]*ToolStrategyStats),
		}
		s.states[key] = state
	}

	stats, ok := state.ToolStats[trace.ToolName]
	if !ok {
		stats = &ToolStrategyStats{LastUsedAt: trace.OccurredAt}
		state.ToolStats[trace.ToolName] = stats
	}

	switch trace.Status {
	case "success":
		stats.SuccessCount++
	case "failed":
		stats.FailedCount++
	default:
		stats.SkippedCount++
	}

	if trace.LatencyMs > 0 {
		stats.TotalLatencyMs += trace.LatencyMs
	}
	stats.LastUsedAt = trace.OccurredAt
	state.UpdatedAt = trace.OccurredAt

	s.promotePreferredTool(state)
}

// GetCachedTools retrieves previously verified tools for this scope.
func (s *CapabilityStrategyService) GetCachedTools(scope CapabilityStrategyScope) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, exists := s.states[scope.Key()]
	if !exists || state == nil || len(state.CachedTools) == 0 {
		return nil
	}
	res := make([]string, len(state.CachedTools))
	copy(res, state.CachedTools)
	return res
}

// SaveCachedTools saves the verified list of tools for this scope.
func (s *CapabilityStrategyService) SaveCachedTools(scope CapabilityStrategyScope, tools []string) {
	key := scope.Key()

	s.mu.Lock()
	defer s.mu.Unlock()

	state, exists := s.states[key]
	if !exists {
		state = &CapabilityStrategyState{
			Scope:     scope,
			ToolStats: make(map[string]*ToolStrategyStats),
		}
		s.states[key] = state
	}

	state.CachedTools = append([]string(nil), tools...)
	state.UpdatedAt = time.Now().UTC()
}

func (s *CapabilityStrategyService) promotePreferredTool(state *CapabilityStrategyState) {
	type toolCandidate struct {
		name  string
		stats *ToolStrategyStats
		score int
	}

	var candidates []toolCandidate
	for name, stats := range state.ToolStats {
		candidates = append(candidates, toolCandidate{
			name:  name,
			stats: stats,
			score: s.computeToolScore(stats),
		})
	}

	if len(candidates) == 0 {
		state.Promoted = false
		state.PreferredTool = ""
		return
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	top := candidates[0]
	attempts := top.stats.SuccessCount + top.stats.FailedCount
	var successRate float64
	if attempts > 0 {
		successRate = float64(top.stats.SuccessCount) / float64(attempts)
	}

	canPromote := top.stats.SuccessCount >= MinPromotionSuccesses && successRate >= MinPromotionSuccessRate

	state.Promoted = canPromote
	if canPromote {
		state.PreferredTool = top.name
	} else {
		state.PreferredTool = ""
	}
}

func (s *CapabilityStrategyService) computeToolScore(stats *ToolStrategyStats) int {
	if stats == nil {
		return 0
	}
	return stats.SuccessCount*2 - stats.FailedCount - stats.SkippedCount
}
