package clireview

import (
	"github.com/scandrix/backend/internal/clireview/domain"
)

type ITrialRateLimiterService = domain.ITrialRateLimiterService
type IAuthenticatedRateLimiterService = domain.IAuthenticatedRateLimiterService
type RateLimitResult = domain.RateLimitResult
type AuthenticatedRateLimitResult = domain.AuthenticatedRateLimitResult
type ReadTraceDecisionBranchInput = domain.ReadTraceDecisionBranchInput
type ITraceDecisionBranchReader = domain.ITraceDecisionBranchReader
type IGitHubPublicPrService = domain.IGitHubPublicPrService
type IPublicPrAiSummaryService = domain.IPublicPrAiSummaryService
type IPublicPrGroupingService = domain.IPublicPrGroupingService
type IFeaturedPublicReviewRepository = domain.IFeaturedPublicReviewRepository
type ISessionEventRepository = domain.ISessionEventRepository
type OrphanedSessionRef = domain.OrphanedSessionRef
type ICliSessionCaptureRepository = domain.ICliSessionCaptureRepository
type IAutomationExecutionService = domain.IAutomationExecutionService
type IDrixyRulesService = domain.IDrixyRulesService
type IParametersService = domain.IParametersService
type ICodeManagementService = domain.ICodeManagementService
