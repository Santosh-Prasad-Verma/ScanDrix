package repositories

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/clireview/domain"
)

// SessionEventRecord represents a fully persisted session event with classification state.
type SessionEventRecord struct {
	UUID                 string                                `json:"uuid"`
	OrganizationID       string                                `json:"organizationId"`
	TeamID               string                                `json:"teamId"`
	SessionID            string                                `json:"sessionId"`
	Type                 string                                `json:"type"` // "session_start", "turn_start", "turn_end", "subagent_start", "subagent_end", "session_end"
	Branch               string                                `json:"branch"`
	EventTimestamp       time.Time                             `json:"eventTimestamp"`
	Payload              map[string]any                        `json:"payload"`
	ClassificationStatus string                                `json:"classificationStatus,omitempty"` // "PROCESSING", "COMPLETED", "FAILED", "SKIPPED"
	Decisions            []domain.CliSessionClassifiedDecision `json:"decisions,omitempty"`
	ClassificationSource string                                `json:"classificationSource,omitempty"` // "llm", "heuristic", "heuristic-fallback", "empty"
	ClassificationError  string                                `json:"classificationError,omitempty"`
	ClassifiedAt         *time.Time                            `json:"classifiedAt,omitempty"`
	CreatedAt            time.Time                             `json:"createdAt"`
	UpdatedAt            time.Time                             `json:"updatedAt"`
}

// SessionEventRepository provides thread-safe in-memory and relational storage for telemetry events.
type SessionEventRepository struct {
	mu     sync.RWMutex
	events map[string]*SessionEventRecord // uuid -> event
	bySess map[string][]string            // orgId:sessionId -> [uuid]
}

// NewSessionEventRepository creates an initialized SessionEventRepository.
func NewSessionEventRepository() *SessionEventRepository {
	return &SessionEventRepository{
		events: make(map[string]*SessionEventRecord),
		bySess: make(map[string][]string),
	}
}

// Create inserts a new session event record.
func (r *SessionEventRepository) Create(ctx context.Context, event *domain.SessionEvent) (*domain.SessionEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := event.ID
	if id == "" {
		id = uuid.New().String()
		event.ID = id
	}

	createdAt := event.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
		event.CreatedAt = createdAt
	}

	branch := "unknown"
	if b, ok := event.Payload["branch"].(string); ok && b != "" {
		branch = b
	}

	record := &SessionEventRecord{
		UUID:           id,
		OrganizationID: event.OrganizationID,
		TeamID:         event.TeamID,
		SessionID:      event.SessionID,
		Type:           event.EventType,
		Branch:         branch,
		EventTimestamp: createdAt,
		Payload:        event.Payload,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	}

	r.events[id] = record
	key := fmt.Sprintf("%s:%s", event.OrganizationID, event.SessionID)
	r.bySess[key] = append(r.bySess[key], id)

	return event, nil
}

// FindByUUID retrieves an event by its unique ID.
func (r *SessionEventRepository) FindByUUID(ctx context.Context, id string) (*domain.SessionEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, ok := r.events[id]
	if !ok {
		return nil, nil
	}

	return &domain.SessionEvent{
		ID:             rec.UUID,
		OrganizationID: rec.OrganizationID,
		TeamID:         rec.TeamID,
		SessionID:      rec.SessionID,
		EventType:      rec.Type,
		Payload:        rec.Payload,
		CreatedAt:      rec.EventTimestamp,
	}, nil
}

// FindBySessionID retrieves all chronological events for a given session.
func (r *SessionEventRepository) FindBySessionID(ctx context.Context, sessionID, orgID string) ([]*domain.SessionEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", orgID, sessionID)
	ids := r.bySess[key]

	var res []*domain.SessionEvent
	for _, id := range ids {
		if rec, ok := r.events[id]; ok {
			res = append(res, &domain.SessionEvent{
				ID:             rec.UUID,
				OrganizationID: rec.OrganizationID,
				TeamID:         rec.TeamID,
				SessionID:      rec.SessionID,
				EventType:      rec.Type,
				Payload:        rec.Payload,
				CreatedAt:      rec.EventTimestamp,
			})
		}
	}

	sort.Slice(res, func(i, j int) bool {
		return res[i].CreatedAt.Before(res[j].CreatedAt)
	})

	return res, nil
}

// MarkClassificationProcessing marks the session_end event as currently being analyzed.
func (r *SessionEventRepository) MarkClassificationProcessing(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.events[id]
	if !ok {
		return fmt.Errorf("session event %s not found", id)
	}

	rec.ClassificationStatus = "PROCESSING"
	rec.ClassificationError = ""
	rec.UpdatedAt = time.Now().UTC()
	return nil
}

// MarkClassificationCompleted updates the session_end event with decisions and provenance.
func (r *SessionEventRepository) MarkClassificationCompleted(
	ctx context.Context,
	id string,
	decisions []domain.CliSessionClassifiedDecision,
	source string,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.events[id]
	if !ok {
		return fmt.Errorf("session event %s not found", id)
	}

	now := time.Now().UTC()
	rec.ClassificationStatus = "COMPLETED"
	rec.Decisions = decisions
	rec.ClassificationSource = source
	rec.ClassifiedAt = &now
	rec.ClassificationError = ""
	rec.UpdatedAt = now
	return nil
}

// MarkClassificationFailed records classification failure details.
func (r *SessionEventRepository) MarkClassificationFailed(ctx context.Context, id string, errorMessage string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.events[id]
	if !ok {
		return fmt.Errorf("session event %s not found", id)
	}

	now := time.Now().UTC()
	rec.ClassificationStatus = "FAILED"
	rec.ClassificationError = errorMessage
	rec.ClassifiedAt = &now
	rec.UpdatedAt = now
	return nil
}

// MarkClassificationSkipped records reason why classification was bypassed.
func (r *SessionEventRepository) MarkClassificationSkipped(ctx context.Context, id string, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.events[id]
	if !ok {
		return fmt.Errorf("session event %s not found", id)
	}

	now := time.Now().UTC()
	rec.ClassificationStatus = "SKIPPED"
	rec.ClassificationError = reason
	rec.ClassifiedAt = &now
	rec.UpdatedAt = now
	return nil
}

// FindOrphanedSessions finds inactive sessions without a session_end or classification.
func (r *SessionEventRepository) FindOrphanedSessions(
	ctx context.Context,
	inactivityMinutes int,
	limit int,
) ([]domain.OrphanedSessionRef, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	threshold := time.Now().UTC().Add(-time.Duration(inactivityMinutes) * time.Minute)

	type sessionAgg struct {
		orgID        string
		sessID       string
		teamID       string
		branch       string
		hasEnd       bool
		hasClass     bool
		maxTimestamp time.Time
	}

	aggs := make(map[string]*sessionAgg)

	for _, rec := range r.events {
		key := fmt.Sprintf("%s:%s", rec.OrganizationID, rec.SessionID)
		agg, ok := aggs[key]
		if !ok {
			agg = &sessionAgg{
				orgID:        rec.OrganizationID,
				sessID:       rec.SessionID,
				teamID:       rec.TeamID,
				branch:       rec.Branch,
				maxTimestamp: rec.EventTimestamp,
			}
			aggs[key] = agg
		}

		if rec.Type == "session_end" {
			agg.hasEnd = true
		}
		if rec.ClassificationStatus != "" {
			agg.hasClass = true
		}
		if rec.EventTimestamp.After(agg.maxTimestamp) {
			agg.maxTimestamp = rec.EventTimestamp
			agg.branch = rec.Branch
			agg.teamID = rec.TeamID
		}
	}

	var candidates []domain.OrphanedSessionRef
	for _, agg := range aggs {
		if !agg.hasEnd && !agg.hasClass && agg.maxTimestamp.Before(threshold) {
			candidates = append(candidates, domain.OrphanedSessionRef{
				SessionID:      agg.sessID,
				OrganizationID: agg.orgID,
				TeamID:         agg.teamID,
				Branch:         agg.branch,
				LastEventAt:    agg.maxTimestamp,
			})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].LastEventAt.Before(candidates[j].LastEventAt)
	})

	if len(candidates) > limit && limit > 0 {
		candidates = candidates[:limit]
	}

	return candidates, nil
}

// FindUnclassifiedSyntheticEnds locates synthetic session_end events that crashed before classification.
func (r *SessionEventRepository) FindUnclassifiedSyntheticEnds(ctx context.Context, limit int) ([]*SessionEventRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var res []*SessionEventRecord
	for _, rec := range r.events {
		if rec.Type == "session_end" && rec.ClassificationStatus == "" {
			if synth, ok := rec.Payload["synthetic"].(bool); ok && synth {
				res = append(res, rec)
			} else if synthStr, ok := rec.Payload["synthetic"].(string); ok && synthStr == "true" {
				res = append(res, rec)
			}
		}
	}

	sort.Slice(res, func(i, j int) bool {
		return res[i].EventTimestamp.Before(res[j].EventTimestamp)
	})

	if len(res) > limit && limit > 0 {
		res = res[:limit]
	}

	return res, nil
}
