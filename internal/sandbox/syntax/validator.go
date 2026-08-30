package syntax

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// SyntaxValidationResult reports whether a suggested code fix is syntactically valid.
type SyntaxValidationResult struct {
	IsValid      bool   `json:"is_valid"`
	Language     string `json:"language"`
	ErrorMessage string `json:"error_message,omitempty"`
	LineOffset   int    `json:"line_offset,omitempty"`
}

// SandboxSyntaxValidator validates code patches and replacements across multiple languages.
type SandboxSyntaxValidator struct{}

// NewSandboxSyntaxValidator initializes the syntax validator.
func NewSandboxSyntaxValidator() *SandboxSyntaxValidator {
	return &SandboxSyntaxValidator{}
}

// ValidateSuggestion checks whether a proposed code suggestion or whole file content is valid syntax.
func (v *SandboxSyntaxValidator) ValidateSuggestion(filePath, snippet string) SyntaxValidationResult {
	if strings.TrimSpace(snippet) == "" {
		return SyntaxValidationResult{IsValid: true, Language: "empty"}
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return v.validateGo(snippet)
	case ".json":
		return v.validateJSON(snippet)
	case ".js", ".ts", ".jsx", ".tsx":
		return v.validateJS(snippet)
	case ".py":
		return v.validatePython(snippet)
	default:
		// Default generic brace balance validation
		return v.validateGenericBraces(snippet, ext)
	}
}

func (v *SandboxSyntaxValidator) validateGo(snippet string) SyntaxValidationResult {
	fset := token.NewFileSet()

	// 1. Try parsing directly as full source file
	_, err := parser.ParseFile(fset, "snippet.go", snippet, parser.AllErrors)
	if err == nil {
		return SyntaxValidationResult{IsValid: true, Language: "go"}
	}

	// 2. Try wrapping in function body if it's a snippet
	wrapped := fmt.Sprintf("package main\nfunc _testWrap() {\n%s\n}", snippet)
	fsetWrap := token.NewFileSet()
	_, wrapErr := parser.ParseFile(fsetWrap, "snippet.go", wrapped, parser.AllErrors)
	if wrapErr == nil {
		return SyntaxValidationResult{IsValid: true, Language: "go"}
	}

	// 3. Try wrapping as top-level declaration
	wrappedDecl := fmt.Sprintf("package main\n%s", snippet)
	fsetDecl := token.NewFileSet()
	_, declErr := parser.ParseFile(fsetDecl, "snippet.go", wrappedDecl, parser.AllErrors)
	if declErr == nil {
		return SyntaxValidationResult{IsValid: true, Language: "go"}
	}

	return SyntaxValidationResult{
		IsValid:      false,
		Language:     "go",
		ErrorMessage: err.Error(),
	}
}

func (v *SandboxSyntaxValidator) validateJSON(snippet string) SyntaxValidationResult {
	var js any
	if err := json.Unmarshal([]byte(snippet), &js); err != nil {
		return SyntaxValidationResult{
			IsValid:      false,
			Language:     "json",
			ErrorMessage: err.Error(),
		}
	}
	return SyntaxValidationResult{IsValid: true, Language: "json"}
}

func (v *SandboxSyntaxValidator) validateJS(snippet string) SyntaxValidationResult {
	// Check bracket and parenthesis balancing
	return v.validateGenericBraces(snippet, "javascript")
}

func (v *SandboxSyntaxValidator) validatePython(snippet string) SyntaxValidationResult {
	// Check colon and bracket balancing
	return v.validateGenericBraces(snippet, "python")
}

func (v *SandboxSyntaxValidator) validateGenericBraces(snippet, lang string) SyntaxValidationResult {
	stack := make([]rune, 0)
	inString := false
	var stringChar rune
	escaped := false

	for idx, r := range snippet {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}

		if inString {
			if r == stringChar {
				inString = false
			}
			continue
		}

		if r == '"' || r == '\'' || r == '`' {
			inString = true
			stringChar = r
			continue
		}

		if r == '{' || r == '(' || r == '[' {
			stack = append(stack, r)
		} else if r == '}' || r == ')' || r == ']' {
			if len(stack) == 0 {
				return SyntaxValidationResult{
					IsValid:      false,
					Language:     lang,
					ErrorMessage: fmt.Sprintf("unmatched closing brace '%c' at character %d", r, idx),
					LineOffset:   idx,
				}
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			if (r == '}' && top != '{') || (r == ')' && top != '(') || (r == ']' && top != '[') {
				return SyntaxValidationResult{
					IsValid:      false,
					Language:     lang,
					ErrorMessage: fmt.Sprintf("mismatched closing brace '%c' for opening '%c' at character %d", r, top, idx),
					LineOffset:   idx,
				}
			}
		}
	}

	if inString {
		return SyntaxValidationResult{
			IsValid:      false,
			Language:     lang,
			ErrorMessage: "unterminated string literal",
		}
	}

	if len(stack) > 0 {
		return SyntaxValidationResult{
			IsValid:      false,
			Language:     lang,
			ErrorMessage: fmt.Sprintf("unclosed open brace '%c'", stack[len(stack)-1]),
		}
	}

	return SyntaxValidationResult{
		IsValid:  true,
		Language: lang,
	}
}
