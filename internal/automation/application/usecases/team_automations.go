package usecases

import (
	"context"
	"fmt"

	"github.com/scandrix/backend/internal/automation/domain"
)

// Integration and configuration keys matching platform conventions.
const (
	ConfigKeyDailyCheckinSchedule       = "DAILY_CHECKIN_SCHEDULE"
	ConfigKeyAutomationIssueAlert       = "AUTOMATION_ISSUE_ALERT_TIME"
	IntegrationCategoryCommunication    = "COMMUNICATION"
	ProfileConfigKeyUserNotifications   = "USER_NOTIFICATIONS"
)

// IntegrationConfigService provides configuration storage for platform integrations.
type IntegrationConfigService interface {
	CreateOrUpdateConfig(ctx context.Context, key string, value any, integrationUUID *string, orgTeamData map[string]any) error
}

// IntegrationService provides lookup for active platform integrations.
type IntegrationService interface {
	FindOne(ctx context.Context, filter map[string]any) (map[string]any, error)
}

// ProfileConfigService provides configuration storage for user profiles and notifications.
type ProfileConfigService interface {
	FindOne(ctx context.Context, filter map[string]any) (map[string]any, error)
}

// TeamAutomationInput models an automation configuration item for a team.
type TeamAutomationInput struct {
	AutomationUUID string                `json:"automation_uuid"`
	AutomationType domain.AutomationType `json:"automation_type"`
	Status         bool                  `json:"status"`
}

// TeamAutomationsDTO encapsulates team automation batch updates.
type TeamAutomationsDTO struct {
	TeamID      string                `json:"team_id"`
	Automations []TeamAutomationInput `json:"automations"`
}

// UpdateTeamAutomationStatusUseCase updates the active flag on a team automation binding.
type UpdateTeamAutomationStatusUseCase struct {
	teamAutoService domain.TeamAutomationService
}

// NewUpdateTeamAutomationStatusUseCase initializes the status update use case.
func NewUpdateTeamAutomationStatusUseCase(teamAutoService domain.TeamAutomationService) *UpdateTeamAutomationStatusUseCase {
	return &UpdateTeamAutomationStatusUseCase{teamAutoService: teamAutoService}
}

func (uc *UpdateTeamAutomationStatusUseCase) Execute(ctx context.Context, teamAutomationID string, status bool) (*domain.TeamAutomationEntity, error) {
	return uc.teamAutoService.Update(ctx, map[string]any{"uuid": teamAutomationID}, map[string]any{
		"status": status,
	})
}

// UpdateOrCreateTeamAutomationUseCase synchronizes team automations against configured strategies.
type UpdateOrCreateTeamAutomationUseCase struct {
	teamAutoService      domain.TeamAutomationService
	execAutoService      domain.ExecuteAutomationService
	profileConfigService ProfileConfigService
}

// NewUpdateOrCreateTeamAutomationUseCase initializes the sync use case.
func NewUpdateOrCreateTeamAutomationUseCase(
	teamAutoService domain.TeamAutomationService,
	execAutoService domain.ExecuteAutomationService,
) *UpdateOrCreateTeamAutomationUseCase {
	return &UpdateOrCreateTeamAutomationUseCase{
		teamAutoService: teamAutoService,
		execAutoService: execAutoService,
	}
}

// SetProfileConfigService attaches an optional profile config service.
func (uc *UpdateOrCreateTeamAutomationUseCase) SetProfileConfigService(svc ProfileConfigService) {
	uc.profileConfigService = svc
}

func (uc *UpdateOrCreateTeamAutomationUseCase) Execute(ctx context.Context, dto TeamAutomationsDTO, organizationID string) (any, error) {
	existing, err := uc.teamAutoService.Find(ctx, map[string]any{
		"teamId": dto.TeamID,
	})
	if err != nil {
		return nil, err
	}

	orgTeamData := map[string]any{
		"organizationId": organizationID,
		"teamId":         dto.TeamID,
	}

	if len(existing) == 0 {
		for _, auto := range dto.Automations {
			if uc.execAutoService != nil {
				_ = uc.execAutoService.SetupStrategy(ctx, auto.AutomationType, orgTeamData)
			}
			_, _ = uc.teamAutoService.Create(ctx, &domain.TeamAutomationEntity{
				Status:       auto.Status,
				TeamID:       dto.TeamID,
				AutomationID: auto.AutomationUUID,
			})
		}
	} else {
		existingMap := make(map[string]*domain.TeamAutomationEntity)
		for _, old := range existing {
			if old.AutomationID != "" {
				existingMap[old.AutomationID] = old
			}
		}

		for _, auto := range dto.Automations {
			if old, found := existingMap[auto.AutomationUUID]; found {
				_, _ = uc.teamAutoService.Update(ctx, map[string]any{"uuid": old.UUID}, map[string]any{
					"status":       auto.Status,
					"teamId":       dto.TeamID,
					"automationId": auto.AutomationUUID,
				})
			} else if auto.Status {
				if uc.execAutoService != nil {
					_ = uc.execAutoService.SetupStrategy(ctx, auto.AutomationType, orgTeamData)
				}
				_, _ = uc.teamAutoService.Create(ctx, &domain.TeamAutomationEntity{
					Status:       auto.Status,
					TeamID:       dto.TeamID,
					AutomationID: auto.AutomationUUID,
				})
			}
		}
	}

	if uc.profileConfigService != nil {
		profileConfig, err := uc.profileConfigService.FindOne(ctx, map[string]any{
			"configKey": ProfileConfigKeyUserNotifications,
		})
		if err == nil && profileConfig != nil {
			if cfgVal, ok := profileConfig["configValue"].(map[string]any); ok {
				return map[string]any{
					"id":   cfgVal["communicationId"],
					"name": cfgVal["name"],
				}, nil
			}
		}
		return "Team members not found", nil
	}

	return nil, nil
}

// ActiveCodeManagementTeamAutomationsUseCase activates code management automations for a team.
type ActiveCodeManagementTeamAutomationsUseCase struct {
	automationService domain.AutomationService
	updateOrCreate    *UpdateOrCreateTeamAutomationUseCase
}

// NewActiveCodeManagementTeamAutomationsUseCase creates the use case.
func NewActiveCodeManagementTeamAutomationsUseCase(
	automationService domain.AutomationService,
	updateOrCreate *UpdateOrCreateTeamAutomationUseCase,
) *ActiveCodeManagementTeamAutomationsUseCase {
	return &ActiveCodeManagementTeamAutomationsUseCase{
		automationService: automationService,
		updateOrCreate:    updateOrCreate,
	}
}

func (uc *ActiveCodeManagementTeamAutomationsUseCase) Execute(ctx context.Context, teamID, organizationID string) ([]TeamAutomationInput, error) {
	codeMgmtTypes := domain.AutomationCategoryMapping[domain.CategoryCodeManagement]

	automations, err := uc.automationService.Find(ctx, map[string]any{
		"status": true,
		"level":  domain.LevelTeam,
	})
	if err != nil {
		return nil, err
	}

	typeSet := make(map[domain.AutomationType]bool)
	for _, t := range codeMgmtTypes {
		typeSet[t] = true
	}

	var inputs []TeamAutomationInput
	for _, a := range automations {
		if typeSet[a.AutomationType] {
			inputs = append(inputs, TeamAutomationInput{
				AutomationUUID: a.UUID,
				AutomationType: a.AutomationType,
				Status:         a.Status,
			})
		}
	}

	dto := TeamAutomationsDTO{
		TeamID:      teamID,
		Automations: inputs,
	}

	_, err = uc.updateOrCreate.Execute(ctx, dto, organizationID)
	if err != nil {
		return nil, err
	}

	return inputs, nil
}

// ActiveCodeReviewAutomationUseCase activates review automation specifically.
type ActiveCodeReviewAutomationUseCase struct {
	teamAutoService domain.TeamAutomationService
	updateStatus    *UpdateTeamAutomationStatusUseCase
}

// NewActiveCodeReviewAutomationUseCase initializes the code review activation use case.
func NewActiveCodeReviewAutomationUseCase(
	teamAutoService domain.TeamAutomationService,
	updateStatus *UpdateTeamAutomationStatusUseCase,
) *ActiveCodeReviewAutomationUseCase {
	return &ActiveCodeReviewAutomationUseCase{
		teamAutoService: teamAutoService,
		updateStatus:    updateStatus,
	}
}

func (uc *ActiveCodeReviewAutomationUseCase) Execute(ctx context.Context, teamID string, automations []TeamAutomationInput) error {
	var reviewAuto *TeamAutomationInput
	for i := range automations {
		if automations[i].AutomationType == domain.AutomationCodeReview {
			reviewAuto = &automations[i]
			break
		}
	}

	if reviewAuto == nil || reviewAuto.AutomationUUID == "" {
		return nil
	}

	results, err := uc.teamAutoService.Find(ctx, map[string]any{
		"teamId":       teamID,
		"automationId": reviewAuto.AutomationUUID,
	})
	if err != nil || len(results) == 0 {
		return fmt.Errorf("team automation not found for review activation")
	}

	_, err = uc.updateStatus.Execute(ctx, results[0].UUID, true)
	return err
}

// ActiveTeamAutomationsUseCase activates all standard team automations and sets up default integration schedules.
type ActiveTeamAutomationsUseCase struct {
	automationService        domain.AutomationService
	updateOrCreate           *UpdateOrCreateTeamAutomationUseCase
	integrationService       IntegrationService
	integrationConfigService IntegrationConfigService
}

// NewActiveTeamAutomationsUseCase creates the use case.
func NewActiveTeamAutomationsUseCase(
	automationService domain.AutomationService,
	updateOrCreate *UpdateOrCreateTeamAutomationUseCase,
) *ActiveTeamAutomationsUseCase {
	return &ActiveTeamAutomationsUseCase{
		automationService: automationService,
		updateOrCreate:    updateOrCreate,
	}
}

// SetIntegrationServices configures optional integration services for default schedule hooks.
func (uc *ActiveTeamAutomationsUseCase) SetIntegrationServices(
	integrationService IntegrationService,
	integrationConfigService IntegrationConfigService,
) {
	uc.integrationService = integrationService
	uc.integrationConfigService = integrationConfigService
}

func (uc *ActiveTeamAutomationsUseCase) Execute(ctx context.Context, teamID, organizationID string) error {
	automations, err := uc.automationService.Find(ctx, map[string]any{
		"status": true,
		"level":  domain.LevelTeam,
	})
	if err != nil {
		return err
	}

	var inputs []TeamAutomationInput
	for _, a := range automations {
		inputs = append(inputs, TeamAutomationInput{
			AutomationUUID: a.UUID,
			AutomationType: a.AutomationType,
			Status:         a.Status,
		})
	}

	dto := TeamAutomationsDTO{
		TeamID:      teamID,
		Automations: inputs,
	}

	_, err = uc.updateOrCreate.Execute(ctx, dto, organizationID)
	if err != nil {
		return err
	}

	if uc.integrationService != nil && uc.integrationConfigService != nil {
		integration, err := uc.integrationService.FindOne(ctx, map[string]any{
			"organization":        map[string]any{"uuid": organizationID},
			"team":                map[string]any{"uuid": teamID},
			"integrationCategory": IntegrationCategoryCommunication,
			"status":              true,
		})
		var integrationUUID *string
		if err == nil && integration != nil {
			if id, ok := integration["uuid"].(string); ok && id != "" {
				integrationUUID = &id
			}
		}

		orgTeamData := map[string]any{
			"organizationId": organizationID,
			"teamId":         teamID,
		}

		// By default, the daily check-in schedule is 12:00 UTC (9 AM EST)
		_ = uc.integrationConfigService.CreateOrUpdateConfig(
			ctx,
			ConfigKeyDailyCheckinSchedule,
			map[string]any{"utc": "12:00"},
			integrationUUID,
			orgTeamData,
		)

		// By default, the automation issue alert schedule is 11:00 UTC (8 AM EST)
		_ = uc.integrationConfigService.CreateOrUpdateConfig(
			ctx,
			ConfigKeyAutomationIssueAlert,
			map[string]any{"utc": "11:00"},
			integrationUUID,
			orgTeamData,
		)
	}

	return nil
}
