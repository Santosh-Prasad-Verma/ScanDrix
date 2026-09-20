package repositories

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/automation/domain"
)

// InMemoryTeamAutomationRepository stores team-scoped automation settings.
type InMemoryTeamAutomationRepository struct {
	mu        sync.RWMutex
	teamAutos map[string]*domain.TeamAutomationEntity
}

// NewInMemoryTeamAutomationRepository creates an initialized team automation repository.
func NewInMemoryTeamAutomationRepository() *InMemoryTeamAutomationRepository {
	return &InMemoryTeamAutomationRepository{
		teamAutos: make(map[string]*domain.TeamAutomationEntity),
	}
}

func (r *InMemoryTeamAutomationRepository) Create(ctx context.Context, teamAuto *domain.TeamAutomationEntity) (*domain.TeamAutomationEntity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if teamAuto.UUID == "" {
		teamAuto.UUID = uuid.New().String()
	}
	now := time.Now().UTC()
	if teamAuto.CreatedAt.IsZero() {
		teamAuto.CreatedAt = now
	}
	teamAuto.UpdatedAt = now

	copy := *teamAuto
	r.teamAutos[teamAuto.UUID] = &copy
	return &copy, nil
}

func (r *InMemoryTeamAutomationRepository) Update(ctx context.Context, filter map[string]any, data map[string]any) (*domain.TeamAutomationEntity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, ta := range r.teamAutos {
		if matchesTeamAutoFilter(ta, filter) {
			if s, ok := data["status"].(bool); ok {
				ta.Status = s
			}
			if teamID, ok := data["teamId"].(string); ok {
				ta.TeamID = teamID
			}
			if autoID, ok := data["automationId"].(string); ok {
				ta.AutomationID = autoID
			}
			ta.UpdatedAt = time.Now().UTC()
			copy := *ta
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("team automation not found for update")
}

func (r *InMemoryTeamAutomationRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.teamAutos, id)
	return nil
}

func (r *InMemoryTeamAutomationRepository) FindByID(ctx context.Context, id string) (*domain.TeamAutomationEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ta, exists := r.teamAutos[id]
	if !exists {
		return nil, nil
	}
	copy := *ta
	return &copy, nil
}

func (r *InMemoryTeamAutomationRepository) Find(ctx context.Context, filter map[string]any) ([]*domain.TeamAutomationEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*domain.TeamAutomationEntity
	for _, ta := range r.teamAutos {
		if filter == nil || matchesTeamAutoFilter(ta, filter) {
			copy := *ta
			results = append(results, &copy)
		}
	}
	return results, nil
}

func matchesTeamAutoFilter(ta *domain.TeamAutomationEntity, filter map[string]any) bool {
	if filter == nil {
		return true
	}
	if u, ok := filter["uuid"].(string); ok && u != "" && ta.UUID != u {
		return false
	}
	if teamID, ok := filter["teamId"].(string); ok && teamID != "" && ta.TeamID != teamID {
		return false
	}
	if teamMap, ok := filter["team"].(map[string]any); ok {
		if teamUUID, ok := teamMap["uuid"].(string); ok && teamUUID != "" && ta.TeamID != teamUUID {
			return false
		}
	}
	if autoID, ok := filter["automationId"].(string); ok && autoID != "" && ta.AutomationID != autoID {
		return false
	}
	if autoMap, ok := filter["automation"].(map[string]any); ok {
		if autoUUID, ok := autoMap["uuid"].(string); ok && autoUUID != "" && ta.AutomationID != autoUUID {
			return false
		}
	}
	if s, ok := filter["status"].(bool); ok && ta.Status != s {
		return false
	}
	return true
}
