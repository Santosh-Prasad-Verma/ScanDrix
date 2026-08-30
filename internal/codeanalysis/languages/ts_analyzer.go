package languages

import (
	"regexp"
	"strings"
)

// TSFunctionMeta holds metadata about a TypeScript or JavaScript function.
type TSFunctionMeta struct {
	Name      string
	Signature string
	IsAsync   bool
	IsExport  bool
	StartLine int
	EndLine   int
	Calls     []string
}

// TSFileAnalysis models high-level structure of a TypeScript/JavaScript file.
type TSFileAnalysis struct {
	Imports   []string
	Exports   []string
	Functions []TSFunctionMeta
	Classes   []string
}

var (
	tsImportRegex   = regexp.MustCompile(`import\s+.*?from\s+['"](.*?)['"]`)
	tsExportRegex   = regexp.MustCompile(`export\s+(?:default\s+)?(?:class|function|const|let|var|type|interface)\s+([a-zA-Z0-9_$]+)`)
	tsClassRegex    = regexp.MustCompile(`class\s+([a-zA-Z0-9_$]+)`)
	tsFunctionRegex = regexp.MustCompile(`(?:export\s+)?(?:async\s+)?function\s+([a-zA-Z0-9_$]+)\s*\((.*?)\)`)
	tsArrowRegex    = regexp.MustCompile(`(?:export\s+)?(?:const|let|var)\s+([a-zA-Z0-9_$]+)\s*=\s*(?:async\s*)?\((.*?)\)`)
	tsCallRegex     = regexp.MustCompile(`(?:([a-zA-Z0-9_$]+)\.)?([a-zA-Z0-9_$]+)\s*\(`)
)

// AnalyzeTSSource parses a TypeScript/JavaScript source buffer and extracts structural tokens.
func AnalyzeTSSource(content string) *TSFileAnalysis {
	analysis := &TSFileAnalysis{
		Imports:   make([]string, 0),
		Exports:   make([]string, 0),
		Functions: make([]TSFunctionMeta, 0),
		Classes:   make([]string, 0),
	}

	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	var currentFunc *TSFunctionMeta
	braceDepth := 0

	for i, rawLine := range lines {
		lineNo := i + 1
		line := strings.TrimSpace(rawLine)

		// Imports
		if match := tsImportRegex.FindStringSubmatch(line); len(match) > 1 {
			analysis.Imports = append(analysis.Imports, match[1])
		}

		// Exports
		if match := tsExportRegex.FindStringSubmatch(line); len(match) > 1 {
			analysis.Exports = append(analysis.Exports, match[1])
		}

		// Classes
		if match := tsClassRegex.FindStringSubmatch(line); len(match) > 1 {
			analysis.Classes = append(analysis.Classes, match[1])
		}

		// Track open/close braces for enclosing scope
		openBraces := strings.Count(line, "{")
		closeBraces := strings.Count(line, "}")

		// Function declarations
		if match := tsFunctionRegex.FindStringSubmatch(line); len(match) > 1 {
			if currentFunc != nil {
				currentFunc.EndLine = lineNo - 1
				analysis.Functions = append(analysis.Functions, *currentFunc)
			}
			currentFunc = &TSFunctionMeta{
				Name:      match[1],
				Signature: match[0],
				IsAsync:   strings.Contains(line, "async"),
				IsExport:  strings.Contains(line, "export"),
				StartLine: lineNo,
				EndLine:   totalLines,
				Calls:     make([]string, 0),
			}
			braceDepth = openBraces - closeBraces
			continue
		} else if match := tsArrowRegex.FindStringSubmatch(line); len(match) > 1 {
			if currentFunc != nil {
				currentFunc.EndLine = lineNo - 1
				analysis.Functions = append(analysis.Functions, *currentFunc)
			}
			currentFunc = &TSFunctionMeta{
				Name:      match[1],
				Signature: match[0],
				IsAsync:   strings.Contains(line, "async"),
				IsExport:  strings.Contains(line, "export"),
				StartLine: lineNo,
				EndLine:   totalLines,
				Calls:     make([]string, 0),
			}
			braceDepth = openBraces - closeBraces
			continue
		}

		// Inside function body
		if currentFunc != nil {
			braceDepth += (openBraces - closeBraces)

			// Extract calls
			callMatches := tsCallRegex.FindAllStringSubmatch(line, -1)
			for _, cm := range callMatches {
				callee := cm[2]
				if callee != "function" && callee != "if" && callee != "for" && callee != "switch" && callee != "catch" {
					currentFunc.Calls = append(currentFunc.Calls, callee)
				}
			}

			if braceDepth <= 0 && (openBraces > 0 || closeBraces > 0) {
				currentFunc.EndLine = lineNo
				analysis.Functions = append(analysis.Functions, *currentFunc)
				currentFunc = nil
				braceDepth = 0
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
func (a *TSFileAnalysis) FindEnclosingFunction(line int) *TSFunctionMeta {
	for _, fn := range a.Functions {
		if line >= fn.StartLine && line <= fn.EndLine {
			return &fn
		}
	}
	return nil
}
