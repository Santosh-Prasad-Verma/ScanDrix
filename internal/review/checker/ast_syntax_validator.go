package checker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/scanner"
	"go/token"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

// LanguageKind identifies the programming language or data format for syntax verification.
type LanguageKind string

const (
	LangGo         LanguageKind = "go"
	LangTypeScript LanguageKind = "typescript"
	LangJavaScript LanguageKind = "javascript"
	LangTSX        LanguageKind = "tsx"
	LangJSX        LanguageKind = "jsx"
	LangPython     LanguageKind = "python"
	LangJSON       LanguageKind = "json"
	LangYAML       LanguageKind = "yaml"
	LangRust       LanguageKind = "rust"
	LangSQL        LanguageKind = "sql"
	LangShell      LanguageKind = "shell"
	LangHTML       LanguageKind = "html"
	LangCSS        LanguageKind = "css"
	LangUnknown    LanguageKind = "unknown"
)

// SyntaxDiagnosticSeverity indicates the impact of a syntactic defect.
type SyntaxDiagnosticSeverity string

const (
	SeverityError   SyntaxDiagnosticSeverity = "ERROR"
	SeverityWarning SyntaxDiagnosticSeverity = "WARNING"
)

// SyntaxDiagnostic provides detailed location and causal context for a syntax issue.
type SyntaxDiagnostic struct {
	Severity       SyntaxDiagnosticSeverity `json:"severity"`
	Line           int                      `json:"line"`
	Column         int                      `json:"column"`
	Offset         int                      `json:"offset"`
	RuleID         string                   `json:"rule_id"`
	Message        string                   `json:"message"`
	ContextSnippet string                   `json:"context_snippet,omitempty"`
}

// ASTMetrics captures structural complexity metrics of parsed code.
type ASTMetrics struct {
	StatementCount int `json:"statement_count"`
	TokenCount     int `json:"token_count"`
	MaxNesting     int `json:"max_nesting"`
	FunctionCount  int `json:"function_count"`
	LineCount      int `json:"line_count"`
}

// SyntaxValidationReport summarizes the multi-language validation outcome.
type SyntaxValidationReport struct {
	IsValid       bool               `json:"is_valid"`
	Language      LanguageKind       `json:"language"`
	Compiler      string             `json:"compiler"`
	Diagnostics   []SyntaxDiagnostic `json:"diagnostics,omitempty"`
	ErrorCount    int                `json:"error_count"`
	WarningCount  int                `json:"warning_count"`
	ParseDuration time.Duration      `json:"parse_duration"`
	Metrics       ASTMetrics         `json:"metrics"`
	IsSnippet     bool               `json:"is_snippet"`
	Cached        bool               `json:"cached"`
}

// SyntaxValidationOptions configures parser strictness and snippet wrapping behavior.
type SyntaxValidationOptions struct {
	AllowPartialSnippet    bool `json:"allow_partial_snippet"`
	MaxDiagnostics         int  `json:"max_diagnostics"`
	EnforceJSXBrackets     bool `json:"enforce_jsx_brackets"`
	CheckPythonIndentation bool `json:"check_python_indentation"`
	CheckYAMLIndentation   bool `json:"check_yaml_indentation"`
	Timeout                time.Duration
}

// DefaultSyntaxValidationOptions supplies standard validation settings.
func DefaultSyntaxValidationOptions() SyntaxValidationOptions {
	return SyntaxValidationOptions{
		AllowPartialSnippet:    true,
		MaxDiagnostics:         25,
		EnforceJSXBrackets:     true,
		CheckPythonIndentation: true,
		CheckYAMLIndentation:   true,
		Timeout:                5 * time.Second,
	}
}

// ASTSyntaxValidator evaluates code syntax across multiple languages without external sandbox dependencies.
type ASTSyntaxValidator struct {
	mu           sync.RWMutex
	cache        map[string]SyntaxValidationReport
	maxCacheSize int
	options      SyntaxValidationOptions
}

// NewASTSyntaxValidator creates a multi-language AST syntax validator.
func NewASTSyntaxValidator(opts ...SyntaxValidationOptions) *ASTSyntaxValidator {
	opt := DefaultSyntaxValidationOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}
	return &ASTSyntaxValidator{
		cache:        make(map[string]SyntaxValidationReport),
		maxCacheSize: 5000,
		options:      opt,
	}
}

// DetectLanguage resolves the language from file path extension or explicit hint.
func DetectLanguage(filePath, explicitLang string) LanguageKind {
	if explicitLang != "" {
		normalized := strings.ToLower(strings.TrimSpace(explicitLang))
		switch normalized {
		case "go", "golang":
			return LangGo
		case "ts", "typescript":
			return LangTypeScript
		case "js", "javascript":
			return LangJavaScript
		case "tsx":
			return LangTSX
		case "jsx":
			return LangJSX
		case "py", "python":
			return LangPython
		case "json":
			return LangJSON
		case "yaml", "yml":
			return LangYAML
		case "rs", "rust":
			return LangRust
		case "sql":
			return LangSQL
		case "sh", "bash", "shell", "zsh":
			return LangShell
		case "html", "htm":
			return LangHTML
		case "css", "scss", "sass":
			return LangCSS
		}
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return LangGo
	case ".ts":
		return LangTypeScript
	case ".js", ".mjs", ".cjs":
		return LangJavaScript
	case ".tsx":
		return LangTSX
	case ".jsx":
		return LangJSX
	case ".py", ".pyw":
		return LangPython
	case ".json":
		return LangJSON
	case ".yaml", ".yml":
		return LangYAML
	case ".rs":
		return LangRust
	case ".sql":
		return LangSQL
	case ".sh", ".bash", ".zsh":
		return LangShell
	case ".html", ".htm":
		return LangHTML
	case ".css", ".scss", ".sass":
		return LangCSS
	default:
		base := strings.ToLower(filepath.Base(filePath))
		switch base {
		case "dockerfile":
			return LangShell
		case "gemfile", "vagrantfile":
			return LangPython // Similar delimiter model
		}
		return LangUnknown
	}
}

// ValidateFile checks whether full file contents are syntactically valid.
func (v *ASTSyntaxValidator) ValidateFile(ctx context.Context, filePath, content string, langHint ...string) SyntaxValidationReport {
	hint := ""
	if len(langHint) > 0 {
		hint = langHint[0]
	}
	opts := v.options
	opts.AllowPartialSnippet = false
	return v.validateInternal(ctx, filePath, content, hint, opts)
}

// ValidateSnippet checks whether a patch, hunk, or function replacement snippet is syntactically valid.
func (v *ASTSyntaxValidator) ValidateSnippet(ctx context.Context, filePath, snippet string, langHint ...string) SyntaxValidationReport {
	hint := ""
	if len(langHint) > 0 {
		hint = langHint[0]
	}
	opts := v.options
	opts.AllowPartialSnippet = true
	return v.validateInternal(ctx, filePath, snippet, hint, opts)
}

func (v *ASTSyntaxValidator) validateInternal(
	ctx context.Context,
	filePath, code, langHint string,
	opts SyntaxValidationOptions,
) SyntaxValidationReport {
	start := time.Now()

	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return SyntaxValidationReport{
			IsValid:       true,
			Language:      LangUnknown,
			Compiler:      "empty_check",
			ParseDuration: time.Since(start),
			Metrics:       ASTMetrics{LineCount: 0},
		}
	}

	lang := DetectLanguage(filePath, langHint)

	// Cache lookup
	cacheKey := v.computeCacheKey(filePath, code, lang, opts.AllowPartialSnippet)
	v.mu.RLock()
	if cached, ok := v.cache[cacheKey]; ok {
		v.mu.RUnlock()
		cached.Cached = true
		return cached
	}
	v.mu.RUnlock()

	var report SyntaxValidationReport
	switch lang {
	case LangGo:
		report = v.validateGo(filePath, code, opts)
	case LangJSON:
		report = v.validateJSON(code, opts)
	case LangYAML:
		report = v.validateYAML(code, opts)
	case LangPython:
		report = v.validatePython(code, opts)
	case LangTypeScript, LangJavaScript, LangTSX, LangJSX:
		report = v.validateJavaScriptFamily(code, lang, opts)
	case LangRust:
		report = v.validateRust(code, opts)
	case LangSQL:
		report = v.validateSQL(code, opts)
	case LangShell:
		report = v.validateShell(code, opts)
	case LangHTML:
		report = v.validateHTML(code, opts)
	case LangCSS:
		report = v.validateCSS(code, opts)
	default:
		report = v.validateGenericDelimiters(code, lang, opts)
	}

	report.Language = lang
	report.ParseDuration = time.Since(start)
	report.IsSnippet = opts.AllowPartialSnippet

	// Populate error & warning counts
	for _, d := range report.Diagnostics {
		if d.Severity == SeverityError {
			report.ErrorCount++
		} else {
			report.WarningCount++
		}
	}
	if report.ErrorCount > 0 {
		report.IsValid = false
	}

	// Cache store
	v.mu.Lock()
	if len(v.cache) >= v.maxCacheSize {
		// Evict half
		count := 0
		for k := range v.cache {
			delete(v.cache, k)
			count++
			if count >= v.maxCacheSize/2 {
				break
			}
		}
	}
	v.cache[cacheKey] = report
	v.mu.Unlock()

	return report
}

func (v *ASTSyntaxValidator) computeCacheKey(filePath, code string, lang LanguageKind, isSnippet bool) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%s:%s:%t:", filePath, lang, isSnippet)))
	h.Write([]byte(code))
	return hex.EncodeToString(h.Sum(nil))
}

// -----------------------------------------------------------------------------
// Go AST Syntax Parser
// -----------------------------------------------------------------------------

func (v *ASTSyntaxValidator) validateGo(filePath, code string, opts SyntaxValidationOptions) SyntaxValidationReport {
	fset := token.NewFileSet()
	lines := strings.Split(code, "\n")
	metrics := ASTMetrics{LineCount: len(lines)}

	// 1. Try parsing directly as full Go file
	_, err := parser.ParseFile(fset, filePath, code, parser.AllErrors)
	if err == nil {
		metrics.TokenCount = countGoTokens(fset, code)
		return SyntaxValidationReport{
			IsValid:  true,
			Compiler: "go/parser",
			Metrics:  metrics,
		}
	}

	// If not allowed to be a snippet, reject immediately
	if !opts.AllowPartialSnippet {
		return formatGoErrors(err, lines, metrics)
	}

	// 2. Try wrapping as a function body
	wrappedFunc := fmt.Sprintf("package main\nfunc _scandrixValidate() {\n%s\n}", code)
	fsetFunc := token.NewFileSet()
	_, wrapErr := parser.ParseFile(fsetFunc, filePath, wrappedFunc, parser.AllErrors)
	if wrapErr == nil {
		metrics.TokenCount = countGoTokens(fsetFunc, code)
		return SyntaxValidationReport{
			IsValid:  true,
			Compiler: "go/parser:wrapped_func",
			Metrics:  metrics,
		}
	}

	// 3. Try wrapping as top-level declarations (types, vars, consts, funcs)
	wrappedDecl := fmt.Sprintf("package main\n%s\n", code)
	fsetDecl := token.NewFileSet()
	_, declErr := parser.ParseFile(fsetDecl, filePath, wrappedDecl, parser.AllErrors)
	if declErr == nil {
		metrics.TokenCount = countGoTokens(fsetDecl, code)
		return SyntaxValidationReport{
			IsValid:  true,
			Compiler: "go/parser:wrapped_decl",
			Metrics:  metrics,
		}
	}

	// 4. Try parsing as a standalone expression
	_, exprErr := parser.ParseExpr(code)
	if exprErr == nil {
		return SyntaxValidationReport{
			IsValid:  true,
			Compiler: "go/parser:expr",
			Metrics:  metrics,
		}
	}

	// Return formatted Go syntax errors from the best attempt
	return formatGoErrors(wrapErr, lines, metrics)
}

func countGoTokens(fset *token.FileSet, code string) int {
	var s scanner.Scanner
	file := fset.AddFile("count.go", fset.Base(), len(code))
	s.Init(file, []byte(code), nil, 0)
	count := 0
	for {
		_, tok, _ := s.Scan()
		if tok == token.EOF {
			break
		}
		count++
	}
	return count
}

func formatGoErrors(err error, lines []string, metrics ASTMetrics) SyntaxValidationReport {
	var diags []SyntaxDiagnostic

	if scannerList, ok := err.(scanner.ErrorList); ok {
		for _, se := range scannerList {
			lineIdx := se.Pos.Line - 1
			// Adjust line if wrapped
			adjustedLine := se.Pos.Line
			if adjustedLine > 2 && len(lines) > 0 {
				adjustedLine -= 2
			}
			snippet := ""
			if lineIdx >= 0 && lineIdx < len(lines) {
				snippet = strings.TrimSpace(lines[lineIdx])
			}
			diags = append(diags, SyntaxDiagnostic{
				Severity:       SeverityError,
				Line:           adjustedLine,
				Column:         se.Pos.Column,
				RuleID:         "go/syntax",
				Message:        se.Msg,
				ContextSnippet: snippet,
			})
		}
	} else if err != nil {
		diags = append(diags, SyntaxDiagnostic{
			Severity: SeverityError,
			Line:     1,
			Column:   1,
			RuleID:   "go/syntax",
			Message:  err.Error(),
		})
	}

	return SyntaxValidationReport{
		IsValid:     false,
		Compiler:    "go/parser",
		Diagnostics: diags,
		Metrics:     metrics,
	}
}

// -----------------------------------------------------------------------------
// JSON Syntax Validator
// -----------------------------------------------------------------------------

func (v *ASTSyntaxValidator) validateJSON(code string, opts SyntaxValidationOptions) SyntaxValidationReport {
	lines := strings.Split(code, "\n")
	metrics := ASTMetrics{LineCount: len(lines)}

	var js any
	dec := json.NewDecoder(strings.NewReader(code))
	dec.DisallowUnknownFields()
	err := dec.Decode(&js)
	if err == nil {
		return SyntaxValidationReport{
			IsValid:  true,
			Compiler: "encoding/json",
			Metrics:  metrics,
		}
	}

	// Extract line and column from SyntaxError
	var diags []SyntaxDiagnostic
	if synErr, ok := err.(*json.SyntaxError); ok {
		line, col := offsetToLineCol(code, int(synErr.Offset))
		snippet := ""
		if line-1 < len(lines) && line-1 >= 0 {
			snippet = strings.TrimSpace(lines[line-1])
		}
		diags = append(diags, SyntaxDiagnostic{
			Severity:       SeverityError,
			Line:           line,
			Column:         col,
			Offset:         int(synErr.Offset),
			RuleID:         "json/syntax",
			Message:        synErr.Error(),
			ContextSnippet: snippet,
		})
	} else {
		diags = append(diags, SyntaxDiagnostic{
			Severity: SeverityError,
			Line:     1,
			Column:   1,
			RuleID:   "json/syntax",
			Message:  err.Error(),
		})
	}

	return SyntaxValidationReport{
		IsValid:     false,
		Compiler:    "encoding/json",
		Diagnostics: diags,
		Metrics:     metrics,
	}
}

// -----------------------------------------------------------------------------
// YAML Syntax Validator (Indent, Tabs, Colon-Space, Scalars)
// -----------------------------------------------------------------------------

func (v *ASTSyntaxValidator) validateYAML(code string, opts SyntaxValidationOptions) SyntaxValidationReport {
	lines := strings.Split(code, "\n")
	metrics := ASTMetrics{LineCount: len(lines)}
	var diags []SyntaxDiagnostic

	indentStack := []int{0}
	inMultilineScalar := false
	scalarIndent := 0

	for idx, rawLine := range lines {
		lineNum := idx + 1
		trimmed := strings.TrimRight(rawLine, " \r\t")
		if strings.TrimSpace(trimmed) == "" || strings.HasPrefix(strings.TrimSpace(trimmed), "#") {
			continue
		}

		// Check for tabs in indentation
		leadingSpaces := 0
		hasTab := false
		for _, ch := range rawLine {
			if ch == ' ' {
				leadingSpaces++
			} else if ch == '\t' {
				hasTab = true
				break
			} else {
				break
			}
		}

		if hasTab {
			diags = append(diags, SyntaxDiagnostic{
				Severity:       SeverityError,
				Line:           lineNum,
				Column:         leadingSpaces + 1,
				RuleID:         "yaml/no_tabs",
				Message:        "tab characters are forbidden for YAML indentation; use spaces",
				ContextSnippet: rawLine,
			})
			continue
		}

		if inMultilineScalar {
			if leadingSpaces > scalarIndent {
				continue
			}
			inMultilineScalar = false
		}

		// Check scalar block indicator (| or >)
		trimmedLine := strings.TrimSpace(trimmed)
		if strings.HasSuffix(trimmedLine, "|") || strings.HasSuffix(trimmedLine, ">") ||
			strings.HasSuffix(trimmedLine, "|-") || strings.HasSuffix(trimmedLine, ">-") {
			inMultilineScalar = true
			scalarIndent = leadingSpaces
		}

		// Check colon spacing rule: "key: value" or "key:" at end of line
		// A colon without following whitespace or newline is an invalid mapping key (unless quoted)
		if !inMultilineScalar && strings.Contains(trimmedLine, ":") {
			if errDiag := checkYAMLColonSpacing(trimmedLine, rawLine, lineNum); errDiag != nil {
				diags = append(diags, *errDiag)
			}
		}

		// Check indent stack consistency
		if opts.CheckYAMLIndentation {
			currIndent := leadingSpaces
			lastIndent := indentStack[len(indentStack)-1]
			if currIndent > lastIndent {
				indentStack = append(indentStack, currIndent)
			} else if currIndent < lastIndent {
				// Pop until match or error
				matched := false
				for len(indentStack) > 1 {
					indentStack = indentStack[:len(indentStack)-1]
					if indentStack[len(indentStack)-1] == currIndent {
						matched = true
						break
					}
				}
				if !matched && currIndent != 0 {
					diags = append(diags, SyntaxDiagnostic{
						Severity:       SeverityError,
						Line:           lineNum,
						Column:         currIndent + 1,
						RuleID:         "yaml/indentation_mismatch",
						Message:        fmt.Sprintf("inconsistent indentation level %d; does not match any outer block", currIndent),
						ContextSnippet: rawLine,
					})
				}
			}
		}

		if len(diags) >= opts.MaxDiagnostics {
			break
		}
	}

	isValid := len(diags) == 0
	return SyntaxValidationReport{
		IsValid:     isValid,
		Compiler:    "yaml/structural_validator",
		Diagnostics: diags,
		Metrics:     metrics,
	}
}

func checkYAMLColonSpacing(line, rawLine string, lineNum int) *SyntaxDiagnostic {
	inQuote := false
	var quoteChar rune
	runes := []rune(line)

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if (r == '"' || r == '\'') && (i == 0 || runes[i-1] != '\\') {
			if !inQuote {
				inQuote = true
				quoteChar = r
			} else if quoteChar == r {
				inQuote = false
			}
			continue
		}

		if !inQuote && r == ':' {
			// If colon is not the last character, next char MUST be whitespace
			if i+1 < len(runes) {
				next := runes[i+1]
				if next != ' ' && next != '\t' && next != '\n' && next != '\r' {
					// Exception: URL schemes like http:// or https://
					if (i > 3 && string(runes[i-4:i]) == "http") || (i > 4 && string(runes[i-5:i]) == "https") {
						continue
					}
					return &SyntaxDiagnostic{
						Severity:       SeverityError,
						Line:           lineNum,
						Column:         i + 1,
						RuleID:         "yaml/colon_spacing",
						Message:        "mapping values must be separated from keys by whitespace after ':'",
						ContextSnippet: rawLine,
					}
				}
			}
		}
	}
	return nil
}

// -----------------------------------------------------------------------------
// Python Syntax Validator (Indentation, Colons, Brackets, Multiline Quotes)
// -----------------------------------------------------------------------------

func (v *ASTSyntaxValidator) validatePython(code string, opts SyntaxValidationOptions) SyntaxValidationReport {
	lines := strings.Split(code, "\n")
	metrics := ASTMetrics{LineCount: len(lines)}
	var diags []SyntaxDiagnostic

	indentStack := []int{0}
	bracketStack := []rune{}
	inTripleSingle := false
	inTripleDouble := false

	compoundKeywords := []string{
		"def ", "class ", "if ", "elif ", "else:", "for ", "while ", "try:", "except ", "except:",
		"finally:", "with ", "async def ", "async with ", "async for ",
	}

	for idx, rawLine := range lines {
		lineNum := idx + 1
		trimmed := strings.TrimSpace(rawLine)

		// Check triple quotes
		handleTripleQuotes(rawLine, &inTripleSingle, &inTripleDouble)
		if inTripleSingle || inTripleDouble {
			continue
		}

		// Ignore empty lines and comment lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Count leading spaces
		leadingSpaces := 0
		hasTab := false
		for _, ch := range rawLine {
			if ch == ' ' {
				leadingSpaces++
			} else if ch == '\t' {
				hasTab = true
				break
			} else {
				break
			}
		}

		if hasTab {
			diags = append(diags, SyntaxDiagnostic{
				Severity:       SeverityError,
				Line:           lineNum,
				Column:         leadingSpaces + 1,
				RuleID:         "python/no_tabs",
				Message:        "tabs and spaces must not be mixed in Python indentation",
				ContextSnippet: rawLine,
			})
			continue
		}

		// Bracket balance for current line
		scanBrackets(rawLine, &bracketStack)

		// Indentation check is only strictly enforced outside brackets
		if len(bracketStack) == 0 && opts.CheckPythonIndentation {
			currIndent := leadingSpaces
			lastIndent := indentStack[len(indentStack)-1]

			if currIndent > lastIndent {
				// Must be an indentation jump
				indentStack = append(indentStack, currIndent)
			} else if currIndent < lastIndent {
				// Dedent: must align with an existing indent level
				found := false
				for len(indentStack) > 1 {
					indentStack = indentStack[:len(indentStack)-1]
					if indentStack[len(indentStack)-1] == currIndent {
						found = true
						break
					}
				}
				if !found && currIndent != 0 {
					diags = append(diags, SyntaxDiagnostic{
						Severity:       SeverityError,
						Line:           lineNum,
						Column:         currIndent + 1,
						RuleID:         "python/unindent_mismatch",
						Message:        fmt.Sprintf("unindent does not match any outer indentation level (%d spaces)", currIndent),
						ContextSnippet: rawLine,
					})
				}
			}
		}

		// Compound statement colon check
		if !strings.HasSuffix(trimmed, "\\") && len(bracketStack) == 0 {
			for _, kw := range compoundKeywords {
				cleanKw := strings.TrimSpace(kw)
				if strings.HasPrefix(trimmed, cleanKw) {
					// Ensure it ends with ':' (allowing trailing comment)
					effective := trimmed
					if cIdx := strings.Index(effective, "#"); cIdx != -1 {
						effective = strings.TrimSpace(effective[:cIdx])
					}
					if !strings.HasSuffix(effective, ":") {
						diags = append(diags, SyntaxDiagnostic{
							Severity:       SeverityError,
							Line:           lineNum,
							Column:         len(rawLine),
							RuleID:         "python/expected_colon",
							Message:        fmt.Sprintf("compound statement '%s' must terminate with a colon ':'", cleanKw),
							ContextSnippet: rawLine,
						})
					}
					break
				}
			}
		}

		if len(diags) >= opts.MaxDiagnostics {
			break
		}
	}

	// Unclosed delimiters
	if inTripleSingle || inTripleDouble {
		diags = append(diags, SyntaxDiagnostic{
			Severity: SeverityError,
			Line:     len(lines),
			Column:   1,
			RuleID:   "python/unterminated_string",
			Message:  "unterminated triple-quoted string literal at end of file",
		})
	}
	if len(bracketStack) > 0 {
		diags = append(diags, SyntaxDiagnostic{
			Severity: SeverityError,
			Line:     len(lines),
			Column:   1,
			RuleID:   "python/unclosed_bracket",
			Message:  fmt.Sprintf("unclosed delimiter '%c' at end of block", bracketStack[len(bracketStack)-1]),
		})
	}

	return SyntaxValidationReport{
		IsValid:     len(diags) == 0,
		Compiler:    "python/syntax_checker",
		Diagnostics: diags,
		Metrics:     metrics,
	}
}

func handleTripleQuotes(line string, inSingle, inDouble *bool) {
	if strings.Contains(line, "'''") {
		count := strings.Count(line, "'''")
		if count%2 != 0 {
			*inSingle = !*inSingle
		}
	}
	if strings.Contains(line, `"""`) {
		count := strings.Count(line, `"""`)
		if count%2 != 0 {
			*inDouble = !*inDouble
		}
	}
}

func scanBrackets(line string, stack *[]rune) {
	inStr := false
	var quote rune
	runes := []rune(line)

	for i := 0; i < len(runes); i++ {
		r := runes[i]

		// Skip comments
		if !inStr && r == '#' {
			break
		}

		// String toggle
		if (r == '\'' || r == '"') && (i == 0 || runes[i-1] != '\\') {
			if !inStr {
				inStr = true
				quote = r
			} else if quote == r {
				inStr = false
			}
			continue
		}

		if inStr {
			continue
		}

		switch r {
		case '(', '[', '{':
			*stack = append(*stack, r)
		case ')':
			if len(*stack) > 0 && (*stack)[len(*stack)-1] == '(' {
				*stack = (*stack)[:len(*stack)-1]
			}
		case ']':
			if len(*stack) > 0 && (*stack)[len(*stack)-1] == '[' {
				*stack = (*stack)[:len(*stack)-1]
			}
		case '}':
			if len(*stack) > 0 && (*stack)[len(*stack)-1] == '{' {
				*stack = (*stack)[:len(*stack)-1]
			}
		}
	}
}

// -----------------------------------------------------------------------------
// JavaScript / TypeScript / JSX / TSX AST Syntax Validator
// -----------------------------------------------------------------------------

func (v *ASTSyntaxValidator) validateJavaScriptFamily(code string, lang LanguageKind, opts SyntaxValidationOptions) SyntaxValidationReport {
	lines := strings.Split(code, "\n")
	metrics := ASTMetrics{LineCount: len(lines)}
	var diags []SyntaxDiagnostic

	var bracketStack []rune
	var bracketPositions []struct {
		line int
		col  int
		r    rune
	}

	isJSX := (lang == LangJSX || lang == LangTSX || strings.Contains(code, "</") || strings.Contains(code, "/>"))
	var jsxTagStack []string

	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	templateExprDepth := 0
	inLineComment := false
	inBlockComment := false
	escaped := false

	runes := []rune(code)
	curLine := 1
	curCol := 0

	for i := 0; i < len(runes); i++ {
		curCol++
		r := runes[i]

		if r == '\n' {
			curLine++
			curCol = 0
			inLineComment = false
			continue
		}

		if inLineComment {
			continue
		}

		if inBlockComment {
			if r == '*' && i+1 < len(runes) && runes[i+1] == '/' {
				inBlockComment = false
				i++
				curCol++
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

		// Comment start
		if !inSingleQuote && !inDoubleQuote && !inBacktick && i+1 < len(runes) {
			if r == '/' && runes[i+1] == '/' {
				inLineComment = true
				i++
				curCol++
				continue
			}
			if r == '/' && runes[i+1] == '*' {
				inBlockComment = true
				i++
				curCol++
				continue
			}
		}

		// Strings
		if r == '\'' && !inDoubleQuote && !inBacktick {
			inSingleQuote = !inSingleQuote
			continue
		}
		if r == '"' && !inSingleQuote && !inBacktick {
			inDoubleQuote = !inDoubleQuote
			continue
		}

		// Template literals: `hello ${name}`
		if r == '`' && !inSingleQuote && !inDoubleQuote {
			inBacktick = !inBacktick
			continue
		}

		// Inside template string, detect ${ ... } interpolation
		if inBacktick && r == '$' && i+1 < len(runes) && runes[i+1] == '{' {
			templateExprDepth++
			bracketStack = append(bracketStack, '{')
			bracketPositions = append(bracketPositions, struct {
				line int
				col  int
				r    rune
			}{line: curLine, col: curCol, r: '{'})
			i++
			curCol++
			continue
		}

		if inSingleQuote || inDoubleQuote || (inBacktick && templateExprDepth == 0) {
			continue
		}

		// JSX element parsing
		if isJSX && opts.EnforceJSXBrackets && r == '<' && !inSingleQuote && !inDoubleQuote && !inBacktick {
			// Check if it's a JSX tag rather than a comparison (e.g. `i < 10`)
			tag, isClosing, isSelfClosing, adv := parseJSXTagAt(runes, i)
			if tag != "" {
				if isClosing {
					if len(jsxTagStack) == 0 {
						diags = append(diags, SyntaxDiagnostic{
							Severity:       SeverityError,
							Line:           curLine,
							Column:         curCol,
							RuleID:         "jsx/unmatched_closing_tag",
							Message:        fmt.Sprintf("unexpected closing JSX tag '</%s>' with no opening tag", tag),
							ContextSnippet: getLineSnippet(lines, curLine),
						})
					} else {
						topTag := jsxTagStack[len(jsxTagStack)-1]
						if topTag != tag && tag != "" { // tag == "" for fragment </>
							diags = append(diags, SyntaxDiagnostic{
								Severity:       SeverityError,
								Line:           curLine,
								Column:         curCol,
								RuleID:         "jsx/mismatched_tag",
								Message:        fmt.Sprintf("closing JSX tag '</%s>' does not match opening '<%s>'", tag, topTag),
								ContextSnippet: getLineSnippet(lines, curLine),
							})
						}
						jsxTagStack = jsxTagStack[:len(jsxTagStack)-1]
					}
				} else if !isSelfClosing && !isVoidHTMLTag(tag) {
					jsxTagStack = append(jsxTagStack, tag)
				}
				i += adv
				curCol += adv
				continue
			}
		}

		// Brackets
		switch r {
		case '(', '[', '{':
			bracketStack = append(bracketStack, r)
			bracketPositions = append(bracketPositions, struct {
				line int
				col  int
				r    rune
			}{line: curLine, col: curCol, r: r})
		case ')':
			if len(bracketStack) == 0 || bracketStack[len(bracketStack)-1] != '(' {
				diags = append(diags, SyntaxDiagnostic{
					Severity:       SeverityError,
					Line:           curLine,
					Column:         curCol,
					RuleID:         "js/unmatched_closing_paren",
					Message:        "unmatched closing parenthesis ')'",
					ContextSnippet: getLineSnippet(lines, curLine),
				})
			} else {
				bracketStack = bracketStack[:len(bracketStack)-1]
				bracketPositions = bracketPositions[:len(bracketPositions)-1]
			}
		case ']':
			if len(bracketStack) == 0 || bracketStack[len(bracketStack)-1] != '[' {
				diags = append(diags, SyntaxDiagnostic{
					Severity:       SeverityError,
					Line:           curLine,
					Column:         curCol,
					RuleID:         "js/unmatched_closing_bracket",
					Message:        "unmatched closing square bracket ']'",
					ContextSnippet: getLineSnippet(lines, curLine),
				})
			} else {
				bracketStack = bracketStack[:len(bracketStack)-1]
				bracketPositions = bracketPositions[:len(bracketPositions)-1]
			}
		case '}':
			if len(bracketStack) == 0 || bracketStack[len(bracketStack)-1] != '{' {
				diags = append(diags, SyntaxDiagnostic{
					Severity:       SeverityError,
					Line:           curLine,
					Column:         curCol,
					RuleID:         "js/unmatched_closing_brace",
					Message:        "unmatched closing curly brace '}'",
					ContextSnippet: getLineSnippet(lines, curLine),
				})
			} else {
				bracketStack = bracketStack[:len(bracketStack)-1]
				bracketPositions = bracketPositions[:len(bracketPositions)-1]
				if templateExprDepth > 0 {
					templateExprDepth--
				}
			}
		}

		if len(diags) >= opts.MaxDiagnostics {
			break
		}
	}

	if inSingleQuote || inDoubleQuote {
		diags = append(diags, SyntaxDiagnostic{
			Severity: SeverityError,
			Line:     curLine,
			Column:   curCol,
			RuleID:   "js/unterminated_string",
			Message:  "unterminated string literal at end of code",
		})
	}
	if inBacktick {
		diags = append(diags, SyntaxDiagnostic{
			Severity: SeverityError,
			Line:     curLine,
			Column:   curCol,
			RuleID:   "js/unterminated_template",
			Message:  "unterminated template literal (`) at end of code",
		})
	}
	if len(bracketStack) > 0 {
		topPos := bracketPositions[len(bracketPositions)-1]
		diags = append(diags, SyntaxDiagnostic{
			Severity:       SeverityError,
			Line:           topPos.line,
			Column:         topPos.col,
			RuleID:         "js/unclosed_delimiter",
			Message:        fmt.Sprintf("unclosed opening delimiter '%c'", topPos.r),
			ContextSnippet: getLineSnippet(lines, topPos.line),
		})
	}
	if isJSX && len(jsxTagStack) > 0 {
		topTag := jsxTagStack[len(jsxTagStack)-1]
		diags = append(diags, SyntaxDiagnostic{
			Severity: SeverityError,
			Line:     curLine,
			Column:   curCol,
			RuleID:   "jsx/unclosed_tag",
			Message:  fmt.Sprintf("unclosed JSX element '<%s>' at end of code", topTag),
		})
	}

	return SyntaxValidationReport{
		IsValid:     len(diags) == 0,
		Compiler:    fmt.Sprintf("%s/ast_validator", lang),
		Diagnostics: diags,
		Metrics:     metrics,
	}
}

func parseJSXTagAt(runes []rune, idx int) (tag string, isClosing bool, isSelfClosing bool, advance int) {
	if idx+1 >= len(runes) {
		return "", false, false, 0
	}

	curr := idx + 1
	if runes[curr] == '/' {
		isClosing = true
		curr++
	}

	// For opening tags, if directly preceded by an identifier character or dot (e.g. useState<number> or React.FC<Props>),
	// this is a TypeScript generic parameter or function call, not a JSX tag.
	if !isClosing && idx > 0 {
		prev := runes[idx-1]
		if prev == '.' || unicode.IsLetter(prev) || unicode.IsDigit(prev) || prev == '_' || prev == '$' {
			return "", false, false, 0
		}
	}

	// Fragment check: <> or </>
	if curr < len(runes) && runes[curr] == '>' {
		return "fragment", isClosing, false, curr - idx
	}

	// First character of tag name must be letter or underscore
	if curr >= len(runes) || (!unicode.IsLetter(runes[curr]) && runes[curr] != '_') {
		return "", false, false, 0
	}

	nameStart := curr
	for curr < len(runes) && (unicode.IsLetter(runes[curr]) || unicode.IsDigit(runes[curr]) || runes[curr] == '.' || runes[curr] == '-' || runes[curr] == '_') {
		curr++
	}
	tagName := string(runes[nameStart:curr])

	// TypeScript primitive types are never JSX elements
	switch strings.ToLower(tagName) {
	case "string", "number", "boolean", "any", "unknown", "never", "void", "object", "symbol", "bigint", "null", "undefined":
		return "", false, false, 0
	}

	// Scan through attributes until '>'
	inAttrQuote := false
	var attrQuoteChar rune
	braceDepth := 0

	for curr < len(runes) {
		r := runes[curr]

		if !inAttrQuote {
			if r == '"' || r == '\'' {
				inAttrQuote = true
				attrQuoteChar = r
			} else if r == '{' {
				braceDepth++
			} else if r == '}' && braceDepth > 0 {
				braceDepth--
			} else if r == '/' && curr+1 < len(runes) && runes[curr+1] == '>' && braceDepth == 0 {
				isSelfClosing = true
				curr += 2
				return tagName, isClosing, isSelfClosing, (curr - 1) - idx
			} else if r == '>' && braceDepth == 0 {
				return tagName, isClosing, isSelfClosing, curr - idx
			}
		} else {
			if r == attrQuoteChar && runes[curr-1] != '\\' {
				inAttrQuote = false
			}
		}
		curr++
	}

	return "", false, false, 0
}

func isVoidHTMLTag(tag string) bool {
	switch strings.ToLower(tag) {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	default:
		return false
	}
}

// -----------------------------------------------------------------------------
// Rust, SQL, Shell, HTML, CSS Validators
// -----------------------------------------------------------------------------

func (v *ASTSyntaxValidator) validateRust(code string, opts SyntaxValidationOptions) SyntaxValidationReport {
	return v.validateGenericDelimiters(code, LangRust, opts)
}

func (v *ASTSyntaxValidator) validateSQL(code string, opts SyntaxValidationOptions) SyntaxValidationReport {
	lines := strings.Split(code, "\n")
	metrics := ASTMetrics{LineCount: len(lines)}
	var diags []SyntaxDiagnostic

	// Check string quote balance (single quotes in standard SQL)
	inSingle := false
	inDouble := false
	runes := []rune(code)
	parenStack := 0

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\'' && !inDouble {
			// SQL escape: '' represents an escaped single quote
			if inSingle && i+1 < len(runes) && runes[i+1] == '\'' {
				i++
				continue
			}
			inSingle = !inSingle
			continue
		}
		if r == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}

		if inSingle || inDouble {
			continue
		}

		if r == '(' {
			parenStack++
		} else if r == ')' {
			if parenStack == 0 {
				line, col := offsetToLineCol(code, i)
				diags = append(diags, SyntaxDiagnostic{
					Severity: SeverityError,
					Line:     line,
					Column:   col,
					RuleID:   "sql/unmatched_paren",
					Message:  "unmatched closing parenthesis ')' in SQL query",
				})
			} else {
				parenStack--
			}
		}
	}

	if inSingle {
		diags = append(diags, SyntaxDiagnostic{
			Severity: SeverityError,
			Line:     len(lines),
			Column:   1,
			RuleID:   "sql/unterminated_string",
			Message:  "unterminated string literal in SQL statement",
		})
	}
	if parenStack > 0 {
		diags = append(diags, SyntaxDiagnostic{
			Severity: SeverityError,
			Line:     len(lines),
			Column:   1,
			RuleID:   "sql/unclosed_paren",
			Message:  "unclosed parenthesis in SQL query",
		})
	}

	return SyntaxValidationReport{
		IsValid:     len(diags) == 0,
		Compiler:    "sql/syntax_checker",
		Diagnostics: diags,
		Metrics:     metrics,
	}
}

func (v *ASTSyntaxValidator) validateShell(code string, opts SyntaxValidationOptions) SyntaxValidationReport {
	return v.validateGenericDelimiters(code, LangShell, opts)
}

func (v *ASTSyntaxValidator) validateHTML(code string, opts SyntaxValidationOptions) SyntaxValidationReport {
	return v.validateJavaScriptFamily(code, LangJSX, opts)
}

func (v *ASTSyntaxValidator) validateCSS(code string, opts SyntaxValidationOptions) SyntaxValidationReport {
	return v.validateGenericDelimiters(code, LangCSS, opts)
}

func (v *ASTSyntaxValidator) validateGenericDelimiters(code string, lang LanguageKind, opts SyntaxValidationOptions) SyntaxValidationReport {
	lines := strings.Split(code, "\n")
	metrics := ASTMetrics{LineCount: len(lines)}
	var diags []SyntaxDiagnostic

	var stack []rune
	inSingle := false
	inDouble := false
	inBacktick := false
	escaped := false

	runes := []rune(code)
	line := 1
	col := 0

	for i := 0; i < len(runes); i++ {
		col++
		r := runes[i]

		if r == '\n' {
			line++
			col = 0
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

		if r == '\'' && !inDouble && !inBacktick {
			if lang == LangRust && !inSingle && i+1 < len(runes) && (unicode.IsLetter(runes[i+1]) || runes[i+1] == '_') {
				// Check if character literal like 'a'
				if i+2 < len(runes) && runes[i+2] == '\'' {
					i += 2
					col += 2
					continue
				}
				// Lifetime identifier like 'a, 'static, 'de
				j := i + 1
				for j < len(runes) && (unicode.IsLetter(runes[j]) || unicode.IsDigit(runes[j]) || runes[j] == '_') {
					j++
				}
				col += (j - 1 - i)
				i = j - 1
				continue
			}
			inSingle = !inSingle
			continue
		}
		if r == '"' && !inSingle && !inBacktick {
			inDouble = !inDouble
			continue
		}
		if r == '`' && !inSingle && !inDouble {
			inBacktick = !inBacktick
			continue
		}

		if inSingle || inDouble || inBacktick {
			continue
		}

		switch r {
		case '(', '[', '{':
			stack = append(stack, r)
		case ')':
			if len(stack) == 0 || stack[len(stack)-1] != '(' {
				diags = append(diags, SyntaxDiagnostic{
					Severity:       SeverityError,
					Line:           line,
					Column:         col,
					RuleID:         "generic/unmatched_paren",
					Message:        "unmatched closing parenthesis ')'",
					ContextSnippet: getLineSnippet(lines, line),
				})
			} else {
				stack = stack[:len(stack)-1]
			}
		case ']':
			if len(stack) == 0 || stack[len(stack)-1] != '[' {
				diags = append(diags, SyntaxDiagnostic{
					Severity:       SeverityError,
					Line:           line,
					Column:         col,
					RuleID:         "generic/unmatched_bracket",
					Message:        "unmatched closing bracket ']'",
					ContextSnippet: getLineSnippet(lines, line),
				})
			} else {
				stack = stack[:len(stack)-1]
			}
		case '}':
			if len(stack) == 0 || stack[len(stack)-1] != '{' {
				diags = append(diags, SyntaxDiagnostic{
					Severity:       SeverityError,
					Line:           line,
					Column:         col,
					RuleID:         "generic/unmatched_brace",
					Message:        "unmatched closing brace '}'",
					ContextSnippet: getLineSnippet(lines, line),
				})
			} else {
				stack = stack[:len(stack)-1]
			}
		}

		if len(diags) >= opts.MaxDiagnostics {
			break
		}
	}

	if inSingle || inDouble || inBacktick {
		diags = append(diags, SyntaxDiagnostic{
			Severity: SeverityError,
			Line:     line,
			Column:   col,
			RuleID:   "generic/unterminated_string",
			Message:  "unterminated string literal at end of block",
		})
	}
	if len(stack) > 0 {
		diags = append(diags, SyntaxDiagnostic{
			Severity: SeverityError,
			Line:     line,
			Column:   col,
			RuleID:   "generic/unclosed_delimiter",
			Message:  fmt.Sprintf("unclosed delimiter '%c'", stack[len(stack)-1]),
		})
	}

	return SyntaxValidationReport{
		IsValid:     len(diags) == 0,
		Compiler:    fmt.Sprintf("%s/delimiter_checker", lang),
		Diagnostics: diags,
		Metrics:     metrics,
	}
}

// -----------------------------------------------------------------------------
// Utilities
// -----------------------------------------------------------------------------

func offsetToLineCol(text string, offset int) (int, int) {
	line := 1
	col := 1
	for idx, r := range text {
		if idx >= offset {
			break
		}
		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

func getLineSnippet(lines []string, lineNum int) string {
	idx := lineNum - 1
	if idx >= 0 && idx < len(lines) {
		return strings.TrimSpace(lines[idx])
	}
	return ""
}
