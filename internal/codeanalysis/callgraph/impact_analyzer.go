package callgraph

import (
	"bufio"
	"context"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// DefaultImpactAnalyzer implements pure-Go multi-language Call-Graph Impact Analysis.
type DefaultImpactAnalyzer struct {
	// Regular expressions for detecting symbol declarations across languages
	goFuncRegex    *regexp.Regexp
	pyDefRegex     *regexp.Regexp
	tsFuncRegex    *regexp.Regexp
	rustFnRegex    *regexp.Regexp
	javaMethodReg  *regexp.Regexp
	callSiteRegexes map[string]*regexp.Regexp
}

// NewImpactAnalyzer initializes the analyzer with pre-compiled extraction patterns.
func NewImpactAnalyzer() *DefaultImpactAnalyzer {
	return &DefaultImpactAnalyzer{
		// Go: func (r *Receiver) Method(arg Type) Ret
		goFuncRegex: regexp.MustCompile(`^func\s+(?:\([^)]+\)\s+)?([A-Za-z0-9_]+)\s*\((.*?)\)(?:\s*(.+))?`),
		// Python: def func(arg: int) -> str:
		pyDefRegex: regexp.MustCompile(`^(?:async\s+)?def\s+([A-Za-z0-9_]+)\s*\((.*?)\)(?:\s*->\s*([^:]+))?:`),
		// TS/JS: function name(arg: type): ret OR const name = (arg) => ...
		tsFuncRegex: regexp.MustCompile(`(?:export\s+)?(?:async\s+)?(?:function\s+([A-Za-z0-9_]+)|(?:const|let|var)\s+([A-Za-z0-9_]+)\s*=\s*(?:async\s+)?)\s*\((.*?)\)(?:\s*:\s*([^={]+))?`),
		// Rust: pub fn name(arg: type) -> ret
		rustFnRegex: regexp.MustCompile(`(?:pub(?:\([^)]+\))?\s+)?(?:async\s+)?fn\s+([A-Za-z0-9_]+)\s*(?:<[^>]+>)?\s*\((.*?)\)(?:\s*->\s*([^{]+))?`),
		// Java: public static Ret name(arg) {
		javaMethodReg: regexp.MustCompile(`(?:public|protected|private)\s+(?:static\s+)?[A-Za-z0-9_<>[\]]+\s+([A-Za-z0-9_]+)\s*\((.*?)\)`),
		callSiteRegexes: make(map[string]*regexp.Regexp),
	}
}

// DiffFileHunks represents parsed changes per file in a unified diff.
type DiffFileHunks struct {
	OldPath string
	NewPath string
	RemovedLines []string
	AddedLines   []string
}

// AnalyzeDiff performs impact analysis from diffText alone.
func (a *DefaultImpactAnalyzer) AnalyzeDiff(ctx context.Context, repoID string, diffText string) (*ImpactReport, error) {
	return a.AnalyzeDiffWithFiles(ctx, repoID, diffText, nil)
}

// AnalyzeDiffWithFiles analyzes the diff and finds all downstream call sites across repo files.
func (a *DefaultImpactAnalyzer) AnalyzeDiffWithFiles(ctx context.Context, repoID string, diffText string, files FileProvider) (*ImpactReport, error) {
	report := &ImpactReport{
		ModifiedSymbols: make([]SymbolChange, 0),
		AffectedFiles:   make([]string, 0),
		CallSites:       make([]CallSite, 0),
	}

	if strings.TrimSpace(diffText) == "" {
		return report, nil
	}

	hunks := a.parseUnifiedDiff(diffText)

	for _, hunk := range hunks {
		filePath := hunk.NewPath
		if filePath == "/dev/null" || filePath == "" {
			filePath = hunk.OldPath
		}
		filePath = strings.TrimPrefix(filePath, "b/")
		filePath = strings.TrimPrefix(filePath, "a/")

		// Detect modified symbols in this file
		symbols := a.detectSymbolChanges(filePath, hunk)
		report.ModifiedSymbols = append(report.ModifiedSymbols, symbols...)
	}

	// If a FileProvider is supplied, search for call sites across unchanged files
	if files != nil && len(report.ModifiedSymbols) > 0 {
		fileList, err := files.ListFiles(ctx)
		if err == nil {
			affectedMap := make(map[string]bool)
			modifiedFileMap := make(map[string]bool)
			for _, sym := range report.ModifiedSymbols {
				modifiedFileMap[sym.FilePath] = true
			}

			for _, sym := range report.ModifiedSymbols {
				// Only track external call sites for breaking or modified symbols
				callRegex := a.getCallRegex(sym.SymbolName)
				for _, f := range fileList {
					// Ignore the file that introduced the definition itself
					if modifiedFileMap[f] || a.isIgnoredPath(f) {
						continue
					}

					content, rErr := files.ReadFile(ctx, f)
					if rErr != nil || len(content) > 500*1024 {
						continue // skip unreadable or >500KB files
					}

					scanner := bufio.NewScanner(strings.NewReader(string(content)))
					lineNo := 0
					for scanner.Scan() {
						lineNo++
						line := scanner.Text()
						if callRegex.MatchString(line) {
							snippet := strings.TrimSpace(line)
							if len(snippet) > 120 {
								snippet = snippet[:117] + "..."
							}
							report.CallSites = append(report.CallSites, CallSite{
								CallerFile: f,
								LineNumber: lineNo,
								Snippet:    snippet,
							})
							affectedMap[f] = true
						}
					}
				}
			}

			for f := range affectedMap {
				report.AffectedFiles = append(report.AffectedFiles, f)
			}
			sort.Strings(report.AffectedFiles)
		}
	}

	return report, nil
}

func (a *DefaultImpactAnalyzer) parseUnifiedDiff(diffText string) []DiffFileHunks {
	var result []DiffFileHunks
	scanner := bufio.NewScanner(strings.NewReader(diffText))

	var current *DiffFileHunks
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "diff --git ") {
			if current != nil {
				result = append(result, *current)
			}
			current = &DiffFileHunks{
				RemovedLines: make([]string, 0),
				AddedLines:   make([]string, 0),
			}
		} else if strings.HasPrefix(line, "--- ") {
			if current != nil {
				current.OldPath = strings.TrimSpace(strings.TrimPrefix(line, "--- "))
			}
		} else if strings.HasPrefix(line, "+++ ") {
			if current != nil {
				current.NewPath = strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
			}
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			if current != nil {
				current.RemovedLines = append(current.RemovedLines, strings.TrimPrefix(line, "-"))
			}
		} else if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			if current != nil {
				current.AddedLines = append(current.AddedLines, strings.TrimPrefix(line, "+"))
			}
		}
	}
	if current != nil {
		result = append(result, *current)
	}
	return result
}

func (a *DefaultImpactAnalyzer) detectSymbolChanges(filePath string, hunk DiffFileHunks) []SymbolChange {
	var changes []SymbolChange
	ext := strings.ToLower(filepath.Ext(filePath))

	oldSymbols := a.extractSymbols(ext, hunk.RemovedLines)
	newSymbols := a.extractSymbols(ext, hunk.AddedLines)

	for symName, newSig := range newSymbols {
		oldSig, existed := oldSymbols[symName]
		if existed {
			if oldSig != newSig {
				isBreaking := a.checkIfBreaking(oldSig, newSig)
				changes = append(changes, SymbolChange{
					SymbolName:   symName,
					FilePath:     filePath,
					OldSignature: oldSig,
					NewSignature: newSig,
					IsBreaking:   isBreaking,
				})
			}
		} else {
			// Newly added symbol - generally non-breaking unless modifying interface
			changes = append(changes, SymbolChange{
				SymbolName:   symName,
				FilePath:     filePath,
				OldSignature: "",
				NewSignature: newSig,
				IsBreaking:   false,
			})
		}
	}

	// Check for deleted symbols (breaking change)
	for symName, oldSig := range oldSymbols {
		if _, exists := newSymbols[symName]; !exists {
			changes = append(changes, SymbolChange{
				SymbolName:   symName,
				FilePath:     filePath,
				OldSignature: oldSig,
				NewSignature: "",
				IsBreaking:   true,
			})
		}
	}

	return changes
}

func (a *DefaultImpactAnalyzer) extractSymbols(ext string, lines []string) map[string]string {
	symbols := make(map[string]string)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch ext {
		case ".go":
			if m := a.goFuncRegex.FindStringSubmatch(trimmed); len(m) > 1 {
				name := m[1]
				symbols[name] = trimmed
			}
		case ".py":
			if m := a.pyDefRegex.FindStringSubmatch(trimmed); len(m) > 1 {
				name := m[1]
				symbols[name] = trimmed
			}
		case ".ts", ".js", ".tsx", ".jsx":
			if m := a.tsFuncRegex.FindStringSubmatch(trimmed); len(m) > 2 {
				name := m[1]
				if name == "" {
					name = m[2]
				}
				if name != "" {
					symbols[name] = trimmed
				}
			}
		case ".rs":
			if m := a.rustFnRegex.FindStringSubmatch(trimmed); len(m) > 1 {
				name := m[1]
				symbols[name] = trimmed
			}
		case ".java", ".cpp", ".cc", ".h":
			if m := a.javaMethodReg.FindStringSubmatch(trimmed); len(m) > 1 {
				name := m[1]
				symbols[name] = trimmed
			}
		}
	}

	return symbols
}

func (a *DefaultImpactAnalyzer) checkIfBreaking(oldSig, newSig string) bool {
	// 1. Signature completely removed
	if newSig == "" && oldSig != "" {
		return true
	}

	// 2. Count parameters
	oldParams := extractParamSlice(oldSig)
	newParams := extractParamSlice(newSig)

	if len(oldParams) != len(newParams) {
		return true
	}

	// 3. Check for parameter type or name mismatches
	for i := range oldParams {
		if oldParams[i] != newParams[i] {
			return true
		}
	}

	return false
}

func extractParamSlice(sig string) []string {
	open := strings.Index(sig, "(")
	close := strings.LastIndex(sig, ")")
	if open == -1 || close == -1 || close <= open {
		return nil
	}
	paramStr := strings.TrimSpace(sig[open+1 : close])
	if paramStr == "" {
		return []string{}
	}
	parts := strings.Split(paramStr, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		res = append(res, strings.TrimSpace(p))
	}
	return res
}

func (a *DefaultImpactAnalyzer) getCallRegex(symbolName string) *regexp.Regexp {
	if reg, exists := a.callSiteRegexes[symbolName]; exists {
		return reg
	}
	// Matches symbolName( or .symbolName(
	reg := regexp.MustCompile(`(?:\b` + regexp.QuoteMeta(symbolName) + `\s*\()`)
	a.callSiteRegexes[symbolName] = reg
	return reg
}

func (a *DefaultImpactAnalyzer) isIgnoredPath(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, "vendor/") ||
		strings.Contains(lower, "node_modules/") ||
		strings.Contains(lower, ".git/") ||
		strings.HasSuffix(lower, "_test.go") ||
		strings.HasSuffix(lower, ".test.ts") ||
		strings.HasSuffix(lower, ".spec.ts")
}
