package warehouse

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EventStore manages the append-only log of domain events in-memory.
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

// PostgresEventStore persists domain events into the PostgreSQL warehouse_domain_events table.
type PostgresEventStore struct {
	pool *pgxpool.Pool
}

// NewPostgresEventStore creates a PostgreSQL-backed event store.
func NewPostgresEventStore(pool *pgxpool.Pool) *PostgresEventStore {
	return &PostgresEventStore{pool: pool}
}

// Append persists a domain event into the PostgreSQL database.
func (s *PostgresEventStore) Append(ctx context.Context, evt DomainEvent) error {
	if s.pool == nil {
		return nil
	}

	query := `
		INSERT INTO warehouse_domain_events (
			id, workspace_id, aggregate_id, aggregate_type, event_type, version, payload, occurred_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	now := time.Now().UTC()
	if evt.EventID == uuid.Nil {
		evt.EventID = uuid.New()
	}
	if evt.OccurredAt.IsZero() {
		evt.OccurredAt = now
	}

	_, err := s.pool.Exec(ctx, query,
		evt.EventID, evt.WorkspaceID, evt.AggregateID, evt.AggregateType,
		string(evt.EventType), evt.Version, evt.Payload, evt.OccurredAt, now,
	)
	if err != nil {
		return fmt.Errorf("failed to persist domain event to warehouse: %w", err)
	}
	return nil
}

// AppendBatch stores multiple events using a single batch transaction.
func (s *PostgresEventStore) AppendBatch(ctx context.Context, tenantID uuid.UUID, evts []DomainEvent) error {
	if s.pool == nil || len(evts) == 0 {
		return nil
	}

	query := `
		INSERT INTO warehouse_domain_events (
			id, workspace_id, aggregate_id, aggregate_type, event_type, version, payload, occurred_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	batch := &pgx.Batch{}
	now := time.Now().UTC()
	for _, evt := range evts {
		id := evt.EventID
		if id == uuid.Nil {
			id = uuid.New()
		}
		occ := evt.OccurredAt
		if occ.IsZero() {
			occ = now
		}
		batch.Queue(query, id, evt.WorkspaceID, evt.AggregateID, evt.AggregateType, string(evt.EventType), evt.Version, evt.Payload, occ, now)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range evts {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("failed inserting batch warehouse event: %w", err)
		}
	}
	return nil
}
