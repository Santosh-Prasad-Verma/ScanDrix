package streaming

import (
	"time"

	"github.com/google/uuid"
)

// EventType categorizes real-time review progress events.
type EventType string

const (
	EventStageTransition    EventType = "STAGE_TRANSITION"
	EventFindingDiscovered  EventType = "FINDING_DISCOVERED"
	EventCritiqueDebate     EventType = "CRITIQUE_DEBATE"
	EventReviewCompleted    EventType = "REVIEW_COMPLETED"
	EventHeartbeat          EventType = "HEARTBEAT"
)

// StreamEvent represents an atomic telemetry packet broadcast to subscribers.
type StreamEvent struct {
	ID        uuid.UUID `json:"id"`
	ReviewID  uuid.UUID `json:"review_id"`
	Type      EventType `json:"type"`
	Payload   any       `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

// StageTransitionPayload carries pipeline stage progress.
type StageTransitionPayload struct {
	StageName string  `json:"stage_name"`
	Progress  float64 `json:"progress"` // 0.0 to 1.0
	Message   string  `json:"message"`
}

// ClientSubscription tracks a connected WebSocket or SSE client.
type ClientSubscription struct {
	ClientID   string
	ReviewID   uuid.UUID
	EventChan  chan StreamEvent
	ClosedChan chan struct{}
}
