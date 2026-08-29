package stages

import (
	"context"
	"go/parser"
	"go/token"
	"strings"

	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/pkg/models"
)

// SuggestionValidatorStage dry-runs code replacement proposals through syntax compilers.
type SuggestionValidatorStage struct{}

func NewSuggestionValidatorStage() *SuggestionValidatorStage {
	return &SuggestionValidatorStage{}
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
		alignedFindings = append(alignedFindings, f)

		codeSnippet := f.SuggestedDiff
		if codeSnippet == "" {
			codeSnippet = f.Remediation
		}
		if strings.TrimSpace(codeSnippet) == "" {
			continue
		}

		syntaxValid := true
		confidence := 0.85

		// If Go file, validate snippet syntax
		if strings.HasSuffix(f.FilePath, ".go") {
			wrappedSnippet := "package main\n" + codeSnippet
			fset := token.NewFileSet()
			_, err := parser.ParseFile(fset, "", wrappedSnippet, parser.AllErrors)
			if err != nil {
				// Try as function body snippet
				wrappedFunc := "package main\nfunc _test() {\n" + codeSnippet + "\n}"
				_, errFunc := parser.ParseFile(fset, "", wrappedFunc, parser.AllErrors)
				if errFunc != nil {
					syntaxValid = false
					confidence = 0.50
				}
			}
		}

		if syntaxValid {
			confidence = 0.95
		}

		validated = append(validated, pipeline.ValidatedSuggestion{
			FindingID:       f.ID,
			FilePath:        f.FilePath,
			StartLine:       f.StartLine,
			EndLine:         f.EndLine,
			SuggestedCode:   codeSnippet,
			SyntaxValid:     syntaxValid,
			ConfidenceScore: confidence,
		})
	}

	pCtx.AllFindings = alignedFindings
	pCtx.Suggestions = validated
	return nil
}
