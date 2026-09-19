package clireview

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SessionStore provides thread-safe storage for captures and events.
type SessionStore struct {
	mu       sync.RWMutex
	captures map[string]*CliSessionCapture
	events   map[string][]SessionEvent
}

// NewSessionStore creates an initialized session store.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		captures: make(map[string]*CliSessionCapture),
		events:   make(map[string][]SessionEvent),
	}
}

// SaveCapture inserts or updates a session capture.
func (ss *SessionStore) SaveCapture(capture *CliSessionCapture) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.captures[capture.CaptureID] = capture
}

// GetCapture retrieves a session capture by ID.
func (ss *SessionStore) GetCapture(captureID string) (*CliSessionCapture, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	c, ok := ss.captures[captureID]
	return c, ok
}

// AppendEvent stores a session telemetry event.
func (ss *SessionStore) AppendEvent(event SessionEvent) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.events[event.SessionID] = append(ss.events[event.SessionID], event)
}

// GetEvents returns all events for a given session ID.
func (ss *SessionStore) GetEvents(sessionID string) []SessionEvent {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	evs := ss.events[sessionID]
	out := make([]SessionEvent, len(evs))
	copy(out, evs)
	return out
}

// SessionIngester orchestrates event ingestion and session lifecycle.
type SessionIngester struct {
	store      *SessionStore
	classifier *SessionClassifier
}

// NewSessionIngester creates an initialized SessionIngester.
func NewSessionIngester(store *SessionStore, classifier *SessionClassifier) *SessionIngester {
	if store == nil {
		store = NewSessionStore()
	}
	if classifier == nil {
		classifier = NewSessionClassifier()
	}
	return &SessionIngester{
		store:      store,
		classifier: classifier,
	}
}

// IngestEvent records an individual event and associates it with the active session capture.
func (si *SessionIngester) IngestEvent(ctx context.Context, event SessionEvent) (*CliSessionCapture, error) {
	if event.SessionID == "" {
		return nil, errors.New("missing sessionId in session event")
	}

	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}

	si.store.AppendEvent(event)

	// Fetch or create capture
	capture, exists := si.store.GetCapture(event.SessionID)
	if !exists {
		capture = &CliSessionCapture{
			CaptureID:      event.SessionID,
			OrganizationID: event.OrganizationID,
			TeamID:         event.TeamID,
			Event:          event.EventType,
			Status:         "pending",
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
			Signals:        &CliSessionSignals{},
		}
	}

	// Update capture signals based on event payload
	if capture.Signals == nil {
		capture.Signals = &CliSessionSignals{}
	}

	if prompt, ok := event.Payload["prompt"].(string); ok && prompt != "" {
		capture.Signals.Prompt = prompt
	}
	if msg, ok := event.Payload["assistantMessage"].(string); ok && msg != "" {
		capture.Signals.AssistantMessage = msg
	}
	if summary, ok := event.Payload["summary"].(string); ok && summary != "" {
		capture.Summary = summary
	}
	if files, ok := event.Payload["modifiedFiles"].([]string); ok {
		capture.Signals.ModifiedFiles = append(capture.Signals.ModifiedFiles, files...)
	}

	capture.Event = event.EventType
	capture.UpdatedAt = time.Now().UTC()

	// If event signals end of session, classify decisions
	if event.EventType == "stop" || event.EventType == "commit" || event.EventType == "review" {
		si.classifier.ClassifySession(ctx, capture)
	}

	si.store.SaveCapture(capture)
	return capture, nil
}
