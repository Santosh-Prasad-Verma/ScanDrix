package languages

import (
	"bufio"
	"regexp"
	"strings"
)

// TSFunctionMeta holds metadata about a TypeScript or JavaScript function.
type TSFunctionMeta struct {
	Name      string
	IsAsync   bool
	IsExport  bool
	StartLine int
	EndLine   int
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
	tsFunctionRegex = regexp.MustCompile(`(?:export\s+)?(?:async\s+)?function\s+([a-zA-Z0-9_$]+)`)
	tsArrowRegex    = regexp.MustCompile(`(?:export\s+)?(?:const|let)\s+([a-zA-Z0-9_$]+)\s*=\s*(?:async\s*)?\(`)
)

// AnalyzeTSSource parses a TypeScript/JavaScript source buffer and extracts structural tokens.
func AnalyzeTSSource(content string) *TSFileAnalysis {
	analysis := &TSFileAnalysis{
		Imports:   make([]string, 0),
		Exports:   make([]string, 0),
		Functions: make([]TSFunctionMeta, 0),
		Classes:   make([]string, 0),
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())

		if match := tsImportRegex.FindStringSubmatch(line); len(match) > 1 {
			analysis.Imports = append(analysis.Imports, match[1])
		}

		if match := tsExportRegex.FindStringSubmatch(line); len(match) > 1 {
			analysis.Exports = append(analysis.Exports, match[1])
		}

		if match := tsClassRegex.FindStringSubmatch(line); len(match) > 1 {
			analysis.Classes = append(analysis.Classes, match[1])
		}

		if match := tsFunctionRegex.FindStringSubmatch(line); len(match) > 1 {
			analysis.Functions = append(analysis.Functions, TSFunctionMeta{
				Name:      match[1],
				IsAsync:   strings.Contains(line, "async"),
				IsExport:  strings.Contains(line, "export"),
				StartLine: lineNo,
				EndLine:   lineNo,
			})
		} else if match := tsArrowRegex.FindStringSubmatch(line); len(match) > 1 {
			analysis.Functions = append(analysis.Functions, TSFunctionMeta{
				Name:      match[1],
				IsAsync:   strings.Contains(line, "async"),
				IsExport:  strings.Contains(line, "export"),
				StartLine: lineNo,
				EndLine:   lineNo,
			})
		}
	}

	return analysis
}
