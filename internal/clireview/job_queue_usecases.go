package clireview

import (
	"github.com/scandrix/backend/internal/clireview/application/usecases"
)

// JobQueueItem represents an enqueued CLI review task.
type JobQueueItem = usecases.JobQueueItem

// IJobQueueService abstracts background review task queuing.
type IJobQueueService = usecases.IJobQueueService

// EnqueueCliReviewUseCase pushes a review job to the execution queue.
type EnqueueCliReviewUseCase = usecases.EnqueueCliReviewUseCase

// NewEnqueueCliReviewUseCase creates an initialized enqueue use case.
var NewEnqueueCliReviewUseCase = usecases.NewEnqueueCliReviewUseCase

// WaitForCliReviewJobUseCase polls until a review job completes or times out.
type WaitForCliReviewJobUseCase = usecases.WaitForCliReviewJobUseCase

// NewWaitForCliReviewJobUseCase creates an initialized polling use case.
var NewWaitForCliReviewJobUseCase = usecases.NewWaitForCliReviewJobUseCase

// GetCliReviewJobStatusInput defines parameters for querying job status.
type GetCliReviewJobStatusInput = usecases.GetCliReviewJobStatusInput

// CliReviewJobStatusResponse describes the current progress or outcome of a job.
type CliReviewJobStatusResponse = usecases.CliReviewJobStatusResponse

// GetCliReviewJobStatusUseCase checks the status of a specific review job.
type GetCliReviewJobStatusUseCase = usecases.GetCliReviewJobStatusUseCase

// NewGetCliReviewJobStatusUseCase creates an initialized job status use case.
var NewGetCliReviewJobStatusUseCase = usecases.NewGetCliReviewJobStatusUseCase

// InMemoryJobQueueService provides an in-memory implementation of IJobQueueService.
type InMemoryJobQueueService = usecases.InMemoryJobQueueService

// NewInMemoryJobQueueService instantiates an in-memory queue service for testing.
var NewInMemoryJobQueueService = usecases.NewInMemoryJobQueueService
