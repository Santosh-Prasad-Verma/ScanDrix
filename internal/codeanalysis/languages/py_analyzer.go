package languages

import (
	"bufio"
	"regexp"
	"strings"
)

// PyFunctionMeta models a Python function or method definition.
type PyFunctionMeta struct {
	Name      string
	IsAsync   bool
	StartLine int
	EndLine   int
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
	pyDefRegex    = regexp.MustCompile(`^(?:async\s+)?def\s+([a-zA-Z0-9_]+)\s*\(`)
)

// AnalyzePySource parses a Python source buffer and extracts structural tokens.
func AnalyzePySource(content string) *PyFileAnalysis {
	analysis := &PyFileAnalysis{
		Imports:   make([]string, 0),
		Classes:   make([]string, 0),
		Functions: make([]PyFunctionMeta, 0),
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())

		if match := pyImportRegex.FindStringSubmatch(line); len(match) > 1 {
			if match[1] != "" {
				analysis.Imports = append(analysis.Imports, match[1])
			} else if match[2] != "" {
				analysis.Imports = append(analysis.Imports, match[2])
			}
		}

		if match := pyClassRegex.FindStringSubmatch(line); len(match) > 1 {
			analysis.Classes = append(analysis.Classes, match[1])
		}

		if match := pyDefRegex.FindStringSubmatch(line); len(match) > 1 {
			analysis.Functions = append(analysis.Functions, PyFunctionMeta{
				Name:      match[1],
				IsAsync:   strings.HasPrefix(line, "async def"),
				StartLine: lineNo,
				EndLine:   lineNo,
			})
		}
	}

	return analysis
}
