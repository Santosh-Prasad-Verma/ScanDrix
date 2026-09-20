package clireview_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/clireview"
)

type mockSessionEventRepo struct {
	mu            sync.Mutex
	createCalls   []*clireview.SessionEvent
	findBySess    []string
	createdEvent  *clireview.SessionEvent
	priorEvents   []*clireview.SessionEvent
	createErr     error
	findBySessErr error
}

func (m *mockSessionEventRepo) Create(ctx context.Context, event *clireview.SessionEvent) (*clireview.SessionEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls = append(m.createCalls, event)
	if m.createErr != nil {
		return nil, m.createErr
	}
	if m.createdEvent != nil {
		return m.createdEvent, nil
	}
	return event, nil
}

func (m *mockSessionEventRepo) FindByUUID(ctx context.Context, uuid string) (*clireview.SessionEvent, error) {
	return nil, nil
}

func (m *mockSessionEventRepo) FindBySessionID(ctx context.Context, sessionID, orgID string) ([]*clireview.SessionEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.findBySess = append(m.findBySess, sessionID)
	if m.findBySessErr != nil {
		return nil, m.findBySessErr
	}
	return m.priorEvents, nil
}

func (m *mockSessionEventRepo) MarkClassificationProcessing(ctx context.Context, uuid string) error {
	return nil
}

func (m *mockSessionEventRepo) MarkClassificationCompleted(ctx context.Context, uuid string, decisions []clireview.CliSessionClassifiedDecision, source string) error {
	return nil
}

func (m *mockSessionEventRepo) MarkClassificationFailed(ctx context.Context, uuid string, errorMessage string) error {
	return nil
}

func (m *mockSessionEventRepo) MarkClassificationSkipped(ctx context.Context, uuid string, reason string) error {
	return nil
}

func (m *mockSessionEventRepo) FindOrphanedSessions(ctx context.Context, inactivityMinutes int, limit int) ([]clireview.OrphanedSessionRef, error) {
	return nil, nil
}

type mockClassifyUseCase struct {
	mu          sync.Mutex
	calls       []string
	err         error
	executedCh  chan string
}

func newMockClassifyUseCase() *mockClassifyUseCase {
	return &mockClassifyUseCase{
		executedCh: make(chan string, 10),
	}
}

func (m *mockClassifyUseCase) Execute(ctx context.Context, sessionEndEventUUID string) error {
	m.mu.Lock()
	m.calls = append(m.calls, sessionEndEventUUID)
	m.mu.Unlock()

	select {
	case m.executedCh <- sessionEndEventUUID:
	default:
	}

	return m.err
}

func TestIngestSessionEventUseCase(t *testing.T) {
	baseParams := clireview.IngestSessionEventInput{
		OrganizationAndTeamData: clireview.OrganizationAndTeamData{
			OrganizationID: "org-1",
			TeamID:         "team-1",
		},
	}

	t.Run("should persist event and return accepted", func(t *testing.T) {
		repo := &mockSessionEventRepo{
			createdEvent: &clireview.SessionEvent{UUID: "evt-1"},
		}
		classify := newMockClassifyUseCase()
		useCase := clireview.NewIngestSessionEventUseCase(repo, classify)

		input := baseParams
		input.SessionID = "sess-1"
		input.Type = "turn_start"
		input.Branch = "main"
		input.Timestamp = time.Now()
		input.Payload = map[string]any{"prompt": "hello"}

		result, err := useCase.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result == nil || !result.Accepted {
			t.Errorf("expected accepted true, got %+v", result)
		}

		repo.mu.Lock()
		defer repo.mu.Unlock()
		if len(repo.createCalls) != 1 {
			t.Fatalf("expected 1 create call, got %d", len(repo.createCalls))
		}
		if repo.createCalls[0].OrganizationID != "org-1" || repo.createCalls[0].SessionID != "sess-1" || repo.createCalls[0].EventType != "turn_start" {
			t.Errorf("unexpected create params: %+v", repo.createCalls[0])
		}
	})

	t.Run("should NOT trigger classification for non session_end events", func(t *testing.T) {
		repo := &mockSessionEventRepo{
			createdEvent: &clireview.SessionEvent{UUID: "evt-1"},
		}
		classify := newMockClassifyUseCase()
		useCase := clireview.NewIngestSessionEventUseCase(repo, classify)

		input := baseParams
		input.SessionID = "sess-1"
		input.Type = "turn_start"
		input.Branch = "main"
		input.Timestamp = time.Now()

		_, err := useCase.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Wait briefly to ensure no goroutine fired
		time.Sleep(30 * time.Millisecond)

		classify.mu.Lock()
		defer classify.mu.Unlock()
		if len(classify.calls) != 0 {
			t.Errorf("expected no classification calls for turn_start, got %d", len(classify.calls))
		}
	})

	t.Run("should trigger classification for session_end events", func(t *testing.T) {
		repo := &mockSessionEventRepo{
			createdEvent: &clireview.SessionEvent{UUID: "end-1"},
		}
		classify := newMockClassifyUseCase()
		useCase := clireview.NewIngestSessionEventUseCase(repo, classify)

		doneCh := make(chan struct{}, 1)
		useCase.SetOnClassifyDone(doneCh)

		input := baseParams
		input.SessionID = "sess-1"
		input.Type = "session_end"
		input.Branch = "main"
		input.Timestamp = time.Now()

		_, err := useCase.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		select {
		case <-doneCh:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("timed out waiting for background classification")
		}

		classify.mu.Lock()
		defer classify.mu.Unlock()
		if len(classify.calls) != 1 || classify.calls[0] != "end-1" {
			t.Errorf("expected classification call for end-1, got %+v", classify.calls)
		}
	})

	t.Run("should not throw if classification fails asynchronously", func(t *testing.T) {
		repo := &mockSessionEventRepo{
			createdEvent: &clireview.SessionEvent{UUID: "end-1"},
		}
		classify := newMockClassifyUseCase()
		classify.err = errors.New("classify boom")
		useCase := clireview.NewIngestSessionEventUseCase(repo, classify)

		doneCh := make(chan struct{}, 1)
		useCase.SetOnClassifyDone(doneCh)

		input := baseParams
		input.SessionID = "sess-1"
		input.Type = "session_end"
		input.Branch = "main"
		input.Timestamp = time.Now()

		result, err := useCase.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result == nil || !result.Accepted {
			t.Errorf("expected accepted true, got %+v", result)
		}

		select {
		case <-doneCh:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("timed out waiting for background classification failure")
		}
	})

	t.Run("should check for prior turn_start when ingesting turn_end", func(t *testing.T) {
		repo := &mockSessionEventRepo{
			createdEvent: &clireview.SessionEvent{UUID: "evt-1"},
			priorEvents:  []*clireview.SessionEvent{},
		}
		classify := newMockClassifyUseCase()
		useCase := clireview.NewIngestSessionEventUseCase(repo, classify)

		input := baseParams
		input.SessionID = "sess-1"
		input.Type = "turn_end"
		input.Branch = "main"
		input.Timestamp = time.Now()

		_, err := useCase.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		repo.mu.Lock()
		defer repo.mu.Unlock()
		if len(repo.findBySess) != 1 || repo.findBySess[0] != "sess-1" {
			t.Errorf("expected FindBySessionID called with sess-1, got %+v", repo.findBySess)
		}
	})

	t.Run("should not query prior events for non turn_end events", func(t *testing.T) {
		repo := &mockSessionEventRepo{
			createdEvent: &clireview.SessionEvent{UUID: "evt-1"},
		}
		classify := newMockClassifyUseCase()
		useCase := clireview.NewIngestSessionEventUseCase(repo, classify)

		input := baseParams
		input.SessionID = "sess-1"
		input.Type = "turn_start"
		input.Branch = "main"
		input.Timestamp = time.Now()

		_, err := useCase.Execute(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		repo.mu.Lock()
		defer repo.mu.Unlock()
		if len(repo.findBySess) != 0 {
			t.Errorf("expected FindBySessionID not called for turn_start, got %+v", repo.findBySess)
		}
	})
}
