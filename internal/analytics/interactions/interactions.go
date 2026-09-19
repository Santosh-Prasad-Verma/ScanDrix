package interactions

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// InteractionExecution represents an recorded user interaction or button action with the AI engine.
type InteractionExecution struct {
	UUID               string    `json:"uuid"`
	InteractionDate    time.Time `json:"interactionDate"`
	PlatformUserID     string    `json:"platformUserId"`
	InteractionType    string    `json:"interactionType"`
	InteractionCommand string    `json:"interactionCommand,omitempty"`
	ButtonLabel        string    `json:"buttonLabel,omitempty"`
	TeamID             string    `json:"teamId,omitempty"`
	OrganizationID     string    `json:"organizationId,omitempty"`
}

// IInteractionRepository defines persistence operations for interaction executions.
type IInteractionRepository interface {
	Save(ctx context.Context, item InteractionExecution) error
	FindByOrg(ctx context.Context, orgID string, start, end time.Time) ([]InteractionExecution, error)
}

// InMemoryInteractionRepository provides thread-safe in-memory storage for interaction executions.
type InMemoryInteractionRepository struct {
	items []InteractionExecution
}

// NewInMemoryInteractionRepository creates a new in-memory repository.
func NewInMemoryInteractionRepository() *InMemoryInteractionRepository {
	return &InMemoryInteractionRepository{
		items: make([]InteractionExecution, 0),
	}
}

func (r *InMemoryInteractionRepository) Save(ctx context.Context, item InteractionExecution) error {
	if item.UUID == "" {
		item.UUID = uuid.NewString()
	}
	if item.InteractionDate.IsZero() {
		item.InteractionDate = time.Now().UTC()
	}
	r.items = append(r.items, item)
	return nil
}

func (r *InMemoryInteractionRepository) FindByOrg(ctx context.Context, orgID string, start, end time.Time) ([]InteractionExecution, error) {
	var result []InteractionExecution
	for _, item := range r.items {
		if item.OrganizationID == orgID {
			if (start.IsZero() || !item.InteractionDate.Before(start)) &&
				(end.IsZero() || !item.InteractionDate.After(end)) {
				result = append(result, item)
			}
		}
	}
	return result, nil
}

// InteractionService records and queries interaction executions.
type InteractionService struct {
	repo IInteractionRepository
}

// NewInteractionService creates a new interaction service.
func NewInteractionService(repo IInteractionRepository) *InteractionService {
	if repo == nil {
		repo = NewInMemoryInteractionRepository()
	}
	return &InteractionService{repo: repo}
}

// RecordInteraction stores a new interaction event.
func (s *InteractionService) RecordInteraction(ctx context.Context, item InteractionExecution) error {
	if item.UUID == "" {
		item.UUID = uuid.NewString()
	}
	if item.InteractionDate.IsZero() {
		item.InteractionDate = time.Now().UTC()
	}
	return s.repo.Save(ctx, item)
}

// GetInteractions retrieves interaction events within a time range for an organization.
func (s *InteractionService) GetInteractions(ctx context.Context, orgID string, start, end time.Time) ([]InteractionExecution, error) {
	return s.repo.FindByOrg(ctx, orgID, start, end)
}
