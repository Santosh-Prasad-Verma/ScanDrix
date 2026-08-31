package languages

import (
	"regexp"
	"strings"
)

// CppFunctionMeta models a C/C++ function or method definition.
type CppFunctionMeta struct {
	Name      string
	Signature string
	ReturnVal string
	StartLine int
	EndLine   int
	Calls     []string
}

// CppFileAnalysis models extracted structure of a C/C++ file.
type CppFileAnalysis struct {
	Includes   []string
	Namespaces []string
	Classes    []string
	Functions  []CppFunctionMeta
}

var (
	cppIncludeRegex   = regexp.MustCompile(`^#include\s+[<"]([a-zA-Z0-9_./\\]+)[>"]`)
	cppNamespaceRegex = regexp.MustCompile(`^namespace\s+([a-zA-Z0-9_]+)`)
	cppClassRegex     = regexp.MustCompile(`^(?:class|struct)\s+([a-zA-Z0-9_]+)`)
	cppFnRegex        = regexp.MustCompile(`^(?:inline|static|virtual|const|constexpr|\s)*\s*([a-zA-Z0-9_:<>[\]*&]+)\s+([a-zA-Z0-9_]+)\s*\((.*?)\)`)
	cppCallRegex      = regexp.MustCompile(`(?:([a-zA-Z0-9_]+)::)?(?:([a-zA-Z0-9_]+)\.)?([a-zA-Z0-9_]+)\s*\(`)
)

// AnalyzeCppSource parses a C/C++ source buffer and extracts structural tokens.
func AnalyzeCppSource(content string) *CppFileAnalysis {
	analysis := &CppFileAnalysis{
		Includes:   make([]string, 0),
		Namespaces: make([]string, 0),
		Classes:    make([]string, 0),
		Functions:  make([]CppFunctionMeta, 0),
	}

	lines := strings.Split(content, "\n")
	var currentFn *CppFunctionMeta
	braceDepth := 0
	fnBraceDepth := 0

	for i, rawLine := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}

		if match := cppIncludeRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			analysis.Includes = append(analysis.Includes, match[1])
		}

		if match := cppNamespaceRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			analysis.Namespaces = append(analysis.Namespaces, match[1])
		}

		if match := cppClassRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			analysis.Classes = append(analysis.Classes, match[1])
		}

		// Function definition
		if match := cppFnRegex.FindStringSubmatch(trimmed); len(match) > 2 && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "return ") {
			if currentFn == nil && (strings.Contains(trimmed, "{") || i < len(lines)-1 && strings.Contains(lines[i+1], "{")) {
				currentFn = &CppFunctionMeta{
					Name:      match[2],
					ReturnVal: match[1],
					Signature: match[0],
					StartLine: lineNo,
					EndLine:   lineNo,
					Calls:     make([]string, 0),
				}
				fnBraceDepth = braceDepth
			}
		}

		// Count braces
		openCount := strings.Count(trimmed, "{")
		closeCount := strings.Count(trimmed, "}")
		braceDepth += openCount - closeCount

		// Function body calls
		if currentFn != nil {
			callMatches := cppCallRegex.FindAllStringSubmatch(trimmed, -1)
			for _, cm := range callMatches {
				callee := cm[3]
				if callee != "if" && callee != "for" && callee != "while" && callee != "switch" && callee != "catch" && callee != "sizeof" && callee != currentFn.Name {
					currentFn.Calls = append(currentFn.Calls, callee)
				}
			}

			if braceDepth <= fnBraceDepth && closeCount > 0 {
				currentFn.EndLine = lineNo
				analysis.Functions = append(analysis.Functions, *currentFn)
				currentFn = nil
			}
		}
	}

	if currentFn != nil {
		currentFn.EndLine = len(lines)
		analysis.Functions = append(analysis.Functions, *currentFn)
	}

	return analysis
}
