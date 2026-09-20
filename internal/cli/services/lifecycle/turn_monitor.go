// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package lifecycle

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// ToolExecutionStat captures execution latency and outcome for a single tool call.
type ToolExecutionStat struct {
	ToolName   string        `json:"tool_name"`
	Duration   time.Duration `json:"duration"`
	Success    bool          `json:"success"`
	OutputSize int           `json:"output_size"`
	Timestamp  time.Time     `json:"timestamp"`
}

// TurnMetrics aggregates runtime performance and cost metrics for a session turn.
type TurnMetrics struct {
	TurnID          string                       `json:"turn_id"`
	SessionID       string                       `json:"session_id"`
	StartedAt       time.Time                    `json:"started_at"`
	EndedAt         time.Time                    `json:"ended_at"`
	Duration        time.Duration                `json:"duration"`
	ToolCalls       []ToolExecutionStat          `json:"tool_calls"`
	TotalToolsCount int                          `json:"total_tools_count"`
	FailedToolsCount int                         `json:"failed_tools_count"`
	FilesModified   []string                     `json:"files_modified"`
	FilesRead       []string                     `json:"files_read"`
	EstimatedTokens int                          `json:"estimated_tokens"`
}

// TurnMonitor provides thread-safe real-time performance and tool execution tracking.
type TurnMonitor struct {
	mu           sync.RWMutex
	activeTurns  map[string]*TurnMetrics
	history      []TurnMetrics
	maxHistory   int
}

// NewTurnMonitor initializes a TurnMonitor instance.
func NewTurnMonitor(maxHistory int) *TurnMonitor {
	if maxHistory <= 0 {
		maxHistory = 100
	}
	return &TurnMonitor{
		activeTurns: make(map[string]*TurnMetrics),
		history:     make([]TurnMetrics, 0),
		maxHistory:  maxHistory,
	}
}

// StartTurn begins tracking a new turn.
func (m *TurnMonitor) StartTurn(sessionID, turnID string) *TurnMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()

	metrics := &TurnMetrics{
		TurnID:        turnID,
		SessionID:     sessionID,
		StartedAt:     time.Now().UTC(),
		ToolCalls:     make([]ToolExecutionStat, 0),
		FilesModified: make([]string, 0),
		FilesRead:     make([]string, 0),
	}

	m.activeTurns[turnID] = metrics
	return metrics
}

// RecordToolExecution records a single tool invocation's performance.
func (m *TurnMonitor) RecordToolExecution(turnID, toolName string, duration time.Duration, success bool, outputBytes int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	turn, ok := m.activeTurns[turnID]
	if !ok {
		return
	}

	stat := ToolExecutionStat{
		ToolName:   toolName,
		Duration:   duration,
		Success:    success,
		OutputSize: outputBytes,
		Timestamp:  time.Now().UTC(),
	}

	turn.ToolCalls = append(turn.ToolCalls, stat)
	turn.TotalToolsCount++
	if !success {
		turn.FailedToolsCount++
	}
}

// RecordFileAccess logs files read or modified during the turn.
func (m *TurnMonitor) RecordFileAccess(turnID string, readFiles, modifiedFiles []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	turn, ok := m.activeTurns[turnID]
	if !ok {
		return
	}

	turn.FilesRead = append(turn.FilesRead, readFiles...)
	turn.FilesModified = append(turn.FilesModified, modifiedFiles...)
}

// EndTurn completes tracking for the specified turn and moves it to history.
func (m *TurnMonitor) EndTurn(turnID string, estimatedTokens int) (*TurnMetrics, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	turn, ok := m.activeTurns[turnID]
	if !ok {
		return nil, fmt.Errorf("active turn %q not found", turnID)
	}

	turn.EndedAt = time.Now().UTC()
	turn.Duration = turn.EndedAt.Sub(turn.StartedAt)
	turn.EstimatedTokens = estimatedTokens

	delete(m.activeTurns, turnID)

	m.history = append(m.history, *turn)
	if len(m.history) > m.maxHistory {
		m.history = m.history[len(m.history)-m.maxHistory:]
	}

	return turn, nil
}

// GetHistory returns copies of completed turn metrics.
func (m *TurnMonitor) GetHistory() []TurnMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]TurnMetrics, len(m.history))
	copy(result, m.history)
	return result
}

// FormatTurnSummary returns a user-friendly terminal line summarizing turn efficiency.
func (m *TurnMonitor) FormatTurnSummary(metrics *TurnMetrics) string {
	if metrics == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚡ Turn %s: %v elapsed | %d tool(s) run",
		metrics.TurnID, metrics.Duration.Round(time.Millisecond), metrics.TotalToolsCount))

	if metrics.FailedToolsCount > 0 {
		sb.WriteString(fmt.Sprintf(" (%d failed)", metrics.FailedToolsCount))
	}

	if len(metrics.FilesModified) > 0 {
		sb.WriteString(fmt.Sprintf(" | %d file(s) modified", len(metrics.FilesModified)))
	}

	return sb.String()
}
