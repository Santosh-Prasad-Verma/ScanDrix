package clireview

import (
	"github.com/scandrix/backend/internal/clireview/application/usecases"
)

// GetFeaturedPublicReviewInput identifies showcase review to retrieve.
type GetFeaturedPublicReviewInput = usecases.GetFeaturedPublicReviewInput

// GetFeaturedPublicReviewResult holds showcase details for UI presentation.
type GetFeaturedPublicReviewResult = usecases.GetFeaturedPublicReviewResult

// GetFeaturedPublicReviewUseCase retrieves a single featured review by slug.
type GetFeaturedPublicReviewUseCase = usecases.GetFeaturedPublicReviewUseCase

// NewGetFeaturedPublicReviewUseCase creates an initialized featured review retriever.
var NewGetFeaturedPublicReviewUseCase = usecases.NewGetFeaturedPublicReviewUseCase

// ListFeaturedPublicReviewsUseCase lists published showcase reviews for landing page.
type ListFeaturedPublicReviewsUseCase = usecases.ListFeaturedPublicReviewsUseCase

// NewListFeaturedPublicReviewsUseCase creates a showcase catalog use case.
var NewListFeaturedPublicReviewsUseCase = usecases.NewListFeaturedPublicReviewsUseCase

// IngestSessionEventInput defines telemetry event received from CLI.
type IngestSessionEventInput = usecases.IngestSessionEventInput

// IngestSessionEventResult returns status of ingested event.
type IngestSessionEventResult = usecases.IngestSessionEventResult

// IClassifySessionUseCase triggers async decision extraction on whole sessions.
type IClassifySessionUseCase = usecases.IClassifySessionUseCase

// IngestSessionEventUseCase processes incoming CLI agent telemetry events.
type IngestSessionEventUseCase = usecases.IngestSessionEventUseCase

// NewIngestSessionEventUseCase creates an initialized session event ingestion use case.
var NewIngestSessionEventUseCase = usecases.NewIngestSessionEventUseCase
