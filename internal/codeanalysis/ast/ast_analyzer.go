package ast

import (
	"path/filepath"
	"strings"
)

// ASTComplexityAnalyzer coordinates multi-language structural complexity and dead code analysis.
type ASTComplexityAnalyzer struct {
	goAnalyzer *GoASTAnalyzer
}

// NewASTComplexityAnalyzer initializes the engine.
func NewASTComplexityAnalyzer() *ASTComplexityAnalyzer {
	return &ASTComplexityAnalyzer{
		goAnalyzer: NewGoASTAnalyzer(),
	}
}

// Analyze evaluates code and returns complexity metrics and findings.
func (a *ASTComplexityAnalyzer) Analyze(filePath, src string) (*ComplexityReport, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".go":
		return a.goAnalyzer.Analyze(filePath, src)
	default:
		// Generic line & Halstead fallback for non-Go sources
		lines := strings.Split(src, "\n")
		return &ComplexityReport{
			FilePath:     filePath,
			Language:     strings.TrimPrefix(ext, "."),
			LinesOfCode:  len(lines),
			Functions:    make([]FunctionComplexity, 0),
			Halstead:     CalculateHalstead(src),
			Findings:     nil,
		}, nil
	}
}
