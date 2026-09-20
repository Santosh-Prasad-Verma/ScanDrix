package repositories

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/automation/domain"
)

// InMemoryCodeReviewExecutionRepository tracks individual code review pipeline stages.
type InMemoryCodeReviewExecutionRepository struct {
	mu     sync.RWMutex
	stages map[string]*domain.CodeReviewExecutionEntity
}

// NewInMemoryCodeReviewExecutionRepository creates an initialized stage repository.
func NewInMemoryCodeReviewExecutionRepository() *InMemoryCodeReviewExecutionRepository {
	return &InMemoryCodeReviewExecutionRepository{
		stages: make(map[string]*domain.CodeReviewExecutionEntity),
	}
}

func (r *InMemoryCodeReviewExecutionRepository) Create(ctx context.Context, exec *domain.CodeReviewExecutionEntity) (*domain.CodeReviewExecutionEntity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if exec.UUID == "" {
		exec.UUID = uuid.New().String()
	}
	now := time.Now().UTC()
	if exec.CreatedAt.IsZero() {
		exec.CreatedAt = now
	}
	exec.UpdatedAt = now

	copy := *exec
	r.stages[exec.UUID] = &copy
	return &copy, nil
}

func (r *InMemoryCodeReviewExecutionRepository) Update(ctx context.Context, filter map[string]any, data map[string]any) (*domain.CodeReviewExecutionEntity, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, stage := range r.stages {
		if matchesStageFilter(stage, filter) {
			if s, ok := data["status"].(domain.AutomationStatus); ok {
				stage.Status = s
			}
			if msg, ok := data["message"].(string); ok {
				stage.Message = msg
			}
			if sn, ok := data["stageName"].(string); ok {
				stage.StageName = sn
			}
			if meta, ok := data["metadata"].(map[string]any); ok {
				stage.Metadata = meta
			}
			if fa, ok := data["finishedAt"].(*time.Time); ok {
				stage.FinishedAt = fa
			}
			stage.UpdatedAt = time.Now().UTC()
			copy := *stage
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("stage log not found for update")
}

func (r *InMemoryCodeReviewExecutionRepository) Find(ctx context.Context, filter map[string]any) ([]*domain.CodeReviewExecutionEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*domain.CodeReviewExecutionEntity
	for _, stage := range r.stages {
		if filter == nil || matchesStageFilter(stage, filter) {
			copy := *stage
			results = append(results, &copy)
		}
	}
	return results, nil
}

func (r *InMemoryCodeReviewExecutionRepository) FindOne(ctx context.Context, filter map[string]any) (*domain.CodeReviewExecutionEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, stage := range r.stages {
		if matchesStageFilter(stage, filter) {
			copy := *stage
			return &copy, nil
		}
	}
	return nil, nil
}

func (r *InMemoryCodeReviewExecutionRepository) FindManyByAutomationExecutionIDs(ctx context.Context, executionIDs []string) ([]*domain.CodeReviewExecutionEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	idMap := make(map[string]bool, len(executionIDs))
	for _, id := range executionIDs {
		idMap[id] = true
	}

	var results []*domain.CodeReviewExecutionEntity
	for _, stage := range r.stages {
		if idMap[stage.AutomationExecutionID] {
			copy := *stage
			results = append(results, &copy)
		}
	}
	return results, nil
}


func (r *InMemoryCodeReviewExecutionRepository) ExistsByAutomationExecutionAndStageStatus(ctx context.Context, executionID string, stageName string, status domain.AutomationStatus) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, stage := range r.stages {
		if stage.AutomationExecutionID == executionID &&
			stage.StageName == stageName &&
			stage.Status == status {
			return true, nil
		}
	}
	return false, nil
}

func (r *InMemoryCodeReviewExecutionRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.stages, id)
	return nil
}

func matchesStageFilter(s *domain.CodeReviewExecutionEntity, filter map[string]any) bool {
	if filter == nil {
		return true
	}
	if u, ok := filter["uuid"].(string); ok && u != "" && s.UUID != u {
		return false
	}
	if execID, ok := filter["automationExecutionId"].(string); ok && execID != "" && s.AutomationExecutionID != execID {
		return false
	}
	if st, ok := filter["status"].(domain.AutomationStatus); ok && s.Status != st {
		return false
	}
	if sn, ok := filter["stageName"].(string); ok && sn != "" && s.StageName != sn {
		return false
	}
	return true
}
