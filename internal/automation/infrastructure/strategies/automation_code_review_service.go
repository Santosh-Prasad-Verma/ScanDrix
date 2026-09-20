package strategies

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/automation/domain"
)

const (
	activeExecutionLookbackMinutes = 30
)

var staleStartupMessages = map[string]bool{
	"pipeline started":     true,
	"code review started":  true,
	"reviewing file level": true,
}

// PrReviewInProgressError represents refusal of a concurrent review request.
type PrReviewInProgressError struct {
	Gate               string
	HolderVisibleUntil time.Time
	Target             map[string]any
}

func (e *PrReviewInProgressError) Error() string {
	return fmt.Sprintf("code review already in progress (gate: %s, visible until: %s)", e.Gate, e.HolderVisibleUntil.Format(time.RFC3339))
}

// DistributedLock abstracts distributed synchronization.
type DistributedLock interface {
	Release(ctx context.Context) error
}

// DistributedLockService manages distributed mutexes.
type DistributedLockService interface {
	Acquire(ctx context.Context, key string, ttl time.Duration) (DistributedLock, error)
}

// SimpleLockService provides a memory-based distributed lock for testing or local runs.
type SimpleLockService struct {
	mu    sync.Mutex
	locks map[string]time.Time
}

// NewSimpleLockService creates a lock service.
func NewSimpleLockService() *SimpleLockService {
	return &SimpleLockService{
		locks: make(map[string]time.Time),
	}
}

type simpleLock struct {
	svc *SimpleLockService
	key string
}

func (l *simpleLock) Release(ctx context.Context) error {
	l.svc.mu.Lock()
	defer l.svc.mu.Unlock()
	delete(l.svc.locks, l.key)
	return nil
}

func (s *SimpleLockService) Acquire(ctx context.Context, key string, ttl time.Duration) (DistributedLock, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if exp, exists := s.locks[key]; exists && exp.After(now) {
		return nil, nil // Lock is busy
	}

	s.locks[key] = now.Add(ttl)
	return &simpleLock{svc: s, key: key}, nil
}

// ReviewPipelineHandler invokes the actual code review engine.
type ReviewPipelineHandler interface {
	HandlePullRequest(ctx context.Context, payload map[string]any) (map[string]any, error)
}

// AutomationCodeReviewService orchestrates code review automations.
type AutomationCodeReviewService struct {
	logger                    *slog.Logger
	teamAutoService           domain.TeamAutomationService
	automationService         domain.AutomationService
	automationExecService     domain.AutomationExecutionService
	lockService               DistributedLockService
	reviewHandler             ReviewPipelineHandler
	automationType            domain.AutomationType
}

// NewAutomationCodeReviewService initializes the code review automation service.
func NewAutomationCodeReviewService(
	logger *slog.Logger,
	teamAutoService domain.TeamAutomationService,
	automationService domain.AutomationService,
	automationExecService domain.AutomationExecutionService,
	lockService DistributedLockService,
	reviewHandler ReviewPipelineHandler,
) *AutomationCodeReviewService {
	if logger == nil {
		logger = slog.Default()
	}
	if lockService == nil {
		lockService = NewSimpleLockService()
	}
	return &AutomationCodeReviewService{
		logger:                logger,
		teamAutoService:       teamAutoService,
		automationService:     automationService,
		automationExecService: automationExecService,
		lockService:           lockService,
		reviewHandler:         reviewHandler,
		automationType:        domain.AutomationCodeReview,
	}
}

// GetAutomationType returns the automation type handled by this service.
func (s *AutomationCodeReviewService) GetAutomationType() domain.AutomationType {
	return s.automationType
}

// Setup registers the team automation binding for code review.
func (s *AutomationCodeReviewService) Setup(ctx context.Context, payload any) error {
	payloadMap, ok := payload.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid payload for setup")
	}

	teamID, _ := payloadMap["teamId"].(string)
	if teamID == "" {
		return fmt.Errorf("teamId is required for setup")
	}

	automations, err := s.automationService.Find(ctx, map[string]any{
		"automationType": s.automationType,
	})
	if err != nil || len(automations) == 0 {
		return fmt.Errorf("code review automation definition not found: %w", err)
	}

	_, err = s.teamAutoService.Register(ctx, &domain.TeamAutomationEntity{
		Status:       false,
		AutomationID: automations[0].UUID,
		TeamID:       teamID,
	})
	if err != nil {
		s.logger.Error("error creating team automation", "error", err, "teamId", teamID)
		return err
	}
	return nil
}

// Run executes the review workflow for an incoming event.
func (s *AutomationCodeReviewService) Run(ctx context.Context, payload any) (any, error) {
	pMap, ok := payload.(map[string]any)
	if !ok {
		return nil, errors.New("invalid payload for automation run")
	}

	orgData, _ := pMap["organizationAndTeamData"].(map[string]any)
	repo, _ := pMap["repository"].(map[string]any)
	pr, _ := pMap["pullRequest"].(map[string]any)

	orgID, _ := orgData["organizationId"].(string)
	repoID, _ := repo["id"].(string)
	prNumber := 0
	if n, ok := pr["number"].(int); ok {
		prNumber = n
	} else if nf, ok := pr["number"].(float64); ok {
		prNumber = int(nf)
	}

	if orgID == "" || repoID == "" || prNumber <= 0 {
		s.logger.Error("missing required identifiers for review automation",
			"orgId", orgID, "repoId", repoID, "prNumber", prNumber)
		return nil, errors.New("missing required identifiers for code review")
	}

	teamAutoID, _ := pMap["teamAutomationId"].(string)
	origin, _ := pMap["origin"].(string)

	lockKey := fmt.Sprintf("CODE_REVIEW:%s:%s:%d", orgID, repoID, prNumber)
	var lock DistributedLock
	var lockHeldByAnother bool

	acquiredLock, err := s.lockService.Acquire(ctx, lockKey, 1*time.Minute)
	if err != nil {
		s.logger.Error("error acquiring lock, proceeding fail-open", "error", err, "lockKey", lockKey)
	} else if acquiredLock == nil {
		lockHeldByAnother = true
	} else {
		lock = acquiredLock
	}

	if lockHeldByAnother {
		s.logger.Warn("code review already in progress for this PR, skipping", "lockKey", lockKey)
		if isCommandOrigin(origin) {
			holder, _ := s.getActiveExecution(ctx, teamAutoID, prNumber, repoID)
			visibleUntil := holderVisibleUntil(holder)
			return nil, &PrReviewInProgressError{
				Gate:               "lock",
				HolderVisibleUntil: visibleUntil,
				Target:             pMap,
			}
		}
		return "Code review already in progress for this PR", nil
	}

	if lock != nil {
		defer func() {
			_ = lock.Release(ctx)
		}()
	}

	existingExec, _ := s.getActiveExecution(ctx, teamAutoID, prNumber, repoID)
	if existingExec != nil {
		s.logger.Warn("code review execution already in progress for PR", "prNumber", prNumber)
		if isCommandOrigin(origin) {
			visibleUntil := holderVisibleUntil(existingExec)
			return nil, &PrReviewInProgressError{
				Gate:               "execution",
				HolderVisibleUntil: visibleUntil,
				Target:             pMap,
			}
		}
		return "Code review already in progress for this PR", nil
	}

	res, err := s.automationExecService.CreateCodeReview(ctx, &domain.AutomationExecutionEntity{
		Status: domain.StatusInProgress,
		DataExecution: map[string]any{
			"platformType":            pMap["platformType"],
			"organizationAndTeamData": orgData,
			"pullRequestNumber":       prNumber,
			"repositoryId":            repoID,
			"workflowJobId":           pMap["workflowJobId"],
			"correlationId":           pMap["correlationId"],
		},
		TeamAutomation:    &domain.TeamAutomationEntity{UUID: teamAutoID},
		Origin:            origin,
		PullRequestNumber: prNumber,
		RepositoryID:      repoID,
	}, "", "Drixy Review Started")

	if err != nil || res == nil || res.Execution == nil {
		s.logger.Error("could not create code review execution", "error", err)
		return nil, errors.New("could not create code review execution")
	}
	exec := res.Execution

	if valErr, ok := pMap["validationError"].(map[string]any); ok && valErr != nil {
		errType, _ := valErr["errorType"].(string)
		s.logger.Warn("automation blocked by validation error", "errorType", errType)
		_, _ = s.automationExecService.UpdateCodeReview(ctx, map[string]any{"uuid": exec.UUID}, map[string]any{
			"status":        domain.StatusError,
			"errorMessage":  fmt.Sprintf("Blocked by validation: %s", errType),
			"dataExecution": s.buildExecutionData(pMap, nil),
		}, fmt.Sprintf("Blocked by validation: %s", errType), "Drixy Review Finished")
		return fmt.Sprintf("Automation blocked: %s", errType), nil
	}

	if s.reviewHandler != nil {
		result, runErr := s.reviewHandler.HandlePullRequest(ctx, pMap)
		if runErr != nil {
			s.logger.Error("error during review pipeline execution", "error", runErr)
			_, _ = s.automationExecService.UpdateCodeReview(ctx, map[string]any{"uuid": exec.UUID}, map[string]any{
				"status":        domain.StatusError,
				"errorMessage":  runErr.Error(),
				"dataExecution": s.buildExecutionData(pMap, nil),
			}, runErr.Error(), "Drixy Review Finished")
			return nil, runErr
		}

		finalStatus := s.deriveFinalStatus(result)
		finalMsg := s.buildFinalMessage(result, finalStatus)
		newData := s.buildExecutionData(pMap, result)

		_, _ = s.automationExecService.UpdateCodeReview(ctx, map[string]any{"uuid": exec.UUID}, map[string]any{
			"status":        finalStatus,
			"errorMessage":  finalMsg,
			"dataExecution": newData,
		}, finalMsg, "Drixy Review Finished")

		return "Automation executed successfully", nil
	}

	// Default success path if reviewHandler was not injected
	_, _ = s.automationExecService.UpdateCodeReview(ctx, map[string]any{"uuid": exec.UUID}, map[string]any{
		"status":        domain.StatusSuccess,
		"dataExecution": s.buildExecutionData(pMap, nil),
	}, "Automation completed successfully.", "Drixy Review Finished")

	return "Automation executed successfully", nil
}

// Stop terminates an active review execution.
func (s *AutomationCodeReviewService) Stop(ctx context.Context, payload any) error {
	return nil
}

func (s *AutomationCodeReviewService) getActiveExecution(
	ctx context.Context,
	teamAutoID string,
	prNumber int,
	repoID string,
) (*domain.AutomationExecutionEntity, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(activeExecutionLookbackMinutes) * time.Minute)
	execs, err := s.automationExecService.Find(ctx, map[string]any{
		"teamAutomationId":  teamAutoID,
		"pullRequestNumber": prNumber,
		"repositoryId":      repoID,
		"status":            domain.StatusInProgress,
		"createdAtGte":      cutoff,
	})
	if err != nil || len(execs) == 0 {
		return nil, err
	}
	return execs[0], nil
}

func (s *AutomationCodeReviewService) deriveFinalStatus(result map[string]any) domain.AutomationStatus {
	if result == nil {
		return domain.StatusError
	}
	if statusInfo, ok := result["statusInfo"].(map[string]any); ok {
		if st, ok := statusInfo["status"].(string); ok && st == string(domain.StatusSkipped) {
			return domain.StatusSkipped
		}
	}

	if errorsList, ok := result["errors"].([]any); ok {
		hasCritical := false
		hasPartial := false
		for _, errItem := range errorsList {
			if eMap, ok := errItem.(map[string]any); ok {
				sev, _ := eMap["severity"].(string)
				if sev == "critical" || sev == "" {
					hasCritical = true
				} else if sev == "partial" {
					hasPartial = true
				}
			}
		}
		if hasCritical {
			return domain.StatusError
		}
		if hasPartial {
			return domain.StatusPartialError
		}
	}

	return domain.StatusSuccess
}

func (s *AutomationCodeReviewService) buildFinalMessage(result map[string]any, finalStatus domain.AutomationStatus) string {
	if finalStatus == domain.StatusError || finalStatus == domain.StatusPartialError {
		if statusInfo, ok := result["statusInfo"].(map[string]any); ok {
			if msg, ok := statusInfo["message"].(string); ok && strings.TrimSpace(msg) != "" {
				if !staleStartupMessages[strings.ToLower(strings.TrimSpace(msg))] {
					return msg
				}
			}
		}
		if finalStatus == domain.StatusPartialError {
			return "Code review completed with issues."
		}
		return "Code review failed."
	}

	if statusInfo, ok := result["statusInfo"].(map[string]any); ok {
		if msg, ok := statusInfo["message"].(string); ok && strings.TrimSpace(msg) != "" {
			return msg
		}
	}
	return "Automation completed successfully."
}

func (s *AutomationCodeReviewService) buildExecutionData(payload map[string]any, result map[string]any) map[string]any {
	orgData, _ := payload["organizationAndTeamData"].(map[string]any)
	repo, _ := payload["repository"].(map[string]any)
	pr, _ := payload["pullRequest"].(map[string]any)

	data := map[string]any{
		"codeManagementEvent":     payload["codeManagementEvent"],
		"platformType":            payload["platformType"],
		"organizationAndTeamData": orgData,
		"pullRequestNumber":       pr["number"],
		"repositoryId":            repo["id"],
	}

	if result != nil {
		if commit, ok := result["lastAnalyzedCommit"].(map[string]any); ok && len(commit) > 0 {
			data["lastAnalyzedCommit"] = commit
			data["commentId"] = result["commentId"]
			data["noteId"] = result["noteId"]
			data["threadId"] = result["threadId"]
			data["automaticReviewStatus"] = result["automaticReviewStatus"]
		}
		if orphaned, ok := result["orphanedBaseCommit"].(string); ok && orphaned != "" {
			data["orphanedBaseCommit"] = orphaned
		}
		if hash, ok := result["businessLogicPrBodyHash"].(string); ok && hash != "" {
			data["businessLogicHash"] = hash
		}
		if warnings, ok := result["reviewWarnings"].([]any); ok && len(warnings) > 0 {
			data["reviewWarnings"] = warnings
		}
	}

	return data
}

func isCommandOrigin(origin string) bool {
	return strings.HasPrefix(strings.ToLower(origin), "command") ||
		strings.HasPrefix(strings.ToLower(origin), "cli")
}

func holderVisibleUntil(exec *domain.AutomationExecutionEntity) time.Time {
	since := time.Now().UTC()
	if exec != nil && !exec.CreatedAt.IsZero() {
		since = exec.CreatedAt
	}
	return since.Add(time.Duration(activeExecutionLookbackMinutes) * time.Minute)
}
