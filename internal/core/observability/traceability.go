package observability

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/core/log"
)

// TraceabilitySummary summarizes logs, spans, and overall status for a correlation ID.
type TraceabilitySummary struct {
	TotalLogs      int        `json:"totalLogs"`
	TotalTelemetry int        `json:"totalTelemetry"`
	StartTime      *time.Time `json:"startTime,omitempty"`
	EndTime        *time.Time `json:"endTime,omitempty"`
	DurationMs     int64      `json:"durationMs,omitempty"`
	Status         string     `json:"status"` // "success" | "error" | "running"
}

// TraceabilityTimelineItem represents an entry in the chronological trace timeline.
type TraceabilityTimelineItem struct {
	Timestamp    time.Time `json:"timestamp"`
	Type         string    `json:"type"` // "log" | "telemetry"
	Component    string    `json:"component,omitempty"`
	Message      string    `json:"message,omitempty"`
	Name         string    `json:"name,omitempty"`
	Level        string    `json:"level,omitempty"`
	DurationMs   int64     `json:"durationMs,omitempty"`
	ErrorMessage string    `json:"errorMessage,omitempty"`
	Phase        string    `json:"phase,omitempty"`
	AgentName    string    `json:"agentName,omitempty"`
	ToolName     string    `json:"toolName,omitempty"`
}

// TraceabilityExecution captures agent orchestration steps and inputs/outputs.
type TraceabilityExecution struct {
	ExecutionID string                   `json:"executionId,omitempty"`
	AgentName   string                   `json:"agentName,omitempty"`
	SessionID   string                   `json:"sessionId,omitempty"`
	TenantID    string                   `json:"tenantId,omitempty"`
	Input       interface{}              `json:"input,omitempty"`
	Output      interface{}              `json:"output,omitempty"`
	Steps       []map[string]interface{} `json:"steps,omitempty"`
}

// TraceabilityResponse returns the complete consolidated trace for audit and inspection.
type TraceabilityResponse struct {
	CorrelationID string                     `json:"correlationId"`
	Summary       TraceabilitySummary        `json:"summary"`
	Timeline      []TraceabilityTimelineItem `json:"timeline"`
	Execution     TraceabilityExecution      `json:"execution"`
}

// TraceabilityService retrieves and stitches together logs, spans, and agent steps.
type TraceabilityService struct {
	mu     sync.RWMutex
	store  map[string]*TraceabilityResponse
	logger *log.StructuredLogger
}

// NewTraceabilityService constructs a new TraceabilityService instance.
func NewTraceabilityService() *TraceabilityService {
	return &TraceabilityService{
		store:  make(map[string]*TraceabilityResponse),
		logger: log.CreateLogger("TraceabilityService"),
	}
}

// RecordTimelineItem appends a log or span observation into a correlation trace.
func (s *TraceabilityService) RecordTimelineItem(correlationID string, item TraceabilityTimelineItem) {
	s.mu.Lock()
	defer s.mu.Unlock()

	resp, ok := s.store[correlationID]
	if !ok {
		resp = &TraceabilityResponse{
			CorrelationID: correlationID,
			Summary: TraceabilitySummary{
				Status: "running",
			},
			Timeline: make([]TraceabilityTimelineItem, 0),
		}
		s.store[correlationID] = resp
	}

	resp.Timeline = append(resp.Timeline, item)

	if item.Type == "log" {
		resp.Summary.TotalLogs++
	} else {
		resp.Summary.TotalTelemetry++
	}

	if item.ErrorMessage != "" || item.Level == "error" {
		resp.Summary.Status = "error"
	}
}

// GetTraceability retrieves the consolidated timeline and summary for a correlation ID.
func (s *TraceabilityService) GetTraceability(ctx context.Context, correlationID string) (*TraceabilityResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resp, ok := s.store[correlationID]
	if !ok {
		return &TraceabilityResponse{
			CorrelationID: correlationID,
			Summary: TraceabilitySummary{
				Status: "not_found",
			},
			Timeline: []TraceabilityTimelineItem{},
		}, nil
	}

	// Sort timeline chronologically
	sortedTimeline := make([]TraceabilityTimelineItem, len(resp.Timeline))
	copy(sortedTimeline, resp.Timeline)
	sort.Slice(sortedTimeline, func(i, j int) bool {
		return sortedTimeline[i].Timestamp.Before(sortedTimeline[j].Timestamp)
	})

	result := *resp
	result.Timeline = sortedTimeline
	if len(sortedTimeline) > 0 {
		start := sortedTimeline[0].Timestamp
		end := sortedTimeline[len(sortedTimeline)-1].Timestamp
		result.Summary.StartTime = &start
		result.Summary.EndTime = &end
		result.Summary.DurationMs = end.Sub(start).Milliseconds()
		if result.Summary.Status != "error" {
			result.Summary.Status = "success"
		}
	}

	return &result, nil
}
