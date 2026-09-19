package clireview

import (
	"github.com/scandrix/backend/internal/clireview/application/usecases"
)

// SessionTurnPair correlates human prompt with agent response and tooling.
type SessionTurnPair = usecases.SessionTurnPair

// SubagentInfo describes delegated sub-agent invocations.
type SubagentInfo = usecases.SubagentInfo

// AggregatedSession aggregates turn telemetry for holistic evaluation.
type AggregatedSession = usecases.AggregatedSession

// ClassifySessionUseCase transforms session events into structured architectural decisions.
type ClassifySessionUseCase = usecases.ClassifySessionUseCase

// NewClassifySessionUseCase initializes the usecase with repository and LLM client.
var NewClassifySessionUseCase = usecases.NewClassifySessionUseCase
