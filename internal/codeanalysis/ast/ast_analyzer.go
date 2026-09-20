package ast

import (
	"fmt"
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

// MaxSourceFileSize defines the upper memory boundary for single-file AST parsing (2MB).
const MaxSourceFileSize = 2 * 1024 * 1024

// Analyze evaluates code and returns complexity metrics and findings.
func (a *ASTComplexityAnalyzer) Analyze(filePath, src string) (*ComplexityReport, error) {
	if len(src) > MaxSourceFileSize {
		return nil, fmt.Errorf("source file %s exceeds maximum AST analysis size (%d > %d bytes)", filePath, len(src), MaxSourceFileSize)
	}

	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".go":
		return a.goAnalyzer.Analyze(filePath, src)
	default:
		// Generic line & Halstead fallback for non-Go sources
		lines := strings.Split(src, "\n")
		return &ComplexityReport{
			FilePath:    filePath,
			Language:    strings.TrimPrefix(ext, "."),
			LinesOfCode: len(lines),
			Functions:   make([]FunctionComplexity, 0),
			Halstead:    CalculateHalstead(src),
			Findings:    nil,
		}, nil
	}
}
