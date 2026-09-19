package automation_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/automation/application/usecases"
	"github.com/scandrix/backend/internal/automation/domain"
	"github.com/scandrix/backend/internal/automation/infrastructure/http"
	"github.com/scandrix/backend/internal/automation/infrastructure/repositories"
	"github.com/scandrix/backend/internal/automation/infrastructure/services"
	"github.com/scandrix/backend/internal/automation/infrastructure/strategies"
	"github.com/scandrix/backend/internal/automation/webhook"
)

type mockReviewHandler struct {
	mu           sync.Mutex
	handledCalls []map[string]any
}

func (m *mockReviewHandler) HandlePullRequest(ctx context.Context, payload map[string]any) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handledCalls = append(m.handledCalls, payload)
	return map[string]any{
		"statusInfo": map[string]any{
			"status":  "success",
			"message": "Review completed cleanly",
		},
		"lastAnalyzedCommit": map[string]any{
			"sha": "abc123commit",
		},
	}, nil
}

func TestAutomationRepositoriesAndServices(t *testing.T) {
	ctx := context.Background()

	autoRepo := repositories.NewInMemoryAutomationRepository()
	autoSvc := services.NewDefaultAutomationService(autoRepo)

	// Verify defaults
	autos, err := autoSvc.Find(ctx, nil)
	if err != nil || len(autos) == 0 {
		t.Fatalf("expected seeded default automations, got %d, err: %v", len(autos), err)
	}

	foundCodeReview, err := autoSvc.FindOne(ctx, map[string]any{
		"automationType": domain.AutomationCodeReview,
	})
	if err != nil || foundCodeReview == nil {
		t.Fatalf("expected code review automation, got nil, err: %v", err)
	}

	// Team automations
	teamRepo := repositories.NewInMemoryTeamAutomationRepository()
	teamSvc := services.NewDefaultTeamAutomationService(teamRepo)

	teamAuto, err := teamSvc.Create(ctx, &domain.TeamAutomationEntity{
		Status:       false,
		AutomationID: foundCodeReview.UUID,
		TeamID:       "team-42",
	})
	if err != nil || teamAuto.UUID == "" {
		t.Fatalf("failed to create team automation: %v", err)
	}

	// Executions and stages
	execRepo := repositories.NewInMemoryAutomationExecutionRepository()
	stageRepo := repositories.NewInMemoryCodeReviewExecutionRepository()
	execSvc := services.NewDefaultAutomationExecutionService(execRepo, stageRepo)

	res, err := execSvc.CreateCodeReview(ctx, &domain.AutomationExecutionEntity{
		Status:            domain.StatusInProgress,
		PullRequestNumber: 101,
		RepositoryID:      "repo-1",
		TeamAutomation:    teamAuto,
		Origin:            "webhook",
	}, "Starting Drixy review", "Drixy Review Started")

	if err != nil || res.Execution == nil || res.StageLog == nil {
		t.Fatalf("failed to create code review execution: %v", err)
	}

	// Verify stage log exists
	exists, err := stageRepo.ExistsByAutomationExecutionAndStageStatus(
		ctx,
		res.Execution.UUID,
		"Drixy Review Started",
		domain.StatusInProgress,
	)
	if err != nil || !exists {
		t.Fatalf("expected stage log to exist, err: %v", err)
	}

	// Update code review
	updatedExec, err := execSvc.UpdateCodeReview(ctx, map[string]any{
		"uuid": res.Execution.UUID,
	}, map[string]any{
		"status": domain.StatusSuccess,
		"dataExecution": map[string]any{
			"lastAnalyzedCommit": map[string]any{"sha": "c0ffee"},
		},
	}, "Review finished successfully", "Drixy Review Finished")

	if err != nil || updatedExec.Status != domain.StatusSuccess {
		t.Fatalf("failed to update code review: %v", err)
	}

	// Check distinct reviewed PR keys
	keys, err := execRepo.GetDistinctReviewedPullRequestKeys(ctx, domain.DistinctReviewedParams{
		RepositoryIDs: []string{"repo-1"},
	})
	if err != nil || len(keys) != 1 {
		t.Fatalf("expected 1 reviewed PR key, got %d, err: %v", len(keys), err)
	}
	if keys[0].PullRequestNumber != 101 {
		t.Errorf("expected PR number 101, got %d", keys[0].PullRequestNumber)
	}

	// Check eligible refs
	refs, err := execRepo.FindEligiblePullRequestRefsForApprovalByPeriodAndTeamAutomationID(
		ctx,
		teamAuto.UUID,
		time.Time{},
		time.Time{},
	)
	if err != nil || len(refs) != 1 {
		t.Fatalf("expected 1 eligible ref, got %d, err: %v", len(refs), err)
	}
	if refs[0].HeadCommitSHA != "c0ffee" {
		t.Errorf("expected commit c0ffee, got %s", refs[0].HeadCommitSHA)
	}
}

func TestAutomationCodeReviewStrategy(t *testing.T) {
	ctx := context.Background()

	autoRepo := repositories.NewInMemoryAutomationRepository()
	autoSvc := services.NewDefaultAutomationService(autoRepo)
	teamRepo := repositories.NewInMemoryTeamAutomationRepository()
	teamSvc := services.NewDefaultTeamAutomationService(teamRepo)
	execRepo := repositories.NewInMemoryAutomationExecutionRepository()
	stageRepo := repositories.NewInMemoryCodeReviewExecutionRepository()
	execSvc := services.NewDefaultAutomationExecutionService(execRepo, stageRepo)
	lockSvc := strategies.NewSimpleLockService()
	mockHandler := &mockReviewHandler{}

	strategySvc := strategies.NewAutomationCodeReviewService(
		nil,
		teamSvc,
		autoSvc,
		execSvc,
		lockSvc,
		mockHandler,
	)

	err := strategySvc.Setup(ctx, map[string]any{
		"teamId": "team-test-1",
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	teamAutos, _ := teamSvc.Find(ctx, map[string]any{"teamId": "team-test-1"})
	if len(teamAutos) == 0 {
		t.Fatalf("expected team automation to be created in setup")
	}

	// Run review
	payload := map[string]any{
		"organizationAndTeamData": map[string]any{
			"organizationId": "org-1",
			"teamId":         "team-test-1",
		},
		"repository": map[string]any{
			"id":   "repo-test-1",
			"name": "org/repo-test-1",
		},
		"pullRequest": map[string]any{
			"number": 55,
		},
		"teamAutomationId": teamAutos[0].UUID,
		"origin":           "webhook",
	}

	result, err := strategySvc.Run(ctx, payload)
	if err != nil {
		t.Fatalf("unexpected error running review: %v", err)
	}
	if result != "Automation executed successfully" {
		t.Errorf("expected success message, got %v", result)
	}

	// Verify handler was called
	mockHandler.mu.Lock()
	calls := len(mockHandler.handledCalls)
	mockHandler.mu.Unlock()
	if calls != 1 {
		t.Errorf("expected handler to be called once, got %d", calls)
	}
}

func TestTeamAutomationUseCases(t *testing.T) {
	ctx := context.Background()

	autoRepo := repositories.NewInMemoryAutomationRepository()
	autoSvc := services.NewDefaultAutomationService(autoRepo)
	teamRepo := repositories.NewInMemoryTeamAutomationRepository()
	teamSvc := services.NewDefaultTeamAutomationService(teamRepo)

	updateOrCreateUC := usecases.NewUpdateOrCreateTeamAutomationUseCase(teamSvc, nil)
	updateStatusUC := usecases.NewUpdateTeamAutomationStatusUseCase(teamSvc)
	activeCodeMgmtUC := usecases.NewActiveCodeManagementTeamAutomationsUseCase(autoSvc, updateOrCreateUC)
	activeReviewUC := usecases.NewActiveCodeReviewAutomationUseCase(teamSvc, updateStatusUC)

	// Activate code management
	inputs, err := activeCodeMgmtUC.Execute(ctx, "team-alpha", "org-alpha")
	if err != nil {
		t.Fatalf("failed to activate code management: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("expected active code management inputs, got 0")
	}

	// Activate code review specifically
	err = activeReviewUC.Execute(ctx, "team-alpha", inputs)
	if err != nil {
		t.Fatalf("failed to activate code review: %v", err)
	}

	// Verify team automation status is true
	teamAutos, err := teamSvc.Find(ctx, map[string]any{"teamId": "team-alpha"})
	if err != nil || len(teamAutos) == 0 {
		t.Fatalf("expected team automations, got none, err: %v", err)
	}
	if !teamAutos[0].Status {
		t.Errorf("expected team automation status to be true after review activation")
	}
}

type mockJobRepo struct {
	mu   sync.Mutex
	jobs map[string]*webhook.WorkflowJob
}

func (m *mockJobRepo) FindOne(ctx context.Context, id string) (*webhook.WorkflowJob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jobs[id], nil
}

func (m *mockJobRepo) Update(ctx context.Context, id string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	job := m.jobs[id]
	if job == nil {
		return nil
	}
	if st, ok := updates["status"].(webhook.JobStatus); ok {
		job.Status = st
	}
	return nil
}

type mockWebhookHandler struct {
	handled bool
}

func (h *mockWebhookHandler) CanHandle(params webhook.WebhookEventParams) bool {
	return true
}

func (h *mockWebhookHandler) Handle(ctx context.Context, params webhook.WebhookEventParams) error {
	h.handled = true
	return nil
}

func TestWebhookProcessingJobProcessor(t *testing.T) {
	ctx := context.Background()

	jobRepo := &mockJobRepo{
		jobs: map[string]*webhook.WorkflowJob{
			"job-1": {
				ID:            "job-1",
				WorkflowType:  "WEBHOOK_PROCESSING",
				CorrelationID: "corr-1",
				Metadata: map[string]any{
					"platformType": "github",
					"event":        "pull_request",
				},
				Payload: map[string]any{"action": "opened"},
				Status:  webhook.JobStatusPending,
			},
		},
	}

	processor := webhook.NewWebhookProcessingJobProcessor(nil, jobRepo)
	handler := &mockWebhookHandler{}
	processor.RegisterHandler(webhook.PlatformGitHub, handler)

	err := processor.Process(ctx, "job-1")
	if err != nil {
		t.Fatalf("processor failed: %v", err)
	}

	if !handler.handled {
		t.Errorf("expected handler to have processed event")
	}

	job, _ := jobRepo.FindOne(ctx, "job-1")
	if job.Status != webhook.JobStatusCompleted {
		t.Errorf("expected job status COMPLETED, got %s", job.Status)
	}
}

func TestAutomationReviewLabels(t *testing.T) {
	v3 := domain.GetReviewLabels(domain.CodeReviewVersionV3Agent)
	if len(v3) != 4 {
		t.Fatalf("expected 4 v3 labels, got %d", len(v3))
	}
	if v3[0].Type != "bug" || v3[1].Type != "security" {
		t.Errorf("unexpected v3 label ordering: %+v", v3)
	}

	v2 := domain.GetReviewLabels(domain.CodeReviewVersionV2)
	if len(v2) != 4 {
		t.Fatalf("expected 4 v2 labels, got %d", len(v2))
	}

	legacy := domain.GetReviewLabels(domain.CodeReviewVersionLegacy)
	if len(legacy) != 11 {
		t.Fatalf("expected 11 legacy labels, got %d", len(legacy))
	}
	hasDrixyRules := false
	for _, l := range legacy {
		if l.Type == "drixy_rules" {
			hasDrixyRules = true
			break
		}
	}
	if !hasDrixyRules {
		t.Errorf("expected drixy_rules in legacy labels, got %+v", legacy)
	}
}

type mockAutomationFactory struct {
	mu         sync.Mutex
	autoType   domain.AutomationType
	setupCalls int
	runCalls   int
	stopCalls  int
}

func (m *mockAutomationFactory) GetAutomationType() domain.AutomationType {
	return m.autoType
}

func (m *mockAutomationFactory) Setup(ctx context.Context, payload any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setupCalls++
	return nil
}

func (m *mockAutomationFactory) Run(ctx context.Context, payload any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runCalls++
	return "mock run success", nil
}

func (m *mockAutomationFactory) Stop(ctx context.Context, payload any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalls++
	return nil
}

func TestAutomationRegistryAndExecuteService(t *testing.T) {
	ctx := context.Background()

	factoryCodeReview := &mockAutomationFactory{autoType: domain.AutomationCodeReview}
	factoryReport := &mockAutomationFactory{autoType: domain.AutomationDailyCheckin}

	registry := services.NewAutomationRegistry(factoryCodeReview)
	registry.Register(factoryReport)

	// Test GetStrategy
	s1, err := registry.GetStrategy(domain.AutomationCodeReview)
	if err != nil || s1 == nil {
		t.Fatalf("expected code review strategy, got error: %v", err)
	}
	if s1.GetAutomationType() != domain.AutomationCodeReview {
		t.Errorf("expected AutomationCodeReview, got %s", s1.GetAutomationType())
	}

	// Test Unsupported strategy
	_, err = registry.GetStrategy("UNSUPPORTED_TYPE")
	if err == nil {
		t.Fatalf("expected error for unsupported strategy, got nil")
	}

	// Test ExecuteAutomationService
	execSvc := services.NewDefaultExecuteAutomationService(registry)

	// SetupStrategy
	err = execSvc.SetupStrategy(ctx, domain.AutomationCodeReview, map[string]any{"teamId": "t-1"})
	if err != nil {
		t.Fatalf("setup strategy failed: %v", err)
	}

	// ExecuteStrategy
	res, err := execSvc.ExecuteStrategy(ctx, domain.AutomationCodeReview, map[string]any{"action": "run"})
	if err != nil || res != "mock run success" {
		t.Fatalf("execute strategy failed: %v, res: %v", err, res)
	}

	// StopStrategy
	err = execSvc.StopStrategy(ctx, domain.AutomationCodeReview, map[string]any{"reason": "cancelled"})
	if err != nil {
		t.Fatalf("stop strategy failed: %v", err)
	}

	// GetAutomationMethods
	methodStrategy, err := execSvc.GetAutomationMethods(domain.AutomationDailyCheckin)
	if err != nil || methodStrategy == nil {
		t.Fatalf("get automation methods failed: %v", err)
	}

	// Verify call counts
	factoryCodeReview.mu.Lock()
	if factoryCodeReview.setupCalls != 1 || factoryCodeReview.runCalls != 1 || factoryCodeReview.stopCalls != 1 {
		t.Errorf("unexpected call counts on code review factory: setup=%d, run=%d, stop=%d",
			factoryCodeReview.setupCalls, factoryCodeReview.runCalls, factoryCodeReview.stopCalls)
	}
	factoryCodeReview.mu.Unlock()
}

func TestAutomationDTOValidation(t *testing.T) {
	// Valid single DTO
	dto := http.AutomationDTO{
		AutomationUUID: "11111111-1111-1111-1111-111111111111",
		AutomationType: domain.AutomationCodeReview,
		Status:         true,
	}
	if err := dto.Validate(); err != nil {
		t.Fatalf("expected valid DTO, got: %v", err)
	}

	// Invalid UUID
	invalidUUID := http.AutomationDTO{
		AutomationUUID: "not-a-uuid",
		AutomationType: domain.AutomationCodeReview,
		Status:         true,
	}
	if err := invalidUUID.Validate(); err == nil {
		t.Errorf("expected error for invalid UUID, got nil")
	}

	// Missing type
	missingType := http.AutomationDTO{
		AutomationUUID: "11111111-1111-1111-1111-111111111111",
		AutomationType: "",
		Status:         true,
	}
	if err := missingType.Validate(); err == nil {
		t.Errorf("expected error for missing type, got nil")
	}

	// Valid TeamAutomationsDTO
	teamDTO := http.TeamAutomationsDTO{
		TeamID:      "team-123",
		Automations: []http.AutomationDTO{dto},
	}
	if err := teamDTO.Validate(); err != nil {
		t.Fatalf("expected valid TeamAutomationsDTO, got: %v", err)
	}

	// Empty teamId
	invalidTeamDTO := http.TeamAutomationsDTO{
		TeamID:      "",
		Automations: []http.AutomationDTO{dto},
	}
	if err := invalidTeamDTO.Validate(); err == nil {
		t.Errorf("expected error for empty teamId, got nil")
	}

	// Valid OrganizationAutomationsDTO
	orgDTO := http.OrganizationAutomationsDTO{
		OrganizationID: "org-123",
		Automations:    []http.AutomationDTO{dto},
	}
	if err := orgDTO.Validate(); err != nil {
		t.Fatalf("expected valid OrganizationAutomationsDTO, got: %v", err)
	}

	// Empty orgId
	invalidOrgDTO := http.OrganizationAutomationsDTO{
		OrganizationID: "",
		Automations:    []http.AutomationDTO{dto},
	}
	if err := invalidOrgDTO.Validate(); err == nil {
		t.Errorf("expected error for empty organizationId, got nil")
	}
}

type mockIntegrationService struct {
	found map[string]any
}

func (m *mockIntegrationService) FindOne(ctx context.Context, filter map[string]any) (map[string]any, error) {
	return m.found, nil
}

type mockIntegrationConfigService struct {
	mu    sync.Mutex
	saved map[string]any
}

func (m *mockIntegrationConfigService) CreateOrUpdateConfig(
	ctx context.Context,
	key string,
	value any,
	integrationUUID *string,
	orgTeamData map[string]any,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saved == nil {
		m.saved = make(map[string]any)
	}
	m.saved[key] = value
	return nil
}

func TestActiveTeamAutomationsWithIntegrations(t *testing.T) {
	ctx := context.Background()

	autoRepo := repositories.NewInMemoryAutomationRepository()
	autoSvc := services.NewDefaultAutomationService(autoRepo)
	teamRepo := repositories.NewInMemoryTeamAutomationRepository()
	teamSvc := services.NewDefaultTeamAutomationService(teamRepo)

	updateOrCreateUC := usecases.NewUpdateOrCreateTeamAutomationUseCase(teamSvc, nil)
	activeTeamUC := usecases.NewActiveTeamAutomationsUseCase(autoSvc, updateOrCreateUC)

	intSvc := &mockIntegrationService{
		found: map[string]any{
			"uuid": "int-uuid-99",
		},
	}
	cfgSvc := &mockIntegrationConfigService{}
	activeTeamUC.SetIntegrationServices(intSvc, cfgSvc)

	err := activeTeamUC.Execute(ctx, "team-beta", "org-beta")
	if err != nil {
		t.Fatalf("activeTeamUC.Execute failed: %v", err)
	}

	cfgSvc.mu.Lock()
	defer cfgSvc.mu.Unlock()

	dailyVal, ok := cfgSvc.saved[usecases.ConfigKeyDailyCheckinSchedule].(map[string]any)
	if !ok || dailyVal["utc"] != "12:00" {
		t.Errorf("expected daily checkin 12:00 UTC, got: %v", cfgSvc.saved[usecases.ConfigKeyDailyCheckinSchedule])
	}

	alertVal, ok := cfgSvc.saved[usecases.ConfigKeyAutomationIssueAlert].(map[string]any)
	if !ok || alertVal["utc"] != "11:00" {
		t.Errorf("expected alert time 11:00 UTC, got: %v", cfgSvc.saved[usecases.ConfigKeyAutomationIssueAlert])
	}
}

type mockProfileConfigService struct {
	found map[string]any
}

func (m *mockProfileConfigService) FindOne(ctx context.Context, filter map[string]any) (map[string]any, error) {
	return m.found, nil
}

func TestUpdateOrCreateTeamAutomationWithProfileConfig(t *testing.T) {
	ctx := context.Background()

	teamRepo := repositories.NewInMemoryTeamAutomationRepository()
	teamSvc := services.NewDefaultTeamAutomationService(teamRepo)

	uc := usecases.NewUpdateOrCreateTeamAutomationUseCase(teamSvc, nil)

	// Test when profile config service is set and returns a value
	profileSvc := &mockProfileConfigService{
		found: map[string]any{
			"configValue": map[string]any{
				"communicationId": "comm-123",
				"name":            "Team Chat",
			},
		},
	}
	uc.SetProfileConfigService(profileSvc)

	dto := usecases.TeamAutomationsDTO{
		TeamID: "team-gamma",
		Automations: []usecases.TeamAutomationInput{
			{
				AutomationUUID: "auto-1",
				AutomationType: domain.AutomationCodeReview,
				Status:         true,
			},
		},
	}

	result, err := uc.Execute(ctx, dto, "org-gamma")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	resMap, ok := result.(map[string]any)
	if !ok || resMap["id"] != "comm-123" || resMap["name"] != "Team Chat" {
		t.Errorf("unexpected profile result: %v", result)
	}
}


