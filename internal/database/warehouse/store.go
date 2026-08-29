package warehouse

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// EventStore manages the append-only log of domain events.
type EventStore struct {
	mu     sync.RWMutex
	events []DomainEvent
}

// NewEventStore initializes a thread-safe event ledger.
func NewEventStore() *EventStore {
	return &EventStore{
		events: make([]DomainEvent, 0),
	}
}

// Append records a new domain event atomically into the stream.
func (s *EventStore) Append(ctx context.Context, evt DomainEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	evt.Version = int64(len(s.events) + 1)
	s.events = append(s.events, evt)
	return nil
}

// AppendBatch records multiple events atomically into the stream.
func (s *EventStore) AppendBatch(ctx context.Context, evts []DomainEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range evts {
		evts[i].Version = int64(len(s.events) + 1)
		s.events = append(s.events, evts[i])
	}
	return nil
}

// LoadStream returns all events associated with a specific aggregate in chronological order.
func (s *EventStore) LoadStream(ctx context.Context, aggID uuid.UUID) ([]DomainEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var stream []DomainEvent
	for _, e := range s.events {
		if e.AggregateID == aggID {
			stream = append(stream, e)
		}
	}
	return stream, nil
}

// QueryEvents filters events matching the criteria for compliance audits and analytics.
func (s *EventStore) QueryEvents(ctx context.Context, filter EventFilter) ([]DomainEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []DomainEvent
	typeSet := make(map[EventType]bool)
	for _, t := range filter.EventTypes {
		typeSet[t] = true
	}

	for _, e := range s.events {
		if filter.WorkspaceID != uuid.Nil && e.WorkspaceID != filter.WorkspaceID {
			continue
		}
		if filter.AggregateID != nil && e.AggregateID != *filter.AggregateID {
			continue
		}
		if filter.AggregateType != "" && e.AggregateType != filter.AggregateType {
			continue
		}
		if len(typeSet) > 0 && !typeSet[e.EventType] {
			continue
		}
		if filter.Since != nil && e.OccurredAt.Before(*filter.Since) {
			continue
		}
		if filter.Until != nil && e.OccurredAt.After(*filter.Until) {
			continue
		}

		results = append(results, e)
		if filter.Limit > 0 && len(results) >= filter.Limit {
			break
		}
	}

	return results, nil
}
