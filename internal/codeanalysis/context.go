package codeanalysis

import (
	"fmt"
	"strings"

	"github.com/scandrix/backend/internal/codeanalysis/languages"
	"github.com/scandrix/backend/internal/review/diff"
)

// EnrichedHunk contains a diff hunk augmented with AST context and language metadata.
type EnrichedHunk struct {
	FilePath          string
	Language          languages.Language
	EnclosingFunction string
	Imports           []string
	Hunk              diff.Hunk
}

// ContextEnricher enriches raw diff hunks with enclosing scopes and symbol hierarchies.
type ContextEnricher struct{}

// NewContextEnricher initializes the AST context enricher.
func NewContextEnricher() *ContextEnricher {
	return &ContextEnricher{}
}

// EnrichFilePatch inspects a file patch and enriches its hunks with surrounding structural context.
func (e *ContextEnricher) EnrichFilePatch(patch *diff.FilePatch, fullFileContent string) []EnrichedHunk {
	lang := languages.DetectLanguage(patch.NewPath, fullFileContent)
	results := make([]EnrichedHunk, 0, len(patch.Hunks))

	var goAnalysis *languages.GoFileAnalysis
	var tsAnalysis *languages.TSFileAnalysis
	var pyAnalysis *languages.PyFileAnalysis

	switch lang {
	case languages.LangGo:
		if fullFileContent != "" {
			goAnalysis, _ = languages.AnalyzeGoSource(patch.NewPath, fullFileContent)
		}
	case languages.LangTypeScript, languages.LangJavaScript:
		if fullFileContent != "" {
			tsAnalysis = languages.AnalyzeTSSource(fullFileContent)
		}
	case languages.LangPython:
		if fullFileContent != "" {
			pyAnalysis = languages.AnalyzePySource(fullFileContent)
		}
	}

	for _, hunk := range patch.Hunks {
		enriched := EnrichedHunk{
			FilePath: patch.NewPath,
			Language: lang,
			Hunk:     hunk,
		}

		midLine := hunk.NewStart + (hunk.NewLines / 2)

		if goAnalysis != nil {
			enriched.Imports = goAnalysis.Imports
			if fn := goAnalysis.FindEnclosingFunction(midLine); fn != nil {
				enriched.EnclosingFunction = fn.Name
				if fn.Receiver != "" {
					enriched.EnclosingFunction = fmt.Sprintf("(%s).%s", fn.Receiver, fn.Name)
				}
			}
		} else if tsAnalysis != nil {
			enriched.Imports = tsAnalysis.Imports
			for _, fn := range tsAnalysis.Functions {
				if midLine >= fn.StartLine && (fn.EndLine == 0 || midLine <= fn.EndLine+20) {
					enriched.EnclosingFunction = fn.Name
					break
				}
			}
		} else if pyAnalysis != nil {
			enriched.Imports = pyAnalysis.Imports
			for _, fn := range pyAnalysis.Functions {
				if midLine >= fn.StartLine && (fn.EndLine == 0 || midLine <= fn.EndLine+20) {
					enriched.EnclosingFunction = fn.Name
					break
				}
			}
		}

		// Fallback to diff header if AST wasn't available
		if enriched.EnclosingFunction == "" && hunk.Header != "" {
			enriched.EnclosingFunction = strings.TrimSpace(hunk.Header)
		}

		results = append(results, enriched)
	}

	return results
}
