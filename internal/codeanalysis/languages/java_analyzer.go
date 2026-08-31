package languages

import (
	"regexp"
	"strings"
)

// JavaMethodMeta models a Java method definition.
type JavaMethodMeta struct {
	Name       string
	Signature  string
	ReturnVal  string
	StartLine  int
	EndLine    int
	Calls      []string
	Annotation []string
}

// JavaFileAnalysis models extracted structure of a Java file.
type JavaFileAnalysis struct {
	Package    string
	Imports    []string
	Classes    []string
	Interfaces []string
	Methods    []JavaMethodMeta
}

var (
	javaPkgRegex    = regexp.MustCompile(`^package\s+([a-zA-Z0-9_.]+);`)
	javaImportRegex = regexp.MustCompile(`^import\s+(?:static\s+)?([a-zA-Z0-9_.*]+);`)
	javaClassRegex  = regexp.MustCompile(`(?:public|protected|private|abstract|static|final|\s)*\s+(?:class|interface|enum|record)\s+([a-zA-Z0-9_]+)`)
	javaMethodRegex = regexp.MustCompile(`(?:public|protected|private|static|final|synchronized|abstract|default|\s)+\s+([a-zA-Z0-9_<>[\]]+)\s+([a-zA-Z0-9_]+)\s*\((.*?)\)`)
	javaCallRegex   = regexp.MustCompile(`(?:([a-zA-Z0-9_]+)\.)?([a-zA-Z0-9_]+)\s*\(`)
)

// AnalyzeJavaSource parses a Java source buffer and extracts structural tokens.
func AnalyzeJavaSource(content string) *JavaFileAnalysis {
	analysis := &JavaFileAnalysis{
		Imports:    make([]string, 0),
		Classes:    make([]string, 0),
		Interfaces: make([]string, 0),
		Methods:    make([]JavaMethodMeta, 0),
	}

	lines := strings.Split(content, "\n")
	var currentMethod *JavaMethodMeta
	braceDepth := 0
	methodBraceDepth := 0

	for i, rawLine := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}

		if match := javaPkgRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			analysis.Package = match[1]
		}

		if match := javaImportRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			analysis.Imports = append(analysis.Imports, match[1])
		}

		if match := javaClassRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			className := match[1]
			if strings.Contains(trimmed, "interface ") {
				analysis.Interfaces = append(analysis.Interfaces, className)
			} else {
				analysis.Classes = append(analysis.Classes, className)
			}
		}

		// Method definition
		if match := javaMethodRegex.FindStringSubmatch(trimmed); len(match) > 2 && !strings.Contains(trimmed, "new ") && !strings.HasPrefix(trimmed, "return ") {
			if currentMethod == nil && strings.Contains(trimmed, "{") {
				currentMethod = &JavaMethodMeta{
					Name:      match[2],
					ReturnVal: match[1],
					Signature: match[0],
					StartLine: lineNo,
					EndLine:   lineNo,
					Calls:     make([]string, 0),
				}
				methodBraceDepth = braceDepth
			}
		}

		// Count braces
		openCount := strings.Count(trimmed, "{")
		closeCount := strings.Count(trimmed, "}")
		braceDepth += openCount - closeCount

		// Method body calls
		if currentMethod != nil {
			callMatches := javaCallRegex.FindAllStringSubmatch(trimmed, -1)
			for _, cm := range callMatches {
				callee := cm[2]
				if callee != "if" && callee != "for" && callee != "while" && callee != "switch" && callee != "catch" && callee != "synchronized" && callee != currentMethod.Name {
					currentMethod.Calls = append(currentMethod.Calls, callee)
				}
			}

			if braceDepth <= methodBraceDepth && closeCount > 0 {
				currentMethod.EndLine = lineNo
				analysis.Methods = append(analysis.Methods, *currentMethod)
				currentMethod = nil
			}
		}
	}

	if currentMethod != nil {
		currentMethod.EndLine = len(lines)
		analysis.Methods = append(analysis.Methods, *currentMethod)
	}

	return analysis
}
