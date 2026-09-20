package services

import (
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"strings"

	"github.com/scandrix/backend/internal/review/domain"
)

// SandboxSyntaxValidator implements domain.ISandboxSyntaxValidator.
type SandboxSyntaxValidator struct{}

// NewSandboxSyntaxValidator constructs a syntax validator.
func NewSandboxSyntaxValidator() *SandboxSyntaxValidator {
	return &SandboxSyntaxValidator{}
}

// ValidateSyntax inspects modified code snippets for language-specific syntax correctness.
func (v *SandboxSyntaxValidator) ValidateSyntax(ctx context.Context, language, filePath, modifiedContent string) (domain.SyntaxCheckResult, error) {
	if strings.TrimSpace(modifiedContent) == "" {
		return domain.SyntaxCheckResult{
			IsValid:  true,
			Compiler: "empty_check",
		}, nil
	}

	ext := ""
	if idx := strings.LastIndex(filePath, "."); idx != -1 {
		ext = strings.ToLower(filePath[idx:])
	}
	lang := strings.ToLower(language)

	// 1. Go validation via standard library parser
	if lang == "go" || ext == ".go" {
		fset := token.NewFileSet()
		// Try parsing as complete source file
		_, err := parser.ParseFile(fset, filePath, modifiedContent, parser.AllErrors)
		if err != nil {
			// Try wrapping as a function body expression if it's a snippet
			wrapped := fmt.Sprintf("package main\nfunc _dummy() {\n%s\n}", modifiedContent)
			_, wrapErr := parser.ParseFile(fset, filePath, wrapped, parser.AllErrors)
			if wrapErr != nil {
				return domain.SyntaxCheckResult{
					IsValid:      false,
					Compiler:     "go/parser",
					ErrorMessage: err.Error(),
					OutputLines:  strings.Split(err.Error(), "\n"),
				}, nil
			}
		}
		return domain.SyntaxCheckResult{
			IsValid:  true,
			Compiler: "go/parser",
		}, nil
	}

	// 2. JSON validation
	if lang == "json" || ext == ".json" {
		var js interface{}
		err := json.Unmarshal([]byte(modifiedContent), &js)
		if err != nil {
			return domain.SyntaxCheckResult{
				IsValid:      false,
				Compiler:     "encoding/json",
				ErrorMessage: err.Error(),
			}, nil
		}
		return domain.SyntaxCheckResult{
			IsValid:  true,
			Compiler: "encoding/json",
		}, nil
	}

	// 3. Balanced delimiter check for JavaScript, TypeScript, Python, C/C++, Rust
	if err := checkBalancedDelimiters(modifiedContent); err != nil {
		return domain.SyntaxCheckResult{
			IsValid:      false,
			Compiler:     "bracket_verifier",
			ErrorMessage: err.Error(),
		}, nil
	}

	return domain.SyntaxCheckResult{
		IsValid:  true,
		Compiler: "bracket_verifier",
	}, nil
}

// checkBalancedDelimiters verifies matching parentheses, brackets, and braces while ignoring strings and comments.
func checkBalancedDelimiters(code string) error {
	var stack []rune
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inLineComment := false
	escaped := false

	runes := []rune(code)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if inLineComment {
			if r == '\n' {
				inLineComment = false
			}
			continue
		}

		if escaped {
			escaped = false
			continue
		}

		if r == '\\' {
			escaped = true
			continue
		}

		// Comment detection
		if !inSingleQuote && !inDoubleQuote && !inBacktick && i+1 < len(runes) {
			if r == '/' && runes[i+1] == '/' {
				inLineComment = true
				i++
				continue
			}
			if r == '#' {
				inLineComment = true
				continue
			}
		}

		// String detection
		if r == '\'' && !inDoubleQuote && !inBacktick {
			inSingleQuote = !inSingleQuote
			continue
		}
		if r == '"' && !inSingleQuote && !inBacktick {
			inDoubleQuote = !inDoubleQuote
			continue
		}
		if r == '`' && !inSingleQuote && !inDoubleQuote {
			inBacktick = !inBacktick
			continue
		}

		if inSingleQuote || inDoubleQuote || inBacktick {
			continue
		}

		// Brackets
		switch r {
		case '(', '[', '{':
			stack = append(stack, r)
		case ')':
			if len(stack) == 0 || stack[len(stack)-1] != '(' {
				return fmt.Errorf("unmatched closing parenthesis ')'")
			}
			stack = stack[:len(stack)-1]
		case ']':
			if len(stack) == 0 || stack[len(stack)-1] != '[' {
				return fmt.Errorf("unmatched closing bracket ']'")
			}
			stack = stack[:len(stack)-1]
		case '}':
			if len(stack) == 0 || stack[len(stack)-1] != '{' {
				return fmt.Errorf("unmatched closing brace '}'")
			}
			stack = stack[:len(stack)-1]
		}
	}

	if len(stack) > 0 {
		return fmt.Errorf("unclosed delimiter '%c'", stack[len(stack)-1])
	}
	return nil
}

// ValidateSnippet performs a quick syntax validation for code replacement snippets.
func (v *SandboxSyntaxValidator) ValidateSnippet(ctx context.Context, filePath, snippet string) domain.SyntaxCheckResult {
	res, _ := v.ValidateSyntax(ctx, "", filePath, snippet)
	return res
}
