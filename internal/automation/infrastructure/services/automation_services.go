package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/automation/domain"
)

// DefaultAutomationService coordinates automation definitions.
type DefaultAutomationService struct {
	repo domain.AutomationRepository
}

// NewDefaultAutomationService creates an initialized automation service.
func NewDefaultAutomationService(repo domain.AutomationRepository) *DefaultAutomationService {
	return &DefaultAutomationService{repo: repo}
}

func (s *DefaultAutomationService) FindOne(ctx context.Context, filter map[string]any) (*domain.AutomationEntity, error) {
	return s.repo.FindOne(ctx, filter)
}

func (s *DefaultAutomationService) Find(ctx context.Context, filter map[string]any) ([]*domain.AutomationEntity, error) {
	return s.repo.Find(ctx, filter)
}

func (s *DefaultAutomationService) FindByID(ctx context.Context, id string) (*domain.AutomationEntity, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *DefaultAutomationService) Create(ctx context.Context, automation *domain.AutomationEntity) (*domain.AutomationEntity, error) {
	return s.repo.Create(ctx, automation)
}

func (s *DefaultAutomationService) Update(ctx context.Context, filter map[string]any, data map[string]any) (*domain.AutomationEntity, error) {
	return s.repo.Update(ctx, filter, data)
}

func (s *DefaultAutomationService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// DefaultTeamAutomationService coordinates team automation settings.
type DefaultTeamAutomationService struct {
	repo domain.TeamAutomationRepository
}

// NewDefaultTeamAutomationService creates an initialized team automation service.
func NewDefaultTeamAutomationService(repo domain.TeamAutomationRepository) *DefaultTeamAutomationService {
	return &DefaultTeamAutomationService{repo: repo}
}

func (s *DefaultTeamAutomationService) Create(ctx context.Context, teamAuto *domain.TeamAutomationEntity) (*domain.TeamAutomationEntity, error) {
	return s.repo.Create(ctx, teamAuto)
}

func (s *DefaultTeamAutomationService) Update(ctx context.Context, filter map[string]any, data map[string]any) (*domain.TeamAutomationEntity, error) {
	return s.repo.Update(ctx, filter, data)
}

func (s *DefaultTeamAutomationService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *DefaultTeamAutomationService) FindByID(ctx context.Context, id string) (*domain.TeamAutomationEntity, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *DefaultTeamAutomationService) Find(ctx context.Context, filter map[string]any) ([]*domain.TeamAutomationEntity, error) {
	return s.repo.Find(ctx, filter)
}

func (s *DefaultTeamAutomationService) Register(ctx context.Context, teamAuto *domain.TeamAutomationEntity) (*domain.TeamAutomationEntity, error) {
	existing, err := s.repo.Find(ctx, map[string]any{
		"teamId":       teamAuto.TeamID,
		"automationId": teamAuto.AutomationID,
	})
	if err == nil && len(existing) > 0 {
		return s.repo.Update(ctx, map[string]any{"uuid": existing[0].UUID}, map[string]any{
			"status": teamAuto.Status,
		})
	}
	return s.repo.Create(ctx, teamAuto)
}

// DefaultCodeReviewExecutionService coordinates stage tracking for code reviews.
type DefaultCodeReviewExecutionService struct {
	repo domain.CodeReviewExecutionRepository
}

// NewDefaultCodeReviewExecutionService creates an initialized stage service.
func NewDefaultCodeReviewExecutionService(repo domain.CodeReviewExecutionRepository) *DefaultCodeReviewExecutionService {
	return &DefaultCodeReviewExecutionService{repo: repo}
}

func (s *DefaultCodeReviewExecutionService) Create(ctx context.Context, exec *domain.CodeReviewExecutionEntity) (*domain.CodeReviewExecutionEntity, error) {
	return s.repo.Create(ctx, exec)
}

func (s *DefaultCodeReviewExecutionService) Update(ctx context.Context, filter map[string]any, data map[string]any) (*domain.CodeReviewExecutionEntity, error) {
	return s.repo.Update(ctx, filter, data)
}

func (s *DefaultCodeReviewExecutionService) Find(ctx context.Context, filter map[string]any) ([]*domain.CodeReviewExecutionEntity, error) {
	return s.repo.Find(ctx, filter)
}

func (s *DefaultCodeReviewExecutionService) FindOne(ctx context.Context, filter map[string]any) (*domain.CodeReviewExecutionEntity, error) {
	return s.repo.FindOne(ctx, filter)
}

func (s *DefaultCodeReviewExecutionService) FindManyByAutomationExecutionIDs(ctx context.Context, executionIDs []string) ([]*domain.CodeReviewExecutionEntity, error) {
	return s.repo.FindManyByAutomationExecutionIDs(ctx, executionIDs)
}

func (s *DefaultCodeReviewExecutionService) ExistsByAutomationExecutionAndStageStatus(ctx context.Context, executionID string, stageName string, status domain.AutomationStatus) (bool, error) {
	return s.repo.ExistsByAutomationExecutionAndStageStatus(ctx, executionID, stageName, status)
}

func (s *DefaultCodeReviewExecutionService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// DefaultAutomationExecutionService handles execution lifecycle and composite stage transitions.
type DefaultAutomationExecutionService struct {
	domain.AutomationExecutionRepository
	stageRepo domain.CodeReviewExecutionRepository
}

// NewDefaultAutomationExecutionService creates an execution service with composite stage operations.
func NewDefaultAutomationExecutionService(
	execRepo domain.AutomationExecutionRepository,
	stageRepo domain.CodeReviewExecutionRepository,
) *DefaultAutomationExecutionService {
	return &DefaultAutomationExecutionService{
		AutomationExecutionRepository: execRepo,
		stageRepo:                     stageRepo,
	}
}

func (s *DefaultAutomationExecutionService) CreateCodeReview(
	ctx context.Context,
	exec *domain.AutomationExecutionEntity,
	message, stageName string,
) (*domain.CreateCodeReviewResult, error) {
	createdExec, err := s.AutomationExecutionRepository.Create(ctx, exec)
	if err != nil {
		return nil, err
	}

	if stageName == "" {
		stageName = "Drixy Review Started"
	}

	stageLog, err := s.stageRepo.Create(ctx, &domain.CodeReviewExecutionEntity{
		UUID:                  uuid.New().String(),
		AutomationExecutionID: createdExec.UUID,
		Status:                domain.StatusInProgress,
		StageName:             stageName,
		Message:               message,
		CreatedAt:             time.Now().UTC(),
	})
	if err != nil {
		return nil, err
	}

	return &domain.CreateCodeReviewResult{
		Execution: createdExec,
		StageLog:  stageLog,
	}, nil
}

func (s *DefaultAutomationExecutionService) UpdateCodeReview(
	ctx context.Context,
	filter map[string]any,
	data map[string]any,
	message, stageName string,
) (*domain.AutomationExecutionEntity, error) {
	updatedExec, err := s.AutomationExecutionRepository.Update(ctx, filter, data)
	if err != nil {
		return nil, err
	}

	if stageName != "" && updatedExec != nil {
		now := time.Now().UTC()
		var stageStatus domain.AutomationStatus
		if st, ok := data["status"].(domain.AutomationStatus); ok {
			stageStatus = st
		} else {
			stageStatus = updatedExec.Status
		}

		_, _ = s.stageRepo.Create(ctx, &domain.CodeReviewExecutionEntity{
			UUID:                  uuid.New().String(),
			AutomationExecutionID: updatedExec.UUID,
			Status:                stageStatus,
			StageName:             stageName,
			Message:               message,
			FinishedAt:            &now,
			CreatedAt:             now,
		})
	}

	return updatedExec, nil
}

func (s *DefaultAutomationExecutionService) UpdateStageLog(
	ctx context.Context,
	stageLogUUID string,
	data map[string]any,
) error {
	_, err := s.stageRepo.Update(ctx, map[string]any{"uuid": stageLogUUID}, data)
	return err
}
