package warehouse

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// EventType categorizes domain events in the append-only audit stream.
type EventType string

const (
	EventReviewTriggered  EventType = "REVIEW_TRIGGERED"
	EventStageCompleted   EventType = "STAGE_COMPLETED"
	EventFindingDetected  EventType = "FINDING_DETECTED"
	EventFindingTriaged   EventType = "FINDING_TRIAGED"
	EventFindingDismissed EventType = "FINDING_DISMISSED"
	EventFindingResolved  EventType = "FINDING_RESOLVED"
	EventFindingRegressed EventType = "FINDING_REGRESSED"
	EventMergeBlocked     EventType = "MERGE_BLOCKED"
	EventMergeApproved    EventType = "MERGE_APPROVED"
	EventRuleSynced       EventType = "RULE_SYNCED"
	EventLicenseActivated EventType = "LICENSE_ACTIVATED"
)

// DomainEvent represents an immutable state change recorded in the enterprise ledger.
type DomainEvent struct {
	EventID       uuid.UUID       `json:"event_id"`
	WorkspaceID   uuid.UUID       `json:"workspace_id"`
	AggregateID   uuid.UUID       `json:"aggregate_id"`
	AggregateType string          `json:"aggregate_type"` // "REVIEW", "FINDING", "WORKSPACE", "RULE"
	EventType     EventType       `json:"event_type"`
	Payload       json.RawMessage `json:"payload"`
	ActorID       uuid.UUID       `json:"actor_id,omitempty"`
	ActorEmail    string          `json:"actor_email,omitempty"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Version       int64           `json:"version"`
}

// EventFilter defines search criteria to query the domain event stream.
type EventFilter struct {
	WorkspaceID   uuid.UUID   `json:"workspace_id"`
	AggregateID   *uuid.UUID  `json:"aggregate_id,omitempty"`
	AggregateType string      `json:"aggregate_type,omitempty"`
	EventTypes    []EventType `json:"event_types,omitempty"`
	Since         *time.Time  `json:"since,omitempty"`
	Until         *time.Time  `json:"until,omitempty"`
	Limit         int         `json:"limit,omitempty"`
}

// NewDomainEvent constructs an event with timestamp and UUID.
func NewDomainEvent(wsID, aggID uuid.UUID, aggType string, eventType EventType, payload any, actorEmail string) (DomainEvent, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return DomainEvent{}, err
	}

	return DomainEvent{
		EventID:       uuid.New(),
		WorkspaceID:   wsID,
		AggregateID:   aggID,
		AggregateType: aggType,
		EventType:     eventType,
		Payload:       raw,
		ActorEmail:    actorEmail,
		OccurredAt:    time.Now().UTC(),
		Version:       1,
	}, nil
}
