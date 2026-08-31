package languages

import (
	"regexp"
	"strings"
)

// RustFunctionMeta models a Rust function definition.
type RustFunctionMeta struct {
	Name      string
	Signature string
	IsAsync   bool
	IsPub     bool
	StartLine int
	EndLine   int
	Calls     []string
}

// RustFileAnalysis models extracted structure of a Rust file.
type RustFileAnalysis struct {
	Uses      []string
	Structs   []string
	Enums     []string
	Traits    []string
	Functions []RustFunctionMeta
}

var (
	rustUseRegex    = regexp.MustCompile(`^use\s+([a-zA-Z0-9_:]+);`)
	rustStructRegex = regexp.MustCompile(`^(?:pub\s+)?struct\s+([a-zA-Z0-9_]+)`)
	rustEnumRegex   = regexp.MustCompile(`^(?:pub\s+)?enum\s+([a-zA-Z0-9_]+)`)
	rustTraitRegex  = regexp.MustCompile(`^(?:pub\s+)?trait\s+([a-zA-Z0-9_]+)`)
	rustFnRegex     = regexp.MustCompile(`^(?:pub\s+)?(?:async\s+)?(?:unsafe\s+)?fn\s+([a-zA-Z0-9_]+)\s*(?:<.*?>)?\s*\((.*?)\)`)
	rustCallRegex   = regexp.MustCompile(`(?:([a-zA-Z0-9_]+)::)?(?:([a-zA-Z0-9_]+)\.)?([a-zA-Z0-9_]+)\s*\(`)
)

// AnalyzeRustSource parses a Rust source buffer and extracts structural tokens.
func AnalyzeRustSource(content string) *RustFileAnalysis {
	analysis := &RustFileAnalysis{
		Uses:      make([]string, 0),
		Structs:   make([]string, 0),
		Enums:     make([]string, 0),
		Traits:    make([]string, 0),
		Functions: make([]RustFunctionMeta, 0),
	}

	lines := strings.Split(content, "\n")
	var currentFn *RustFunctionMeta
	braceDepth := 0
	fnBraceDepth := 0

	for i, rawLine := range lines {
		lineNo := i + 1
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}

		if match := rustUseRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			analysis.Uses = append(analysis.Uses, match[1])
		}

		if match := rustStructRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			analysis.Structs = append(analysis.Structs, match[1])
		}

		if match := rustEnumRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			analysis.Enums = append(analysis.Enums, match[1])
		}

		if match := rustTraitRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			analysis.Traits = append(analysis.Traits, match[1])
		}

		// Function definition
		if match := rustFnRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			if currentFn == nil {
				currentFn = &RustFunctionMeta{
					Name:      match[1],
					Signature: match[0],
					IsAsync:   strings.Contains(trimmed, "async fn"),
					IsPub:     strings.HasPrefix(trimmed, "pub "),
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
			callMatches := rustCallRegex.FindAllStringSubmatch(trimmed, -1)
			for _, cm := range callMatches {
				callee := cm[3]
				if callee != "if" && callee != "for" && callee != "while" && callee != "match" && callee != "loop" && callee != "unsafe" && callee != "Ok" && callee != "Err" && callee != "Some" && callee != currentFn.Name {
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
