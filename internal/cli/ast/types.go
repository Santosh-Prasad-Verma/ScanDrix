// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package ast

// Language identifies the programming language of a source file.
type Language string

const (
	LangGo         Language = "go"
	LangTypeScript Language = "typescript"
	LangJavaScript Language = "javascript"
	LangPython     Language = "python"
	LangUnknown    Language = "unknown"
)

// FunctionSymbol represents an extracted function or method in a source file.
type FunctionSymbol struct {
	Name                 string `json:"name"`
	Receiver             string `json:"receiver,omitempty"`
	Signature            string `json:"signature"`
	StartLine            int    `json:"start_line"`
	EndLine              int    `json:"end_line"`
	LinesOfCode          int    `json:"lines_of_code"`
	CyclomaticComplexity int    `json:"cyclomatic_complexity"`
	CognitiveComplexity  int    `json:"cognitive_complexity"`
	NestingDepth         int    `json:"nesting_depth"`
	IsExported           bool   `json:"is_exported"`
}

// CodeSmellType identifies categories of code health and maintainability issues.
type CodeSmellType string

const (
	SmellHighComplexity CodeSmellType = "HIGH_COMPLEXITY"
	SmellDeepNesting    CodeSmellType = "DEEP_NESTING"
	SmellLongFunction   CodeSmellType = "LONG_FUNCTION"
	SmellResourceLeak   CodeSmellType = "RESOURCE_LEAK"
	SmellUnhandledError CodeSmellType = "UNHANDLED_ERROR"
)

// CodeSmell represents a maintainability or design smell discovered through AST analysis.
type CodeSmell struct {
	Type           CodeSmellType `json:"type"`
	FilePath       string        `json:"file_path"`
	FunctionName   string        `json:"function_name,omitempty"`
	StartLine      int           `json:"start_line"`
	EndLine        int           `json:"end_line"`
	Severity       string        `json:"severity"` // warning, error, info
	Description    string        `json:"description"`
	Recommendation string        `json:"recommendation"`
}

// FileAnalysis aggregates AST intelligence, functions, complexity, and smells for a file.
type FileAnalysis struct {
	FilePath        string           `json:"file_path"`
	Language        Language         `json:"language"`
	TotalLines      int              `json:"total_lines"`
	Functions       []FunctionSymbol `json:"functions"`
	AvgComplexity   float64          `json:"avg_complexity"`
	MaxComplexity   int              `json:"max_complexity"`
	Smells          []CodeSmell      `json:"smells"`
}
