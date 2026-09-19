package clireview

import (
	"github.com/scandrix/backend/internal/clireview/application/usecases"
)

// DefaultTraceContextPackTokenBudget caps context decisions injected into review prompts.
const DefaultTraceContextPackTokenBudget = usecases.DefaultTraceContextPackTokenBudget

// BuildTraceContextPackInput defines input to the context-pack compiler.
type BuildTraceContextPackInput = usecases.BuildTraceContextPackInput

// BuildTraceContextPackResult returns selected trace decisions and token stats.
type BuildTraceContextPackResult = usecases.BuildTraceContextPackResult

// BuildTraceContextPackUseCase extracts relevant architecture decisions for a PR or diff.
type BuildTraceContextPackUseCase = usecases.BuildTraceContextPackUseCase

// NewBuildTraceContextPackUseCase creates a new context-pack use case.
var NewBuildTraceContextPackUseCase = usecases.NewBuildTraceContextPackUseCase

// ApplyBudget selects decisions within token limit.
var ApplyBudget = usecases.ApplyBudget

// RenderDecision formats a single decision into markdown.
var RenderDecision = usecases.RenderDecision

// RenderTraceContextPack formats decisions into markdown.
var RenderTraceContextPack = usecases.RenderTraceContextPack

// CompareForPack compares two decisions for prioritization.
var CompareForPack = usecases.CompareForPack

// DedupeDecisions eliminates duplicate decisions.
var DedupeDecisions = usecases.DedupeDecisions

// MatchesAnyPath determines whether a decision matches any file in the PR.
var MatchesAnyPath = usecases.MatchesAnyPath

// PathPrefixes generates all directory prefixes for given paths.
var PathPrefixes = usecases.PathPrefixes

// NormalizePath trims, cleans, and standardizes slash separators.
var NormalizePath = usecases.NormalizePath
