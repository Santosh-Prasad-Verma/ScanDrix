package clireview

import (
	"github.com/scandrix/backend/internal/clireview/application/usecases"
)

// ExecuteCliReviewUseCase orchestrates the complete end-to-end execution of a CLI code review.
type ExecuteCliReviewUseCase = usecases.ExecuteCliReviewUseCase

// NewExecuteCliReviewUseCase creates an initialized review execution usecase.
var NewExecuteCliReviewUseCase = usecases.NewExecuteCliReviewUseCase

// InMemoryAutomationExecutionService provides an in-memory execution store for testing.
type InMemoryAutomationExecutionService = usecases.InMemoryAutomationExecutionService

// NewInMemoryAutomationExecutionService creates an in-memory execution store.
var NewInMemoryAutomationExecutionService = usecases.NewInMemoryAutomationExecutionService
