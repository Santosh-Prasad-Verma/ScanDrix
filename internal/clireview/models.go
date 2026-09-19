package clireview

import (
	"github.com/scandrix/backend/internal/clireview/domain"
)

// Re-export domain models
type CliReviewIssueFix = domain.CliReviewIssueFix
type CliReviewIssue = domain.CliReviewIssue
type CliReviewResponse = domain.CliReviewResponse
type TrialCliReviewResponse = domain.TrialCliReviewResponse
type RateLimitMetadata = domain.RateLimitMetadata
type CliFileInput = domain.CliFileInput
type CliReviewRuleToggles = domain.CliReviewRuleToggles
type CliReviewConfig = domain.CliReviewConfig
type CliReviewInput = domain.CliReviewInput
type ValidateCliKeyInput = domain.ValidateCliKeyInput
type ValidateCliKeyEntity = domain.ValidateCliKeyEntity
type ValidateCliKeyUser = domain.ValidateCliKeyUser
type ValidateCliKeyResult = domain.ValidateCliKeyResult
type CliSessionDecisionType = domain.CliSessionDecisionType

const (
	DecisionArchitecturalDetail = domain.DecisionArchitecturalDetail
	DecisionConvention          = domain.DecisionConvention
	DecisionTradeoff            = domain.DecisionTradeoff
	DecisionImplementation      = domain.DecisionImplementation
	DecisionTooling             = domain.DecisionTooling
	DecisionOther               = domain.DecisionOther
)

type CliSessionDecisionOrigin = domain.CliSessionDecisionOrigin

const (
	OriginHuman         = domain.OriginHuman
	OriginAgent         = domain.OriginAgent
	OriginCollaborative = domain.OriginCollaborative
)

type CliSessionClassifiedDecision = domain.CliSessionClassifiedDecision
type CliSessionToolUse = domain.CliSessionToolUse
type CliSessionSignals = domain.CliSessionSignals
type CliSessionCapture = domain.CliSessionCapture
type SessionEvent = domain.SessionEvent
type TraceDecisionBranch = domain.TraceDecisionBranch
type TraceContextPack = domain.TraceContextPack
type GitContext = domain.GitContext
type CliAuthContext = domain.CliAuthContext
type EnqueueCliReviewInput = domain.EnqueueCliReviewInput
type EnqueueCliReviewResult = domain.EnqueueCliReviewResult
type CliReviewsQuery = domain.CliReviewsQuery
type CliReviewSummary = domain.CliReviewSummary
type TraceContextDecision = domain.TraceContextDecision

const TraceContextPackTokenBudget = domain.TraceContextPackTokenBudget

var EstimateTokens = domain.EstimateTokens

type TraceDecisionBranchRecord = domain.TraceDecisionBranchRecord
type PublicPrAuthor = domain.PublicPrAuthor
type PublicPrLabel = domain.PublicPrLabel
type PublicPrAssignee = domain.PublicPrAssignee
type PublicPrReviewer = domain.PublicPrReviewer
type PublicPrCheckSummary = domain.PublicPrCheckSummary
type PublicPrCommit = domain.PublicPrCommit
type PublicPrComment = domain.PublicPrComment
type PublicPrMetadata = domain.PublicPrMetadata
type PublicPrFetchError = domain.PublicPrFetchError
type ParsedPrURL = domain.ParsedPrURL
type FeaturedPublicReview = domain.FeaturedPublicReview
type FeaturedPublicReviewListItem = domain.FeaturedPublicReviewListItem
type CliReviewsListResponse = domain.CliReviewsListResponse
type PublicPrGrouping = domain.PublicPrGrouping
type ExecutionAuthContext = domain.ExecutionAuthContext
type ExecuteCliReviewInput = domain.ExecuteCliReviewInput
type OrganizationAndTeamData = domain.OrganizationAndTeamData
type CodeReviewConfig = domain.CodeReviewConfig
type ReviewOptions = domain.ReviewOptions
type DrixyRule = domain.DrixyRule
type RepositoryRef = domain.RepositoryRef
type AutomationExecutionRecord = domain.AutomationExecutionRecord
type FileChange = domain.FileChange
type CodeSuggestion = domain.CodeSuggestion
