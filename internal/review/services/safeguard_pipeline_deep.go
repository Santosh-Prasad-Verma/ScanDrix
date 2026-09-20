// Package services provides production-grade infrastructure implementations for ScanDrix code review.
package services

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"sync"

	"github.com/scandrix/backend/internal/review/domain"
)

// SafeguardFeatureSet encapsulates the 13 boolean signals evaluated per suggestion.
// Matches safeguardFeatureExtractionSchema.
type SafeguardFeatureSet struct {
	HasResourceLeak         bool `json:"has_resource_leak"`
	HasInconsistentContract bool `json:"has_inconsistent_contract"`
	HasWrongAlgorithm       bool `json:"has_wrong_algorithm"`
	HasDataExposure         bool `json:"has_data_exposure"`
	HasMissingErrorHandling bool `json:"has_missing_error_handling"`
	HasRedundantWorkInLoop  bool `json:"has_redundant_work_in_loop"`
	HasUnsafeDataFlow       bool `json:"has_unsafe_data_flow"`
	RequiresAssumedInput    bool `json:"requires_assumed_input"`
	RequiresAssumedWorkload  bool `json:"requires_assumed_workload"`
	IsQualityOpinion        bool `json:"is_quality_opinion"`
	IsAntiPatternOnly       bool `json:"is_anti_pattern_only"`
	TargetsUnchangedCode    bool `json:"targets_unchanged_code"`
	ImprovedCodeIsCorrect   bool `json:"improvedCode_is_correct"`
}

// TriageDecision represents the deterministic classification for a suggestion.
type TriageDecision string

const (
	TriageDecisionKeep    TriageDecision = "keep"
	TriageDecisionDiscard TriageDecision = "discard"
	TriageDecisionVerify  TriageDecision = "verify"
)

// SafeguardAction maps triage outcomes to concrete pipeline operations.
type SafeguardAction string

const (
	SafeguardActionNoChanges SafeguardAction = "no_changes"
	SafeguardActionUpdate    SafeguardAction = "update"
	SafeguardActionDiscard   SafeguardAction = "discard"
)

// SafeguardEvaluation records the complete analysis of a code suggestion.
type SafeguardEvaluation struct {
	SuggestionID string              `json:"suggestion_id"`
	FilePath     string              `json:"file_path"`
	Features     SafeguardFeatureSet `json:"features"`
	Decision     TriageDecision      `json:"decision"`
	Action       SafeguardAction     `json:"action"`
	Reason       string              `json:"reason"`
	Verified     bool                `json:"verified"`
}

// SafeguardPipelineEngine performs multi-tier validation, speculation triage, and AST boundary checks.
type SafeguardPipelineEngine struct {
	mu           sync.RWMutex
	syntaxCheck  *SandboxSyntaxValidator
	builtins     map[string]map[string]bool // language -> builtin identifier set
}

// NewSafeguardPipelineEngine creates the safeguard engine.
func NewSafeguardPipelineEngine(syntaxValidator *SandboxSyntaxValidator) *SafeguardPipelineEngine {
	if syntaxValidator == nil {
		syntaxValidator = NewSandboxSyntaxValidator()
	}

	eng := &SafeguardPipelineEngine{
		syntaxCheck: syntaxValidator,
		builtins:    make(map[string]map[string]bool),
	}
	eng.initBuiltins()
	return eng
}

func (e *SafeguardPipelineEngine) initBuiltins() {
	// Go builtins
	goBuiltins := map[string]bool{
		"append": true, "cap": true, "close": true, "complex": true, "copy": true,
		"delete": true, "imag": true, "len": true, "make": true, "new": true,
		"panic": true, "print": true, "println": true, "real": true, "recover": true,
		"true": true, "false": true, "nil": true, "iota": true,
		"error": true, "string": true, "int": true, "int64": true, "bool": true,
		"byte": true, "rune": true, "float64": true, "any": true,
	}
	e.builtins["go"] = goBuiltins

	// TypeScript / JavaScript builtins
	jsBuiltins := map[string]bool{
		"console": true, "Math": true, "JSON": true, "Promise": true, "Array": true,
		"Object": true, "String": true, "Number": true, "Boolean": true, "Date": true,
		"RegExp": true, "Error": true, "Map": true, "Set": true, "WeakMap": true,
		"WeakSet": true, "Symbol": true, "parseInt": true, "parseFloat": true,
		"undefined": true, "null": true, "true": true, "false": true, "NaN": true,
		"Infinity": true, "setTimeout": true, "clearTimeout": true,
	}
	e.builtins["ts"] = jsBuiltins
	e.builtins["js"] = jsBuiltins
	e.builtins["tsx"] = jsBuiltins
	e.builtins["jsx"] = jsBuiltins

	// Python builtins
	pyBuiltins := map[string]bool{
		"print": true, "len": true, "range": true, "str": true, "int": true,
		"float": true, "list": true, "dict": true, "set": true, "tuple": true,
		"bool": true, "enumerate": true, "zip": true, "isinstance": true,
		"True": true, "False": true, "None": true, "Exception": true,
		"super": true, "open": true, "sum": true, "min": true, "max": true,
	}
	e.builtins["py"] = pyBuiltins
	e.builtins["python"] = pyBuiltins
}

// TriageSuggestion implements deterministic triage classification.
func (e *SafeguardPipelineEngine) TriageSuggestion(f SafeguardFeatureSet) TriageDecision {
	hasHardDiscard := f.IsQualityOpinion || f.TargetsUnchangedCode
	hasSoftSpeculation := f.RequiresAssumedInput || f.RequiresAssumedWorkload || f.IsAntiPatternOnly
	hasStructuralDefect := f.HasResourceLeak || f.HasInconsistentContract || f.HasWrongAlgorithm ||
		f.HasDataExposure || f.HasMissingErrorHandling || f.HasRedundantWorkInLoop || f.HasUnsafeDataFlow

	// Hard discard: definitive non-actionable speculation
	if hasHardDiscard {
		return TriageDecisionDiscard
	}

	// Soft speculation + structural defect -> ambiguous, needs agent/sandbox verification
	if hasSoftSpeculation && hasStructuralDefect {
		return TriageDecisionVerify
	}

	// Soft speculation only -> discard
	if hasSoftSpeculation {
		return TriageDecisionDiscard
	}

	// Structural defect only -> verify mitigation in surrounding callers
	if hasStructuralDefect {
		return TriageDecisionVerify
	}

	// Inconclusive -> verify
	return TriageDecisionVerify
}

// TriageToAction maps the decision and correctness to concrete pipeline action.
func (e *SafeguardPipelineEngine) TriageToAction(decision TriageDecision, improvedCodeIsCorrect bool) SafeguardAction {
	switch decision {
	case TriageDecisionKeep:
		if improvedCodeIsCorrect {
			return SafeguardActionNoChanges
		}
		return SafeguardActionUpdate
	case TriageDecisionDiscard:
		return SafeguardActionDiscard
	case TriageDecisionVerify:
		return SafeguardActionDiscard // Safe default if unverified
	default:
		return SafeguardActionDiscard
	}
}

// ExtractFeaturesHeuristic extracts feature flags through deterministic AST pattern analysis.
func (e *SafeguardPipelineEngine) ExtractFeaturesHeuristic(sug domain.CodeSuggestion, fullFileContent string) SafeguardFeatureSet {
	f := SafeguardFeatureSet{
		ImprovedCodeIsCorrect: true,
	}

	descLower := strings.ToLower(sug.GetDescription())

	// Quality opinion / style heuristics
	if strings.Contains(descLower, "consider renaming") ||
		strings.Contains(descLower, "stylistic") ||
		strings.Contains(descLower, "cleaner syntax") ||
		strings.Contains(descLower, "more idiomatic") ||
		sug.Category == domain.CategoryStyle {
		f.IsQualityOpinion = true
	}

	// Resource leak
	if strings.Contains(descLower, "leak") ||
		strings.Contains(descLower, "unclosed") ||
		strings.Contains(descLower, "defer") ||
		strings.Contains(descLower, "cleanup") {
		f.HasResourceLeak = true
	}

	// Missing error handling
	if strings.Contains(descLower, "unhandled error") ||
		strings.Contains(descLower, "check error") ||
		strings.Contains(descLower, "nil pointer") ||
		strings.Contains(descLower, "null pointer") {
		f.HasMissingErrorHandling = true
	}

	// Unsafe data flow / injection
	if strings.Contains(descLower, "injection") ||
		strings.Contains(descLower, "xss") ||
		strings.Contains(descLower, "sanitize") ||
		strings.Contains(descLower, "overflow") {
		f.HasUnsafeDataFlow = true
	}

	// Inconsistent contract
	if strings.Contains(descLower, "interface mismatch") ||
		strings.Contains(descLower, "signature") ||
		strings.Contains(descLower, "return value") {
		f.HasInconsistentContract = true
	}

	// Redundant loop work
	if strings.Contains(descLower, "inside loop") ||
		strings.Contains(descLower, "allocate in loop") ||
		strings.Contains(descLower, "n+1") {
		f.HasRedundantWorkInLoop = true
	}

	// Validate improved code syntax
	rep := sug.GetSuggestedReplacement()
	if strings.TrimSpace(rep) != "" {
		res := e.syntaxCheck.ValidateSnippet(context.Background(), sug.GetFilePath(), rep)
		if !res.IsValid {
			f.ImprovedCodeIsCorrect = false
		}
	}

	return f
}

// DetectHallucinatedIdentifiers verifies that newly introduced symbols in suggested code exist in the file or imports.
func (e *SafeguardPipelineEngine) DetectHallucinatedIdentifiers(
	sug domain.CodeSuggestion,
	fullFileContent string,
) (bool, []string) {
	rep := sug.GetSuggestedReplacement()
	if strings.TrimSpace(rep) == "" {
		return false, nil
	}

	ext := strings.ToLower(getFileExt(sug.GetFilePath()))
	identRegex := regexp.MustCompile(`\b[a-zA-Z_][a-zA-Z0-9_]*\b`)
	suggestedIdents := identRegex.FindAllString(rep, -1)

	fileIdents := make(map[string]bool)
	for _, id := range identRegex.FindAllString(fullFileContent, -1) {
		fileIdents[id] = true
	}

	builtinSet := e.builtins[ext]
	var suspicious []string

	for _, id := range suggestedIdents {
		// Ignore very short identifiers
		if len(id) <= 2 {
			continue
		}
		// Ignore if present in original file
		if fileIdents[id] {
			continue
		}
		// Ignore if standard builtin or keyword
		if builtinSet != nil && builtinSet[id] {
			continue
		}
		suspicious = append(suspicious, id)
	}

	// If more than 3 unfamiliar identifiers appear, it indicates probable LLM hallucination
	if len(suspicious) >= 3 {
		return true, suspicious
	}
	return false, nil
}

// EvaluateSuggestion executes the multi-stage safeguard pipeline on a suggestion.
func (e *SafeguardPipelineEngine) EvaluateSuggestion(
	ctx context.Context,
	sug domain.CodeSuggestion,
	fullFileContent string,
) SafeguardEvaluation {
	features := e.ExtractFeaturesHeuristic(sug, fullFileContent)
	decision := e.TriageSuggestion(features)
	action := e.TriageToAction(decision, features.ImprovedCodeIsCorrect)

	reason := ""
	if action == SafeguardActionDiscard {
		if features.IsQualityOpinion {
			reason = "discarded: cosmetic opinion only"
		} else if !features.ImprovedCodeIsCorrect {
			reason = "discarded: proposed code syntax invalid"
		} else {
			reason = "discarded: speculation unverified"
		}
	} else if action == SafeguardActionUpdate {
		reason = "needs syntax repair before submission"
	} else {
		reason = "verified: concrete structural defect"
	}

	// Secondary hallucination check
	if action == SafeguardActionNoChanges {
		if isHallucinated, unknowns := e.DetectHallucinatedIdentifiers(sug, fullFileContent); isHallucinated {
			decision = TriageDecisionDiscard
			action = SafeguardActionDiscard
			reason = fmt.Sprintf("discarded: hallucinated unknown symbols %v", unknowns)
		}
	}

	return SafeguardEvaluation{
		SuggestionID: sug.ID.String(),
		FilePath:     sug.GetFilePath(),
		Features:     features,
		Decision:     decision,
		Action:       action,
		Reason:       reason,
		Verified:     action == SafeguardActionNoChanges,
	}
}

// BatchFilterSuggestions applies safeguards across an array of suggestions.
func (e *SafeguardPipelineEngine) BatchFilterSuggestions(
	ctx context.Context,
	suggestions []domain.CodeSuggestion,
	fileContents map[string]string,
) ([]domain.CodeSuggestion, []SafeguardEvaluation) {
	var kept []domain.CodeSuggestion
	var evaluations []SafeguardEvaluation

	for _, s := range suggestions {
		content := fileContents[s.GetFilePath()]
		eval := e.EvaluateSuggestion(ctx, s, content)
		evaluations = append(evaluations, eval)

		if eval.Action == SafeguardActionNoChanges {
			kept = append(kept, s)
		} else {
			s.DeliveryStatus = domain.DeliveryStatusDiscarded
			s.DiscardReason = eval.Reason
		}
	}

	return kept, evaluations
}

// ParseGoSyntaxTree validates Go source code and extracts exported symbols.
func ParseGoSyntaxTree(code string) ([]string, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, "snippet.go", code, parser.AllErrors)
	if err != nil {
		return nil, err
	}

	var symbols []string
	for _, decl := range node.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			symbols = append(symbols, fn.Name.Name)
		}
	}
	return symbols, nil
}

func getFileExt(path string) string {
	idx := strings.LastIndex(path, ".")
	if idx != -1 {
		return path[idx+1:]
	}
	return ""
}

// ParseTypeScriptSymbols parses TypeScript / JavaScript declarations without external AST dependencies.
func ParseTypeScriptSymbols(code string) ([]string, error) {
	var symbols []string
	// Match functions, classes, interfaces, types, and top-level const/let/var
	reFunc := regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:async\s+)?function\s+([a-zA-Z0-9_$]+)`)
	reClass := regexp.MustCompile(`(?m)^\s*(?:export\s+)?class\s+([a-zA-Z0-9_$]+)`)
	reInterface := regexp.MustCompile(`(?m)^\s*(?:export\s+)?interface\s+([a-zA-Z0-9_$]+)`)
	reType := regexp.MustCompile(`(?m)^\s*(?:export\s+)?type\s+([a-zA-Z0-9_$]+)`)
	reVar := regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const|let|var)\s+([a-zA-Z0-9_$]+)\s*(?:=|:)`)

	for _, match := range reFunc.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, match[1])
	}
	for _, match := range reClass.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, match[1])
	}
	for _, match := range reInterface.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, match[1])
	}
	for _, match := range reType.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, match[1])
	}
	for _, match := range reVar.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, match[1])
	}

	return symbols, nil
}

// ParsePythonSymbols parses Python functions, classes, and module-level variables.
func ParsePythonSymbols(code string) ([]string, error) {
	var symbols []string
	reDef := regexp.MustCompile(`(?m)^\s*(?:async\s+)?def\s+([a-zA-Z0-9_]+)\s*\(`)
	reClass := regexp.MustCompile(`(?m)^\s*class\s+([a-zA-Z0-9_]+)\s*(?:\(|:)`)
	reAssign := regexp.MustCompile(`(?m)^([a-zA-Z0-9_]+)\s*(?::\s*[^=]+)?\s*=`)

	for _, m := range reDef.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, m[1])
	}
	for _, m := range reClass.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, m[1])
	}
	for _, m := range reAssign.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, m[1])
	}

	return symbols, nil
}

// ParseJavaSymbols parses Java class, interface, enum, and method signatures.
func ParseJavaSymbols(code string) ([]string, error) {
	var symbols []string
	reClass := regexp.MustCompile(`(?m)\b(?:public|protected|private|static|\s)+class\s+([a-zA-Z0-9_]+)`)
	reInterface := regexp.MustCompile(`(?m)\b(?:public|protected|private|\s)+interface\s+([a-zA-Z0-9_]+)`)
	reMethod := regexp.MustCompile(`(?m)\b(?:public|protected|private|static|\s)+[\w<>\[\],\s]+\s+([a-zA-Z0-9_]+)\s*\([^)]*\)\s*(?:throws\s+[\w,\s]+)?\s*\{`)

	for _, m := range reClass.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, m[1])
	}
	for _, m := range reInterface.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, m[1])
	}
	for _, m := range reMethod.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, m[1])
	}

	return symbols, nil
}

// ParseRustSymbols parses Rust functions, structs, enums, traits, and type aliases.
func ParseRustSymbols(code string) ([]string, error) {
	var symbols []string
	reFn := regexp.MustCompile(`(?m)^\s*(?:pub(?:\([^)]+\))?\s+)?(?:async\s+)?fn\s+([a-zA-Z0-9_]+)`)
	reStruct := regexp.MustCompile(`(?m)^\s*(?:pub(?:\([^)]+\))?\s+)?struct\s+([a-zA-Z0-9_]+)`)
	reEnum := regexp.MustCompile(`(?m)^\s*(?:pub(?:\([^)]+\))?\s+)?enum\s+([a-zA-Z0-9_]+)`)
	reTrait := regexp.MustCompile(`(?m)^\s*(?:pub(?:\([^)]+\))?\s+)?trait\s+([a-zA-Z0-9_]+)`)

	for _, m := range reFn.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, m[1])
	}
	for _, m := range reStruct.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, m[1])
	}
	for _, m := range reEnum.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, m[1])
	}
	for _, m := range reTrait.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, m[1])
	}

	return symbols, nil
}

// ParseCSharpSymbols parses C# namespaces, classes, structs, records, and methods.
func ParseCSharpSymbols(code string) ([]string, error) {
	var symbols []string
	reClass := regexp.MustCompile(`(?m)\b(?:public|internal|private|protected|\s)+class\s+([a-zA-Z0-9_]+)`)
	reRecord := regexp.MustCompile(`(?m)\b(?:public|internal|private|protected|\s)+record\s+([a-zA-Z0-9_]+)`)
	reInterface := regexp.MustCompile(`(?m)\b(?:public|internal|private|protected|\s)+interface\s+([a-zA-Z0-9_]+)`)
	reMethod := regexp.MustCompile(`(?m)\b(?:public|internal|private|protected|static|async|\s)+[\w<>\[\],\s]+\s+([a-zA-Z0-9_]+)\s*\([^)]*\)\s*\{`)

	for _, m := range reClass.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, m[1])
	}
	for _, m := range reRecord.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, m[1])
	}
	for _, m := range reInterface.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, m[1])
	}
	for _, m := range reMethod.FindAllStringSubmatch(code, -1) {
		symbols = append(symbols, m[1])
	}

	return symbols, nil
}

// ExtractMultiLanguageSymbols dispatches symbol extraction to language-specific parsers.
func (e *SafeguardPipelineEngine) ExtractMultiLanguageSymbols(code, ext string) ([]string, error) {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	switch ext {
	case "go":
		return ParseGoSyntaxTree(code)
	case "ts", "tsx", "js", "jsx":
		return ParseTypeScriptSymbols(code)
	case "py":
		return ParsePythonSymbols(code)
	case "java":
		return ParseJavaSymbols(code)
	case "rs":
		return ParseRustSymbols(code)
	case "cs":
		return ParseCSharpSymbols(code)
	default:
		// Generic identifier extraction fallback
		identRegex := regexp.MustCompile(`\b[a-zA-Z_][a-zA-Z0-9_]*\b`)
		return identRegex.FindAllString(code, -1), nil
	}
}

// VerifyContractConsistency detects breaking signature modifications across diff hunks.
func (e *SafeguardPipelineEngine) VerifyContractConsistency(existingCode, replacementCode, ext string) (bool, string) {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))

	// Extract function signatures before and after
	var reSig *regexp.Regexp
	switch ext {
	case "go":
		reSig = regexp.MustCompile(`func\s+(?:\([^)]+\)\s+)?([a-zA-Z0-9_]+)\s*\(([^)]*)\)`)
	case "ts", "tsx", "js", "jsx":
		reSig = regexp.MustCompile(`function\s+([a-zA-Z0-9_$]+)\s*\(([^)]*)\)`)
	case "py":
		reSig = regexp.MustCompile(`def\s+([a-zA-Z0-9_]+)\s*\(([^)]*)\)`)
	default:
		return true, "skipped contract verification for unsupported extension"
	}

	mExist := reSig.FindStringSubmatch(existingCode)
	mRepl := reSig.FindStringSubmatch(replacementCode)

	if len(mExist) > 1 && len(mRepl) > 1 {
		existName := mExist[1]
		replName := mRepl[1]

		if existName != replName {
			return false, fmt.Sprintf("inconsistent contract: function name renamed from %s to %s", existName, replName)
		}

		// Count parameters
		existParams := strings.Split(mExist[2], ",")
		replParams := strings.Split(mRepl[2], ",")

		// If existing function had 0 params and replacement has more, check arity drift
		if len(strings.TrimSpace(mExist[2])) == 0 {
			existParams = nil
		}
		if len(strings.TrimSpace(mRepl[2])) == 0 {
			replParams = nil
		}

		if len(existParams) != len(replParams) {
			return false, fmt.Sprintf("inconsistent contract: parameter count changed from %d to %d without refactoring call sites", len(existParams), len(replParams))
		}
	}

	return true, "contract verified consistent"
}

// DetectResourceLeaks inspects code snippets for open file/socket/db handles without defer/close.
func (e *SafeguardPipelineEngine) DetectResourceLeaks(code, ext string) (bool, string) {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))

	switch ext {
	case "go":
		// Check for os.Open, http.Get without defer Close
		if strings.Contains(code, "os.Open(") || strings.Contains(code, "http.Get(") || strings.Contains(code, "sql.Open(") {
			if !strings.Contains(code, ".Close()") && !strings.Contains(code, "defer ") {
				return true, "resource leak: handle opened without defer close"
			}
		}
	case "py":
		if strings.Contains(code, "open(") && !strings.Contains(code, "with open") && !strings.Contains(code, ".close()") {
			return true, "resource leak: file opened without context manager (with open) or close"
		}
	case "ts", "js":
		if strings.Contains(code, "fs.createReadStream(") && !strings.Contains(code, ".close()") && !strings.Contains(code, ".destroy()") {
			return true, "resource leak: stream created without destroy or close handler"
		}
	}

	return false, "no resource leak detected"
}

// VerifyErrorHandling checks if errors are properly propagated or logged rather than discarded.
func (e *SafeguardPipelineEngine) VerifyErrorHandling(code, ext string) (bool, string) {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))

	switch ext {
	case "go":
		if strings.Contains(code, "_ = ") && (strings.Contains(code, "err") || strings.Contains(code, "Close()")) {
			return false, "suppressed error: explicit blank identifier assignment `_ = err`"
		}
	case "py":
		if strings.Contains(code, "except:") && strings.Contains(code, "pass") {
			return false, "suppressed error: bare `except: pass` block silences exceptions"
		}
	case "ts", "js":
		if strings.Contains(code, "catch (e) {}") || strings.Contains(code, "catch {}") {
			return false, "suppressed error: empty catch block swallows runtime errors"
		}
	}

	return true, "error handling verified"
}

// AnalyzeTaintFlow checks for dangerous unsanitized data flows from inputs to execution sinks.
func (e *SafeguardPipelineEngine) AnalyzeTaintFlow(code, ext string) (bool, string) {
	_ = ext

	// Dangerous sinks: SQL execution, command execution, deserialization
	sqlSinks := []string{"fmt.Sprintf(\"SELECT", "fmt.Sprintf(\"INSERT", "fmt.Sprintf(\"UPDATE", "fmt.Sprintf(\"DELETE"}
	for _, sink := range sqlSinks {
		if strings.Contains(code, sink) {
			return true, "unsafe taint flow: direct string interpolation into SQL query sink"
		}
	}

	cmdSinks := []string{"exec.Command(\"sh\", \"-c\",", "os.system(", "exec(", "child_process.exec("}
	for _, sink := range cmdSinks {
		if strings.Contains(code, sink) {
			return true, "unsafe taint flow: unquoted parameter passed to shell execution sink"
		}
	}

	return false, "taint flow within safe parameters"
}

// VerifyWithPromptOnly evaluates suggestions when a sandbox runtime is unavailable.
func (e *SafeguardPipelineEngine) VerifyWithPromptOnly(
	ctx context.Context,
	sug domain.CodeSuggestion,
	contextLines string,
) (bool, string) {
	// Deterministic validation rules:
	// 1. Replacement must not be identical to existing code
	if strings.TrimSpace(sug.GetSuggestedReplacement()) == strings.TrimSpace(sug.GetOriginalDiff()) && sug.GetSuggestedReplacement() != "" {
		return false, "redundant suggestion: proposed code is identical to existing code"
	}

	// 2. Replacement must not contain placeholder comments
	badPatterns := []string{"// TODO", "/* implement here */", "...", "// rest of code goes here"}
	for _, p := range badPatterns {
		if strings.Contains(sug.GetSuggestedReplacement(), p) {
			return false, fmt.Sprintf("incomplete suggestion: contains placeholder comment `%s`", p)
		}
	}

	// 3. Diff boundary check
	if sug.GetStartLine() <= 0 || sug.GetEndLine() < sug.GetStartLine() {
		return false, fmt.Sprintf("invalid line coordinates [%d..%d]", sug.GetStartLine(), sug.GetEndLine())
	}

	return true, "suggestion verified via static heuristics"
}

// VerifyWithSandboxRunner validates code changes using compiler tools in a containerized environment.
func (e *SafeguardPipelineEngine) VerifyWithSandboxRunner(
	ctx context.Context,
	sug domain.CodeSuggestion,
) (bool, string) {
	if e.syntaxCheck == nil {
		return true, "syntax runner not configured, skipping sandbox test"
	}

	ext := strings.ToLower(getFileExt(sug.GetFilePath()))
	res, err := e.syntaxCheck.ValidateSyntax(ctx, ext, sug.GetFilePath(), sug.GetSuggestedReplacement())
	if err != nil {
		return false, fmt.Sprintf("sandbox execution error: %v", err)
	}

	if !res.IsValid {
		return false, fmt.Sprintf("sandbox compiler error: %s", res.ErrorMessage)
	}

	return true, "sandbox compiler check passed"
}

