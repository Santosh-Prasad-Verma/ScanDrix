package automation

import (
	"github.com/scandrix/backend/internal/automation/application/usecases"
	"github.com/scandrix/backend/internal/automation/domain"
	"github.com/scandrix/backend/internal/automation/infrastructure/http"
	"github.com/scandrix/backend/internal/automation/infrastructure/repositories"
	"github.com/scandrix/backend/internal/automation/infrastructure/services"
	"github.com/scandrix/backend/internal/automation/infrastructure/strategies"
	"github.com/scandrix/backend/internal/automation/webhook"
)

// Re-export domain enums
type AutomationType = domain.AutomationType
type AutomationTypeCategory = domain.AutomationTypeCategory
type AutomationLevel = domain.AutomationLevel
type AutomationStatus = domain.AutomationStatus
type AutomationMessage = domain.AutomationMessage
type CodeReviewExecutionTrigger = domain.CodeReviewExecutionTrigger
type CodeReviewVersion = domain.CodeReviewVersion

const (
	AutomationCodeReview             = domain.AutomationCodeReview
	AutomationTeamProgress           = domain.AutomationTeamProgress
	AutomationInteractionMonitor     = domain.AutomationInteractionMonitor
	AutomationIssuesDetails          = domain.AutomationIssuesDetails
	AutomationImproveTask            = domain.AutomationImproveTask
	AutomationEnsureAssignees        = domain.AutomationEnsureAssignees
	AutomationCommitValidation       = domain.AutomationCommitValidation
	AutomationWipLimits              = domain.AutomationWipLimits
	AutomationWaitingConstraints     = domain.AutomationWaitingConstraints
	AutomationTaskBreakdown          = domain.AutomationTaskBreakdown
	AutomationUserRequestedBreakdown = domain.AutomationUserRequestedBreakdown
	AutomationRetroactiveMovement    = domain.AutomationRetroactiveMovement
	AutomationDailyCheckin           = domain.AutomationDailyCheckin
	AutomationSprintRetro            = domain.AutomationSprintRetro
	AutomationExecutiveCheckin       = domain.AutomationExecutiveCheckin

	CategoryCodeManagement           = domain.CategoryCodeManagement

	LevelOrganization                = domain.LevelOrganization
	LevelTeam                        = domain.LevelTeam
	LevelUser                        = domain.LevelUser

	StatusPending                    = domain.StatusPending
	StatusInProgress                 = domain.StatusInProgress
	StatusSuccess                    = domain.StatusSuccess
	StatusPartialError               = domain.StatusPartialError
	StatusError                      = domain.StatusError
	StatusSkipped                    = domain.StatusSkipped

	TriggerAutomatic                 = domain.TriggerAutomatic
	TriggerCommand                   = domain.TriggerCommand
	TriggerCommitPush                = domain.TriggerCommitPush

	CodeReviewVersionLegacy          = domain.CodeReviewVersionLegacy
	CodeReviewVersionV2              = domain.CodeReviewVersionV2
	CodeReviewVersionV3Agent         = domain.CodeReviewVersionV3Agent
)

// Re-export domain entities and schemas
type AutomationEntity = domain.AutomationEntity
type AutomationExecutionEntity = domain.AutomationExecutionEntity
type CodeReviewExecutionEntity = domain.CodeReviewExecutionEntity
type TeamAutomationEntity = domain.TeamAutomationEntity
type ReviewLabel = domain.ReviewLabel

// Re-export domain contracts and query params
type PRQueryParams = domain.PRQueryParams
type PRKey = domain.PRKey
type AwaitingReviewParams = domain.AwaitingReviewParams
type DistinctReviewedParams = domain.DistinctReviewedParams
type CliQueryParams = domain.CliQueryParams
type PRRef = domain.PRRef
type CreateCodeReviewResult = domain.CreateCodeReviewResult

type AutomationRepository = domain.AutomationRepository
type AutomationService = domain.AutomationService
type AutomationExecutionRepository = domain.AutomationExecutionRepository
type AutomationExecutionService = domain.AutomationExecutionService
type CodeReviewExecutionRepository = domain.CodeReviewExecutionRepository
type CodeReviewExecutionService = domain.CodeReviewExecutionService
type TeamAutomationRepository = domain.TeamAutomationRepository
type TeamAutomationService = domain.TeamAutomationService
type AutomationStrategy = domain.AutomationStrategy
type AutomationFactory = domain.AutomationFactory
type ExecuteAutomationService = domain.ExecuteAutomationService

// Re-export application use cases
type TeamAutomationInput = usecases.TeamAutomationInput
type UseCaseTeamAutomationsDTO = usecases.TeamAutomationsDTO
type UpdateTeamAutomationStatusUseCase = usecases.UpdateTeamAutomationStatusUseCase
type UpdateOrCreateTeamAutomationUseCase = usecases.UpdateOrCreateTeamAutomationUseCase
type ActiveCodeManagementTeamAutomationsUseCase = usecases.ActiveCodeManagementTeamAutomationsUseCase
type ActiveCodeReviewAutomationUseCase = usecases.ActiveCodeReviewAutomationUseCase
type ActiveTeamAutomationsUseCase = usecases.ActiveTeamAutomationsUseCase

type IntegrationConfigService = usecases.IntegrationConfigService
type IntegrationService = usecases.IntegrationService
type ProfileConfigService = usecases.ProfileConfigService

// Re-export infrastructure types
type AutomationRegistry = services.AutomationRegistry
type DefaultExecuteAutomationService = services.DefaultExecuteAutomationService
type DefaultAutomationService = services.DefaultAutomationService
type DefaultTeamAutomationService = services.DefaultTeamAutomationService
type DefaultCodeReviewExecutionService = services.DefaultCodeReviewExecutionService
type DefaultAutomationExecutionService = services.DefaultAutomationExecutionService

type AutomationCodeReviewService = strategies.AutomationCodeReviewService
type PrReviewInProgressError = strategies.PrReviewInProgressError
type DistributedLock = strategies.DistributedLock
type DistributedLockService = strategies.DistributedLockService
type SimpleLockService = strategies.SimpleLockService
type ReviewPipelineHandler = strategies.ReviewPipelineHandler

type InMemoryAutomationRepository = repositories.InMemoryAutomationRepository
type InMemoryAutomationExecutionRepository = repositories.InMemoryAutomationExecutionRepository
type InMemoryCodeReviewExecutionRepository = repositories.InMemoryCodeReviewExecutionRepository
type InMemoryTeamAutomationRepository = repositories.InMemoryTeamAutomationRepository

type AutomationDTO = http.AutomationDTO
type TeamAutomationsDTO = http.TeamAutomationsDTO
type OrganizationAutomationsDTO = http.OrganizationAutomationsDTO

type WebhookProcessingJobProcessor = webhook.WebhookProcessingJobProcessor
type WebhookEventHandler = webhook.WebhookEventHandler
type WebhookEventParams = webhook.WebhookEventParams
type WorkflowJob = webhook.WorkflowJob
type WorkflowJobRepository = webhook.WorkflowJobRepository
type PlatformType = webhook.PlatformType

// Re-export constructors and factory helpers
var (
	// Repositories
	NewInMemoryAutomationRepository          = repositories.NewInMemoryAutomationRepository
	NewInMemoryAutomationExecutionRepository = repositories.NewInMemoryAutomationExecutionRepository
	NewInMemoryCodeReviewExecutionRepository = repositories.NewInMemoryCodeReviewExecutionRepository
	NewInMemoryTeamAutomationRepository      = repositories.NewInMemoryTeamAutomationRepository

	// Services
	NewDefaultAutomationService              = services.NewDefaultAutomationService
	NewDefaultTeamAutomationService          = services.NewDefaultTeamAutomationService
	NewDefaultCodeReviewExecutionService     = services.NewDefaultCodeReviewExecutionService
	NewDefaultAutomationExecutionService     = services.NewDefaultAutomationExecutionService
	NewAutomationRegistry                    = services.NewAutomationRegistry
	NewDefaultExecuteAutomationService       = services.NewDefaultExecuteAutomationService

	// Strategies
	NewAutomationCodeReviewService           = strategies.NewAutomationCodeReviewService
	NewSimpleLockService                     = strategies.NewSimpleLockService

	// Use Cases
	NewUpdateTeamAutomationStatusUseCase         = usecases.NewUpdateTeamAutomationStatusUseCase
	NewUpdateOrCreateTeamAutomationUseCase       = usecases.NewUpdateOrCreateTeamAutomationUseCase
	NewActiveCodeManagementTeamAutomationsUseCase = usecases.NewActiveCodeManagementTeamAutomationsUseCase
	NewActiveCodeReviewAutomationUseCase         = usecases.NewActiveCodeReviewAutomationUseCase
	NewActiveTeamAutomationsUseCase              = usecases.NewActiveTeamAutomationsUseCase

	// Webhook
	NewWebhookProcessingJobProcessor = webhook.NewWebhookProcessingJobProcessor

	// Domain Helpers
	GetReviewLabels = domain.GetReviewLabels
)
