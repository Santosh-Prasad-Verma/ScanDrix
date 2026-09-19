package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/scandrix/backend/internal/clireview/domain"
)

// IngestSessionEventInput parameters for ingestion.
type IngestSessionEventInput struct {
	OrganizationAndTeamData domain.OrganizationAndTeamData `json:"organizationAndTeamData"`
	SessionID               string                         `json:"sessionId"`
	Type                    string                         `json:"type"` // "session_start", "turn_start", "turn_end", "subagent_start", "session_end"
	Branch                  string                         `json:"branch"`
	Timestamp               time.Time                      `json:"timestamp"`
	Payload                 map[string]any                 `json:"payload,omitempty"`
}

// IngestSessionEventResult conveys acceptance.
type IngestSessionEventResult struct {
	Accepted bool `json:"accepted"`
}

// IClassifySessionUseCase represents the session classification usecase.
type IClassifySessionUseCase interface {
	Execute(ctx context.Context, sessionEndEventUUID string) error
}

// IngestSessionEventUseCase stores session telemetry and triggers classification on session end.
type IngestSessionEventUseCase struct {
	eventRepo       domain.ISessionEventRepository
	classifyUseCase IClassifySessionUseCase
	logger          *slog.Logger
	onClassifyDone  chan struct{}
}

// NewIngestSessionEventUseCase creates an initialized ingestion usecase.
func NewIngestSessionEventUseCase(
	eventRepo domain.ISessionEventRepository,
	classifyUseCase IClassifySessionUseCase,
) *IngestSessionEventUseCase {
	return &IngestSessionEventUseCase{
		eventRepo:       eventRepo,
		classifyUseCase: classifyUseCase,
		logger:          slog.Default().With("usecase", "IngestSessionEventUseCase"),
	}
}

// SetOnClassifyDone configures an optional notification channel for background classification completion (used in tests).
func (uc *IngestSessionEventUseCase) SetOnClassifyDone(ch chan struct{}) {
	uc.onClassifyDone = ch
}

// Execute ingests single event and schedules background classification if session ended.
func (uc *IngestSessionEventUseCase) Execute(ctx context.Context, input IngestSessionEventInput) (*IngestSessionEventResult, error) {
	if uc.eventRepo == nil {
		return nil, fmt.Errorf("session event repository unavailable")
	}

	payload := input.Payload
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["branch"] = input.Branch

	event := &domain.SessionEvent{
		OrganizationID: input.OrganizationAndTeamData.OrganizationID,
		TeamID:         input.OrganizationAndTeamData.TeamID,
		SessionID:      input.SessionID,
		EventType:      input.Type,
		Payload:        payload,
		CreatedAt:      input.Timestamp,
	}

	saved, err := uc.eventRepo.Create(ctx, event)
	if err != nil {
		uc.logger.Error("Failed to ingest session event", "error", err, "sessionId", input.SessionID, "type", input.Type)
		return nil, err
	}

	// Validate turn sequence
	if input.Type == "turn_end" {
		prior, err := uc.eventRepo.FindBySessionID(ctx, input.SessionID, input.OrganizationAndTeamData.OrganizationID)
		if err == nil {
			hasTurnStart := false
			for _, e := range prior {
				if e.EventType == "turn_start" {
					hasTurnStart = true
					break
				}
			}
			if !hasTurnStart {
				uc.logger.Warn("turn_end received without prior turn_start",
					"sessionId", input.SessionID,
					"orgId", input.OrganizationAndTeamData.OrganizationID,
				)
			}
		}
	}

	// Trigger classification on session_end
	if input.Type == "session_end" && uc.classifyUseCase != nil {
		eventUUID := saved.UUID
		if eventUUID == "" {
			eventUUID = saved.ID
		}
		go func(uuid string) {
			bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if err := uc.classifyUseCase.Execute(bgCtx, uuid); err != nil {
				uc.logger.Error("Failed to classify session after session_end",
					"error", err,
					"sessionEndEventUuid", uuid,
				)
			}
			if uc.onClassifyDone != nil {
				select {
				case uc.onClassifyDone <- struct{}{}:
				default:
				}
			}
		}(eventUUID)
	}

	return &IngestSessionEventResult{Accepted: true}, nil
}
