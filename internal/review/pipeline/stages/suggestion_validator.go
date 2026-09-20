package stages

import (
	"context"
	"strings"

	"github.com/scandrix/backend/internal/review/checker"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/review/verifier"
	"github.com/scandrix/backend/pkg/models"
)

// SuggestionValidatorStage dry-runs code replacement proposals through syntax compilers and DiffGate.
type SuggestionValidatorStage struct {
	gate *verifier.DiffGate
}

func NewSuggestionValidatorStage() *SuggestionValidatorStage {
	syntaxValidator := checker.NewASTSyntaxValidator()
	return &SuggestionValidatorStage{
		gate: verifier.NewDiffGate(syntaxValidator),
	}
}

func (s *SuggestionValidatorStage) Name() string {
	return "suggestion_validator"
}

func (s *SuggestionValidatorStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	var validated []pipeline.ValidatedSuggestion
	var alignedFindings []models.CodeFinding

	// Index patches by filepath for rapid lookup
	patchMap := make(map[string]*diff.FilePatch)
	for _, p := range pCtx.ParsedPatches {
		patchMap[p.NewPath] = p
		patchMap[p.OldPath] = p
	}

	for _, f := range pCtx.AllFindings {
		// 1. Align finding coordinates against modified diff hunks
		if p, ok := patchMap[f.FilePath]; ok {
			boundaries := diff.ExtractHunkLineBoundaries(p)
			snappedStart, snappedEnd, withinHunks := diff.SnapFindingCoordinates(f.StartLine, f.EndLine, boundaries)
			if !withinHunks {
				// Finding lies entirely outside changed hunks in the PR diff.
				// Drop it to avoid SCM 422 Unprocessable Entity error when posting inline comments.
				continue
			}
			f.StartLine = snappedStart
			f.EndLine = snappedEnd
		}

		codeSnippet := f.SuggestedDiff
		if codeSnippet == "" {
			codeSnippet = f.Remediation
		}
		if strings.TrimSpace(codeSnippet) == "" {
			alignedFindings = append(alignedFindings, f)
			continue
		}

		// Multi-language syntax & committability verification via DiffGate
		gateRes := s.gate.ValidateFindingDiff(ctx, f, "")
		if gateRes.Decision == verifier.DecisionRejected {
			// Suppress no-op or invalid cosmetic churn
			continue
		}

		alignedFindings = append(alignedFindings, f)

		syntaxValid := false
		if gateRes.SyntaxReport != nil {
			syntaxValid = gateRes.SyntaxReport.IsValid
		}

		validated = append(validated, pipeline.ValidatedSuggestion{
			FindingID:       f.ID,
			FilePath:        f.FilePath,
			StartLine:       f.StartLine,
			EndLine:         f.EndLine,
			SuggestedCode:   codeSnippet,
			SyntaxValid:     syntaxValid,
			IsCommittable:   gateRes.IsCommittable,
			ConfidenceScore: gateRes.ConfidenceScore,
		})
	}

	pCtx.AllFindings = alignedFindings
	pCtx.Suggestions = validated
	return nil
}
