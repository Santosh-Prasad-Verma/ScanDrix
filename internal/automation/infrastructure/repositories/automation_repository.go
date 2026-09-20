package repositories

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/automation/domain"
)

// InMemoryAutomationRepository provides a thread-safe repository for automations.
type InMemoryAutomationRepository struct {
	mu          sync.RWMutex
	automations map[string]*domain.AutomationEntity
}

// NewInMemoryAutomationRepository creates an initialized automation repository.
func NewInMemoryAutomationRepository() *InMemoryAutomationRepository {
	repo := &InMemoryAutomationRepository{
		automations: make(map[string]*domain.AutomationEntity),
	}
	repo.seedDefaults()
	return repo
}

func (r *InMemoryAutomationRepository) seedDefaults() {
	defaultAutomations := []*domain.AutomationEntity{
		{
			UUID:           uuid.New().String(),
			Name:           "Code Review Automation",
			Description:    "Automated code reviews, security scans, and recommendations on pull requests",
			Tags:           []string{"code-review", "quality", "security"},
			AntiPatterns:   []string{"untested_code", "security_vulnerability"},
			AutomationType: domain.AutomationCodeReview,
			Status:         true,
			Level:          domain.LevelTeam,
		},
		{
			UUID:           uuid.New().String(),
			Name:           "Daily Check-in Automation",
			Description:    "Automated morning standup and progress summaries",
			Tags:           []string{"standup", "progress"},
			AntiPatterns:   []string{"stale_tasks"},
			AutomationType: domain.AutomationDailyCheckin,
			Status:         true,
			Level:          domain.LevelTeam,
		},
		{
			UUID:           uuid.New().String(),
			Name:           "Team Progress Automation",
			Description:    "Sprint health and goal tracking alerts",
			Tags:           []string{"sprint", "velocity"},
			AntiPatterns:   []string{"scope_creep"},
			AutomationType: domain.AutomationTeamProgress,
			Status:         true,
			Level:          domain.LevelTeam,
		},
	}

	for _, a := range defaultAutomations {
		r.automations[a.UUID] = a
	}
}

func (r *InMemoryAutomationRepository) FindOne(ctx context.Context, filter map[string]any) (*domain.AutomationEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, a := range r.automations {
		if matchesFilter(a, filter) {
			copy := *a
			return &copy, nil
		}
	}
	return nil, nil
}

func (r *InMemoryAutomationRepository) Find(ctx context.Context, filter map[string]any) ([]*domain.AutomationEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*domain.AutomationEntity
	for _, a := range r.automations {
		if filter == nil || matchesFilter(a, filter) {
			copy := *a
			results = append(results, &copy)
		}
	}
	return results, nil
}

func (r *InMemoryAutomationRepository) FindByID(ctx context.Context, id string) (*domain.AutomationEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	a, exists := r.automations[id]
	if !exists {
		return nil, nil
	}
	copy := *a
	return &copy, nil
}

func (r *InMemoryAutomationRepository) Create(ctx context.Context, automation *domain.AutomationEntity) (*domain.AutomationEntity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if automation.UUID == "" {
		automation.UUID = uuid.New().String()
	}

	copy := *automation
	r.automations[automation.UUID] = &copy
	return &copy, nil
}

func (r *InMemoryAutomationRepository) Update(ctx context.Context, filter map[string]any, data map[string]any) (*domain.AutomationEntity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, a := range r.automations {
		if matchesFilter(a, filter) {
			applyAutomationUpdate(a, data)
			copy := *a
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("automation not found for update")
}

func (r *InMemoryAutomationRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.automations, id)
	return nil
}

func matchesFilter(a *domain.AutomationEntity, filter map[string]any) bool {
	if filter == nil {
		return true
	}
	if u, ok := filter["uuid"].(string); ok && u != "" && a.UUID != u {
		return false
	}
	if t, ok := filter["automationType"].(domain.AutomationType); ok && a.AutomationType != t {
		return false
	}
	if t, ok := filter["automation_type"].(domain.AutomationType); ok && a.AutomationType != t {
		return false
	}
	if s, ok := filter["status"].(bool); ok && a.Status != s {
		return false
	}
	if l, ok := filter["level"].(domain.AutomationLevel); ok && a.Level != l {
		return false
	}
	return true
}

func applyAutomationUpdate(a *domain.AutomationEntity, data map[string]any) {
	if name, ok := data["name"].(string); ok {
		a.Name = name
	}
	if desc, ok := data["description"].(string); ok {
		a.Description = desc
	}
	if status, ok := data["status"].(bool); ok {
		a.Status = status
	}
	if level, ok := data["level"].(domain.AutomationLevel); ok {
		a.Level = level
	}
	if tags, ok := data["tags"].([]string); ok {
		a.Tags = tags
	}
	if anti, ok := data["antiPatterns"].([]string); ok {
		a.AntiPatterns = anti
	}
}
