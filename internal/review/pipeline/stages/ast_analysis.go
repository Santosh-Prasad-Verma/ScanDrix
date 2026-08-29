package stages

import (
	"context"
	"strings"

	"github.com/scandrix/backend/internal/codeanalysis"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/pipeline"
	"github.com/scandrix/backend/internal/rules"
)

// ASTAnalysisStage executes static rule evaluation and language AST tokenization on changed hunks.
type ASTAnalysisStage struct {
	evaluator *rules.Evaluator
}

func NewASTAnalysisStage(evaluator *rules.Evaluator) *ASTAnalysisStage {
	return &ASTAnalysisStage{evaluator: evaluator}
}

func (s *ASTAnalysisStage) Name() string {
	return "ast_static_analysis"
}

func (s *ASTAnalysisStage) Execute(ctx context.Context, pCtx *pipeline.PipelineContext) error {
	enricher := codeanalysis.NewContextEnricher()

	for _, file := range pCtx.FilteredPatches {
		var addedLines []string
		for _, hunk := range file.Hunks {
			for _, line := range hunk.Lines {
				if line.Type == diff.LineAddition {
					addedLines = append(addedLines, line.Content)
				}
			}
		}
		joinedDiff := strings.Join(addedLines, "\n")
		enriched := enricher.EnrichFilePatch(file, joinedDiff)
		pCtx.EnrichedHunks = append(pCtx.EnrichedHunks, enriched...)
	}

	pCtx.StaticFindings = s.evaluator.EvaluatePatches(pCtx.ReviewID, pCtx.WorkspaceID, pCtx.FilteredPatches)
	pCtx.AllFindings = append(pCtx.AllFindings, pCtx.StaticFindings...)
	return nil
}
