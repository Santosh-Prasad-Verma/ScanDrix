package clireview

import (
	"github.com/scandrix/backend/internal/clireview/application/usecases"
)

// PublicPrReviewInput parameterizes public PR review execution.
type PublicPrReviewInput = usecases.PublicPrReviewInput

// IEnqueueCliReviewUseCase defines async queuing contract for review jobs.
type IEnqueueCliReviewUseCase = usecases.IEnqueueCliReviewUseCase

// PublicPrReviewUseCase orchestrates fetching public PR metadata, diffs, summaries, and queuing.
type PublicPrReviewUseCase = usecases.PublicPrReviewUseCase

// NewPublicPrReviewUseCase initializes public PR review workflow.
var NewPublicPrReviewUseCase = usecases.NewPublicPrReviewUseCase

// ExtractChangedFiles parses git diff headers to collect all touched file paths.
var ExtractChangedFiles = usecases.ExtractChangedFiles
