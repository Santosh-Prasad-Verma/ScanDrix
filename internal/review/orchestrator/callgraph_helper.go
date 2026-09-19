// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package orchestrator

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// CallGraphLimits bounds context assembly to fit model token budgets.
type CallGraphLimits struct {
	MaxCallGraphChars       int
	MaxChangedFiles         int
	MaxFunctionsPerFile     int
	MaxTotalFunctions       int
	MaxCallersPerFunction   int
	MaxCalleesPerFunction   int
	MaxAssembledContextChars int
	ChangedSnippetRadius    int
	RelatedSnippetRadius    int
}

// DefaultCallGraphLimits returns production token-balanced limits.
func DefaultCallGraphLimits() CallGraphLimits {
	return CallGraphLimits{
		MaxCallGraphChars:       6000,
		MaxChangedFiles:         20,
		MaxFunctionsPerFile:     15,
		MaxTotalFunctions:       50,
		MaxCallersPerFunction:   4,
		MaxCalleesPerFunction:   4,
		MaxAssembledContextChars: 9000,
		ChangedSnippetRadius:    10,
		RelatedSnippetRadius:    8,
	}
}

// FunctionDefinition records a discovered function or method signature.
type FunctionDefinition struct {
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Signature string `json:"signature"`
	Body      string `json:"body,omitempty"`
}

// CallSite records a location where a function is invoked.
type CallSite struct {
	CallerName string `json:"caller_name"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Snippet    string `json:"snippet"`
}

// CallGraphEntry models relationships for a single function.
type CallGraphEntry struct {
	Definition FunctionDefinition `json:"definition"`
	Callers    []CallSite         `json:"callers"`
	Callees    []string           `json:"callees"`
}

// CallGraphHelper analyzes cross-file symbols and invocation graphs.
type CallGraphHelper struct {
	limits        CallGraphLimits
	noiseWords    map[string]struct{}
	nameRegexes   []*regexp.Regexp
	callRegex     *regexp.Regexp
}

// NewCallGraphHelper constructs an AST/symbol call graph analyzer.
func NewCallGraphHelper(limits ...CallGraphLimits) *CallGraphHelper {
	lim := DefaultCallGraphLimits()
	if len(limits) > 0 {
		lim = limits[0]
	}

	noiseList := []string{
		"if", "for", "while", "return", "new", "var", "let", "const", "get", "set",
		"run", "main", "init", "test", "string", "bool", "int", "uint", "error",
		"nil", "null", "void", "self", "this", "super", "type", "interface", "struct",
		"enum", "module", "package", "import", "from", "with", "true", "false",
		"create", "delete", "update", "read", "write", "close", "open", "start",
		"stop", "send", "handle", "process", "execute", "apply", "call", "toString",
		"equals", "hashCode", "valueOf", "validate", "render", "display", "show", "hide",
	}
	noiseMap := make(map[string]struct{}, len(noiseList))
	for _, w := range noiseList {
		noiseMap[strings.ToLower(w)] = struct{}{}
	}

	return &CallGraphHelper{
		limits:     lim,
		noiseWords: noiseMap,
		nameRegexes: []*regexp.Regexp{
			// Go: func (r *Receiver) MethodName( or func FuncName(
			regexp.MustCompile(`func\s+(?:\([^)]+\)\s+)?([A-Za-z0-9_]+)\s*\(`),
			// TS/JS/Python/PHP/Rust: def foo(, function foo(, fn foo(
			regexp.MustCompile(`(?:def|func|fn|function|class)\s+([A-Za-z0-9_]+)`),
			// Java/C#/C++: public void Foo( or async Task<T> Bar(
			regexp.MustCompile(`(?:public|private|protected)\s+(?:static\s+)?(?:async\s+)?(?:[\w<>\[\]]+\s+)+([A-Za-z0-9_]+)\s*\(`),
			// Exported const/let functions: export const foo = (
			regexp.MustCompile(`export\s+(?:default\s+)?(?:const|let|var)\s+([A-Za-z0-9_]+)\s*=\s*(?:async\s*)?\(`),
		},
		// Function invocation pattern: fooBar(
		callRegex: regexp.MustCompile(`\b([A-Za-z0-9_]{3,})\s*\(`),
	}
}

// ExtractDefinitions scans file contents and returns all declared functions.
func (h *CallGraphHelper) ExtractDefinitions(filename, content string) []FunctionDefinition {
	if strings.TrimSpace(content) == "" {
		return nil
	}

	lines := strings.Split(content, "\n")
	var defs []FunctionDefinition

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) == 0 || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		for _, reg := range h.nameRegexes {
			matches := reg.FindStringSubmatch(line)
			if len(matches) > 1 {
				name := matches[1]
				if _, isNoise := h.noiseWords[strings.ToLower(name)]; !isNoise {
					shortName := name
					if parts := strings.Split(name, "."); len(parts) > 1 {
						shortName = parts[len(parts)-1]
					}

					defs = append(defs, FunctionDefinition{
						Name:      name,
						ShortName: shortName,
						File:      filename,
						Line:      idx + 1,
						Signature: strings.TrimSpace(line),
					})
					break
				}
			}
		}

		if len(defs) >= h.limits.MaxFunctionsPerFile {
			break
		}
	}

	return defs
}

// ExtractCallSites scans content for calls to any of the target functions.
func (h *CallGraphHelper) ExtractCallSites(callerFile, content string, targets map[string]FunctionDefinition) []CallSite {
	if strings.TrimSpace(content) == "" || len(targets) == 0 {
		return nil
	}

	lines := strings.Split(content, "\n")
	var sites []CallSite

	for idx, line := range lines {
		matches := h.callRegex.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if len(m) > 1 {
				callee := m[1]
				if def, exists := targets[callee]; exists {
					// Avoid self-calls within the same line
					if callerFile == def.File && idx+1 == def.Line {
						continue
					}

					snippet := strings.TrimSpace(line)
					if len(snippet) > 120 {
						snippet = snippet[:117] + "..."
					}

					sites = append(sites, CallSite{
						CallerName: callee,
						File:       callerFile,
						Line:       idx + 1,
						Snippet:    snippet,
					})
				}
			}
		}
	}

	return sites
}

// BuildCallGraph constructs the caller/callee matrix across all changed files.
func (h *CallGraphHelper) BuildCallGraph(files []ChangedFile) map[string]*CallGraphEntry {
	graph := make(map[string]*CallGraphEntry)
	allDefs := make(map[string]FunctionDefinition)

	// Step 1: Discover all function declarations
	fileCount := 0
	for _, f := range files {
		if f.IsBinary || f.IsVendored || f.IsGenerated {
			continue
		}
		fileCount++
		if fileCount > h.limits.MaxChangedFiles {
			break
		}

		content := f.Content
		if content == "" {
			content = f.Patch
		}

		defs := h.ExtractDefinitions(f.Filename, content)
		for _, def := range defs {
			if len(allDefs) >= h.limits.MaxTotalFunctions {
				break
			}
			allDefs[def.ShortName] = def
			graph[def.ShortName] = &CallGraphEntry{
				Definition: def,
				Callers:    make([]CallSite, 0),
				Callees:    make([]string, 0),
			}
		}
	}

	// Step 2: Discover call sites across files
	for _, f := range files {
		content := f.Content
		if content == "" {
			content = f.Patch
		}

		callSites := h.ExtractCallSites(f.Filename, content, allDefs)
		for _, site := range callSites {
			if entry, ok := graph[site.CallerName]; ok {
				if len(entry.Callers) < h.limits.MaxCallersPerFunction {
					entry.Callers = append(entry.Callers, site)
				}
			}
		}
	}

	// Step 3: Populate callees
	for funcName, entry := range graph {
		for _, callerEntry := range graph {
			for _, callerSite := range callerEntry.Callers {
				if callerSite.File == entry.Definition.File {
					// Check if callerEntry is a callee of entry
					if !containsString(entry.Callees, callerSite.CallerName) && callerSite.CallerName != funcName {
						if len(entry.Callees) < h.limits.MaxCalleesPerFunction {
							entry.Callees = append(entry.Callees, callerSite.CallerName)
						}
					}
				}
			}
		}
	}

	return graph
}

// AssembleCallGraphPrompt builds a structured, Markdown-formatted prompt component
// summarizing cross-file relationships for injection into LLM specialist contexts.
func (h *CallGraphHelper) AssembleCallGraphPrompt(graph map[string]*CallGraphEntry) string {
	if len(graph) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### Cross-File Architectural Call Graph & Dependencies:\n")
	sb.WriteString("The following function call relationships were discovered across the changed files:\n\n")

	// Sort keys for deterministic prompt output
	keys := make([]string, 0, len(graph))
	for k := range graph {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	renderedFunctions := 0
	for _, name := range keys {
		entry := graph[name]
		if len(entry.Callers) == 0 && len(entry.Callees) == 0 {
			// Omit isolated functions to save tokens
			continue
		}

		sb.WriteString(fmt.Sprintf("- **`%s`** (`%s:%d`)\n", entry.Definition.ShortName, entry.Definition.File, entry.Definition.Line))
		if entry.Definition.Signature != "" {
			sb.WriteString(fmt.Sprintf("  - Signature: `%s`\n", entry.Definition.Signature))
		}

		if len(entry.Callers) > 0 {
			sb.WriteString("  - Invoked by:\n")
			for _, c := range entry.Callers {
				sb.WriteString(fmt.Sprintf("    - `%s:%d` -> `%s`\n", c.File, c.Line, c.Snippet))
			}
		}

		if len(entry.Callees) > 0 {
			sb.WriteString(fmt.Sprintf("  - Calls: `%s`\n", strings.Join(entry.Callees, "`, `")))
		}

		renderedFunctions++
		if sb.Len() > h.limits.MaxCallGraphChars {
			sb.WriteString("\n[... additional call graph relationships omitted for context budget ...]\n")
			break
		}
	}

	if renderedFunctions == 0 {
		return ""
	}

	return sb.String()
}

func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
