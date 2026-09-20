package clireview

import (
	"github.com/scandrix/backend/internal/clireview/application/usecases"
)

// ClassifyCliSessionCaptureUseCase processes atomic IDE/CLI session captures into decisions.
type ClassifyCliSessionCaptureUseCase = usecases.ClassifyCliSessionCaptureUseCase

// NewClassifyCliSessionCaptureUseCase initializes the usecase with repository and LLM client.
var NewClassifyCliSessionCaptureUseCase = usecases.NewClassifyCliSessionCaptureUseCase
