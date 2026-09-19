package ast

import (
	"github.com/scandrix/backend/pkg/models"
)

// HalsteadMetrics captures software science metrics according to Maurice Halstead's model.
type HalsteadMetrics struct {
	DistinctOperators int     `json:"distinct_operators"` // eta1
	DistinctOperands  int     `json:"distinct_operands"`  // eta2
	TotalOperators    int     `json:"total_operators"`    // N1
	TotalOperands     int     `json:"total_operands"`     // N2
	ProgramVocabulary int     `json:"program_vocabulary"` // eta = eta1 + eta2
	ProgramLength     int     `json:"program_length"`     // N = N1 + N2
	Volume            float64 `json:"volume"`             // V = N * log2(eta)
	Difficulty        float64 `json:"difficulty"`         // D = (eta1 / 2) * (N2 / eta2)
	Effort            float64 `json:"effort"`             // E = D * V
}

// FunctionComplexity contains AST metrics for a single function or method.
type FunctionComplexity struct {
	Name                 string `json:"name"`
	StartLine            int    `json:"start_line"`
	EndLine              int    `json:"end_line"`
	CyclomaticComplexity int    `json:"cyclomatic_complexity"`
	CognitiveComplexity  int    `json:"cognitive_complexity"`
	ParamCount           int    `json:"param_count"`
	ReturnCount          int    `json:"return_count"`
	MaxNestingDepth      int    `json:"max_nesting_depth"`
}

// ComplexityReport encapsulates complete AST analysis results for a source file.
type ComplexityReport struct {
	FilePath                 string               `json:"file_path"`
	Language                 string               `json:"language"`
	LinesOfCode              int                  `json:"lines_of_code"`
	CommentLines             int                  `json:"comment_lines"`
	BlankLines               int                  `json:"blank_lines"`
	Functions                []FunctionComplexity `json:"functions"`
	FileCyclomaticComplexity int                  `json:"file_cyclomatic_complexity"`
	FileCognitiveComplexity  int                  `json:"file_cognitive_complexity"`
	Halstead                 HalsteadMetrics      `json:"halstead"`
	Findings                 []models.CodeFinding `json:"findings"`
}
