package pipeline

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/scandrix/backend/internal/clireview/domain"
)

// CliReviewPipelineContext carries state throughout CLI review pipeline stages.
type CliReviewPipelineContext struct {
	IsFastMode              bool                            `json:"isFastMode"`
	IsTrialMode             bool                            `json:"isTrialMode"`
	StartTime               time.Time                       `json:"startTime"`
	CorrelationID           string                          `json:"correlationId"`
	OrganizationAndTeamData domain.OrganizationAndTeamData  `json:"organizationAndTeamData"`
	CodeReviewConfig        *domain.CodeReviewConfig        `json:"codeReviewConfig"`
	ChangedFiles            []domain.FileChange             `json:"changedFiles"`
	ReviewDirective         string                          `json:"reviewDirective,omitempty"`
	Heavy                   bool                            `json:"heavy"`
	ValidSuggestions        []domain.CliReviewIssue         `json:"validSuggestions"`
	DiscardedSuggestions    []domain.CliReviewIssue         `json:"discardedSuggestions"`
	Repository              domain.RepositoryRef            `json:"repository"`
	Branch                  string                          `json:"branch"`
	PlatformType            string                          `json:"platformType,omitempty"`
	GitContext              *domain.GitContext              `json:"gitContext,omitempty"`
	CliRawDiff              string                          `json:"cliRawDiff,omitempty"`
	PipelineVersion         string                          `json:"pipelineVersion"`
	Errors                  []string                        `json:"errors"`
	CliResponse             *domain.CliReviewResponse       `json:"cliResponse,omitempty"`
}

// IPipelineStage represents a discrete step in pipeline execution.
type IPipelineStage interface {
	StageName() string
	Execute(ctx context.Context, pctx *CliReviewPipelineContext) (*CliReviewPipelineContext, error)
}

// PrepareCliFilesStage prepares and validates file change entries for analysis.
type PrepareCliFilesStage struct {
	logger *slog.Logger
}

// NewPrepareCliFilesStage creates an initialized PrepareCliFilesStage.
func NewPrepareCliFilesStage() *PrepareCliFilesStage {
	return &PrepareCliFilesStage{
		logger: slog.Default().With("stage", "PrepareCliFilesStage"),
	}
}

func (s *PrepareCliFilesStage) StageName() string {
	return "PrepareCliFilesStage"
}

func (s *PrepareCliFilesStage) Execute(ctx context.Context, pctx *CliReviewPipelineContext) (*CliReviewPipelineContext, error) {
	s.logger.Info("Preparing files for CLI review",
		"correlationId", pctx.CorrelationID,
		"filesCount", len(pctx.ChangedFiles),
		"isTrialMode", pctx.IsTrialMode,
	)

	var validFiles []domain.FileChange
	for _, file := range pctx.ChangedFiles {
		if strings.TrimSpace(file.Filename) == "" {
			s.logger.Warn("File missing filename, skipping", "correlationId", pctx.CorrelationID)
			continue
		}

		if strings.TrimSpace(file.Patch) == "" && strings.TrimSpace(file.PatchWithLinesStr) == "" {
			s.logger.Warn("File missing patch data, skipping",
				"correlationId", pctx.CorrelationID,
				"filename", file.Filename,
			)
			continue
		}

		validFiles = append(validFiles, file)
	}

	if len(validFiles) == 0 {
		s.logger.Warn("No valid files to analyze",
			"correlationId", pctx.CorrelationID,
			"originalCount", len(pctx.ChangedFiles),
		)
	} else {
		s.logger.Info("Prepared valid files for analysis",
			"correlationId", pctx.CorrelationID,
			"validFiles", len(validFiles),
			"filteredOut", len(pctx.ChangedFiles)-len(validFiles),
		)
	}

	pctx.ChangedFiles = validFiles
	return pctx, nil
}

// FormatCliOutputStage formats findings into the standard CLI response payload.
type FormatCliOutputStage struct {
	converter *CliInputConverter
	logger    *slog.Logger
}

// NewFormatCliOutputStage creates an initialized FormatCliOutputStage.
func NewFormatCliOutputStage(converter *CliInputConverter) *FormatCliOutputStage {
	if converter == nil {
		converter = NewCliInputConverter()
	}
	return &FormatCliOutputStage{
		converter: converter,
		logger:    slog.Default().With("stage", "FormatCliOutputStage"),
	}
}

func (s *FormatCliOutputStage) StageName() string {
	return "FormatCliOutputStage"
}

func (s *FormatCliOutputStage) Execute(ctx context.Context, pctx *CliReviewPipelineContext) (*CliReviewPipelineContext, error) {
	s.logger.Info("Formatting CLI output findings",
		"correlationId", pctx.CorrelationID,
		"validSuggestionsCount", len(pctx.ValidSuggestions),
		"discardedSuggestionsCount", len(pctx.DiscardedSuggestions),
	)

	var allSuggestions []domain.CodeSuggestion
	for _, issue := range pctx.ValidSuggestions {
		var startIdx, endIdx *int
		var repl string
		if issue.Fix != nil {
			repl = issue.Fix.Replacement
			s := issue.Fix.Range.Start
			e := issue.Fix.Range.End
			startIdx = &s
			endIdx = &e
		}

		allSuggestions = append(allSuggestions, domain.CodeSuggestion{
			RuleID:         issue.RuleID,
			File:           issue.File,
			Line:           issue.Line,
			EndLine:        issue.EndLine,
			Severity:       issue.Severity,
			Category:       issue.Category,
			Message:        issue.Message,
			Suggestion:     issue.Suggestion,
			Recommendation: issue.Recommendation,
			Fixable:        issue.Fixable,
			Replacement:    repl,
			StartIndex:     startIdx,
			EndIndex:       endIdx,
		})
	}

	res := s.converter.ConvertToCliResponse(allSuggestions, len(pctx.ChangedFiles), pctx.StartTime)
	pctx.CliResponse = &res
	return pctx, nil
}

// CliReviewPipelineStrategy defines the sequential ordering of CLI review stages.
type CliReviewPipelineStrategy struct {
	prepareFilesStage *PrepareCliFilesStage
	formatOutputStage *FormatCliOutputStage
}

// NewCliReviewPipelineStrategy initializes the strategy with requisite stages.
func NewCliReviewPipelineStrategy(
	prepareFilesStage *PrepareCliFilesStage,
	formatOutputStage *FormatCliOutputStage,
) *CliReviewPipelineStrategy {
	if prepareFilesStage == nil {
		prepareFilesStage = NewPrepareCliFilesStage()
	}
	if formatOutputStage == nil {
		formatOutputStage = NewFormatCliOutputStage(nil)
	}
	return &CliReviewPipelineStrategy{
		prepareFilesStage: prepareFilesStage,
		formatOutputStage: formatOutputStage,
	}
}

// ConfigureStages provides the pipeline execution slice.
func (s *CliReviewPipelineStrategy) ConfigureStages() []IPipelineStage {
	return []IPipelineStage{
		s.prepareFilesStage,
		s.formatOutputStage,
	}
}

// GetPipelineName returns the identifier for observability.
func (s *CliReviewPipelineStrategy) GetPipelineName() string {
	return "CliReviewPipeline"
}

// PipelineExecutor runs stages sequentially over a pipeline context.
type PipelineExecutor struct {
	logger *slog.Logger
}

// NewPipelineExecutor creates an executor instance.
func NewPipelineExecutor() *PipelineExecutor {
	return &PipelineExecutor{
		logger: slog.Default().With("component", "PipelineExecutor"),
	}
}

// Execute executes all stages in order, passing context forward.
func (pe *PipelineExecutor) Execute(
	ctx context.Context,
	pipelineContext *CliReviewPipelineContext,
	stages []IPipelineStage,
	pipelineName string,
) (*CliReviewPipelineContext, error) {
	current := pipelineContext

	for _, stage := range stages {
		stageStart := time.Now()
		pe.logger.Info("Executing pipeline stage",
			"pipeline", pipelineName,
			"stage", stage.StageName(),
			"correlationId", current.CorrelationID,
		)

		next, err := stage.Execute(ctx, current)
		if err != nil {
			pe.logger.Error("Pipeline stage failed",
				"pipeline", pipelineName,
				"stage", stage.StageName(),
				"error", err,
			)
			current.Errors = append(current.Errors, err.Error())
			return current, err
		}

		pe.logger.Info("Pipeline stage finished",
			"stage", stage.StageName(),
			"elapsedMs", time.Since(stageStart).Milliseconds(),
		)
		current = next
	}

	return current, nil
}
