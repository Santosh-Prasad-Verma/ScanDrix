package languages

import (
	"regexp"
	"strings"
)

// PyFunctionMeta models a Python function or method definition.
type PyFunctionMeta struct {
	Name      string
	Signature string
	IsAsync   bool
	StartLine int
	EndLine   int
	Calls     []string
}

// PyFileAnalysis models extracted structure of a Python file.
type PyFileAnalysis struct {
	Imports   []string
	Classes   []string
	Functions []PyFunctionMeta
}

var (
	pyImportRegex = regexp.MustCompile(`^(?:from\s+([a-zA-Z0-9_.]+)\s+import|import\s+([a-zA-Z0-9_.]+))`)
	pyClassRegex  = regexp.MustCompile(`^class\s+([a-zA-Z0-9_]+)`)
	pyDefRegex    = regexp.MustCompile(`^(?:async\s+)?def\s+([a-zA-Z0-9_]+)\s*\((.*?)\)`)
	pyCallRegex   = regexp.MustCompile(`(?:([a-zA-Z0-9_]+)\.)?([a-zA-Z0-9_]+)\s*\(`)
)

// AnalyzePySource parses a Python source buffer and extracts structural tokens.
func AnalyzePySource(content string) *PyFileAnalysis {
	analysis := &PyFileAnalysis{
		Imports:   make([]string, 0),
		Classes:   make([]string, 0),
		Functions: make([]PyFunctionMeta, 0),
	}

	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	var currentFunc *PyFunctionMeta
	funcIndent := 0

	for i, rawLine := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		currentIndent := len(rawLine) - len(strings.TrimLeft(rawLine, " \t"))

		// Imports
		if match := pyImportRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			if match[1] != "" {
				analysis.Imports = append(analysis.Imports, match[1])
			} else if match[2] != "" {
				analysis.Imports = append(analysis.Imports, match[2])
			}
		}

		// Classes
		if match := pyClassRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			analysis.Classes = append(analysis.Classes, match[1])
		}

		// Check if current function ended due to dedent
		if currentFunc != nil && currentIndent <= funcIndent && !strings.HasPrefix(trimmed, "def ") && !strings.HasPrefix(trimmed, "async def ") {
			currentFunc.EndLine = lineNo - 1
			analysis.Functions = append(analysis.Functions, *currentFunc)
			currentFunc = nil
		}

		// Function definition
		if match := pyDefRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			if currentFunc != nil {
				currentFunc.EndLine = lineNo - 1
				analysis.Functions = append(analysis.Functions, *currentFunc)
			}

			currentFunc = &PyFunctionMeta{
				Name:      match[1],
				Signature: match[0],
				IsAsync:   strings.HasPrefix(trimmed, "async def"),
				StartLine: lineNo,
				EndLine:   totalLines,
				Calls:     make([]string, 0),
			}
			funcIndent = currentIndent
			continue
		}

		// Inside function body
		if currentFunc != nil {
			callMatches := pyCallRegex.FindAllStringSubmatch(trimmed, -1)
			for _, cm := range callMatches {
				callee := cm[2]
				if callee != "def" && callee != "class" && callee != "if" && callee != "for" && callee != "while" && callee != "with" {
					currentFunc.Calls = append(currentFunc.Calls, callee)
				}
			}
		}
	}

	if currentFunc != nil {
		currentFunc.EndLine = totalLines
		analysis.Functions = append(analysis.Functions, *currentFunc)
	}

	return analysis
}

// FindEnclosingFunction finds the nearest function wrapping the specified line number.
func (a *PyFileAnalysis) FindEnclosingFunction(line int) *PyFunctionMeta {
	for _, fn := range a.Functions {
		if line >= fn.StartLine && line <= fn.EndLine {
			return &fn
		}
	}
	return nil
}
