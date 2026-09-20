package clireview

import (
	"github.com/scandrix/backend/internal/clireview/application/usecases"
)

// CliReviewGitSummary provides repository and branch context for dashboard presentation.
type CliReviewGitSummary = usecases.CliReviewGitSummary

// CliReviewAuthSummary provides authenticated user identity for dashboard presentation.
type CliReviewAuthSummary = usecases.CliReviewAuthSummary

// CliReviewSummaryItem represents a row in the dashboard review history.
type CliReviewSummaryItem = usecases.CliReviewSummaryItem

// CliReviewTimelineEntry represents an execution phase with timestamps and metrics.
type CliReviewTimelineEntry = usecases.CliReviewTimelineEntry

// CliReviewDetailPayload contains comprehensive inspection details of a review.
type CliReviewDetailPayload = usecases.CliReviewDetailPayload

// PaginatedCliReviewSummaries wraps paginated dashboard search results.
type PaginatedCliReviewSummaries = usecases.PaginatedCliReviewSummaries

// MapExecutionToSummary converts raw telemetry execution record into UI presentation summary.
var MapExecutionToSummary = usecases.MapExecutionToSummary

// GetCliReviewsInput specifies search and pagination parameters for review history.
type GetCliReviewsInput = usecases.GetCliReviewsInput

// IAutomationExecutionQueryService defines repository search query contract.
type IAutomationExecutionQueryService = usecases.IAutomationExecutionQueryService

// GetCliReviewsUseCase queries and paginates historical review telemetry for dashboard.
type GetCliReviewsUseCase = usecases.GetCliReviewsUseCase

// NewGetCliReviewsUseCase creates an initialized review history use case.
var NewGetCliReviewsUseCase = usecases.NewGetCliReviewsUseCase

// GetCliReviewByIdInput parameterizes retrieving single execution details.
type GetCliReviewByIdInput = usecases.GetCliReviewByIdInput

// GetCliReviewByIdUseCase fetches comprehensive details and timeline of a specific review.
type GetCliReviewByIdUseCase = usecases.GetCliReviewByIdUseCase

// NewGetCliReviewByIdUseCase creates an initialized execution detail use case.
var NewGetCliReviewByIdUseCase = usecases.NewGetCliReviewByIdUseCase

// InMemoryAutomationQueryService provides an in-memory test implementation of IAutomationExecutionQueryService.
type InMemoryAutomationQueryService = usecases.InMemoryAutomationQueryService

// NewInMemoryAutomationQueryService creates a query service for testing.
var NewInMemoryAutomationQueryService = usecases.NewInMemoryAutomationQueryService
