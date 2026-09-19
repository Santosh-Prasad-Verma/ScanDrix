// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package knowledge

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/google/uuid"
)

var (
	// Language-agnostic regexes for TypeScript/JavaScript/Python
	tsFuncRegex   = regexp.MustCompile(`(?m)^(?:export\s+)?(?:async\s+)?function\s+([a-zA-Z0-9_]+)\s*\((.*?)\)`)
	tsClassRegex  = regexp.MustCompile(`(?m)^(?:export\s+)?class\s+([a-zA-Z0-9_]+)`)
	tsMethodRegex = regexp.MustCompile(`(?m)^\s+(?:public|private|protected|async)?\s*([a-zA-Z0-9_]+)\s*\((.*?)\)`)
	pyFuncRegex   = regexp.MustCompile(`(?m)^(?:\s*)def\s+([a-zA-Z0-9_]+)\s*\((.*?)\):`)
	pyClassRegex  = regexp.MustCompile(`(?m)^class\s+([a-zA-Z0-9_]+)`)
	callRegex     = regexp.MustCompile(`\b([a-zA-Z_]\w*)\s*\(`)
)

// SymbolIndex maintains an in-memory repository of symbol definitions and reference graphs.
type SymbolIndex struct {
	definitionsByName map[string][]SymbolDefinition
	definitionsByFile map[string][]SymbolDefinition
	referencesBySym   map[string][]SymbolReference
	referencesByCall  map[string][]SymbolReference
	mu                sync.RWMutex
}

// NewSymbolIndex constructs an empty symbol index.
func NewSymbolIndex() *SymbolIndex {
	return &SymbolIndex{
		definitionsByName: make(map[string][]SymbolDefinition),
		definitionsByFile: make(map[string][]SymbolDefinition),
		referencesBySym:   make(map[string][]SymbolReference),
		referencesByCall:  make(map[string][]SymbolReference),
	}
}

// AddDefinition indexes a symbol definition.
func (idx *SymbolIndex) AddDefinition(def SymbolDefinition) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	cleanFile := filepath.ToSlash(filepath.Clean(def.Location.FilePath))
	def.Location.FilePath = cleanFile

	idx.definitionsByName[def.Name] = append(idx.definitionsByName[def.Name], def)
	idx.definitionsByFile[cleanFile] = append(idx.definitionsByFile[cleanFile], def)
}

// AddReference indexes a symbol usage or call site.
func (idx *SymbolIndex) AddReference(ref SymbolReference) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	cleanFile := filepath.ToSlash(filepath.Clean(ref.Location.FilePath))
	ref.Location.FilePath = cleanFile

	idx.referencesBySym[ref.SymbolName] = append(idx.referencesBySym[ref.SymbolName], ref)
	if ref.CallerName != "" {
		idx.referencesByCall[ref.CallerName] = append(idx.referencesByCall[ref.CallerName], ref)
	}
}

// GetDefinition retrieves definitions matching the given symbol name.
func (idx *SymbolIndex) GetDefinition(name string) ([]SymbolDefinition, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	defs, ok := idx.definitionsByName[name]
	return defs, ok
}

// GetDefinitionsInFile retrieves all symbols defined within a file.
func (idx *SymbolIndex) GetDefinitionsInFile(filePath string) []SymbolDefinition {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	cleanFile := filepath.ToSlash(filepath.Clean(filePath))
	return idx.definitionsByFile[cleanFile]
}

// RemoveFile removes all definitions and references originating from the specified file.
// It returns the slice of deleted SymbolDefinitions.
func (idx *SymbolIndex) RemoveFile(filePath string) []SymbolDefinition {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	cleanFile := filepath.ToSlash(filepath.Clean(filePath))
	removedDefs := idx.definitionsByFile[cleanFile]
	delete(idx.definitionsByFile, cleanFile)

	// Remove from definitionsByName
	for _, def := range removedDefs {
		defs := idx.definitionsByName[def.Name]
		filtered := make([]SymbolDefinition, 0, len(defs))
		for _, d := range defs {
			if filepath.ToSlash(filepath.Clean(d.Location.FilePath)) != cleanFile {
				filtered = append(filtered, d)
			}
		}
		if len(filtered) == 0 {
			delete(idx.definitionsByName, def.Name)
		} else {
			idx.definitionsByName[def.Name] = filtered
		}
	}

	// Remove references originating in cleanFile
	for symName, refs := range idx.referencesBySym {
		filtered := make([]SymbolReference, 0, len(refs))
		for _, r := range refs {
			if filepath.ToSlash(filepath.Clean(r.Location.FilePath)) != cleanFile {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			delete(idx.referencesBySym, symName)
		} else {
			idx.referencesBySym[symName] = filtered
		}
	}

	for callerName, refs := range idx.referencesByCall {
		filtered := make([]SymbolReference, 0, len(refs))
		for _, r := range refs {
			if filepath.ToSlash(filepath.Clean(r.Location.FilePath)) != cleanFile {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			delete(idx.referencesByCall, callerName)
		} else {
			idx.referencesByCall[callerName] = filtered
		}
	}

	return removedDefs
}

// GetReferencesTo retrieves call sites targeting the given symbol.
func (idx *SymbolIndex) GetReferencesTo(symbolName string) []SymbolReference {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return idx.referencesBySym[symbolName]
}

// FindCallers returns distinct caller function/method names targeting the symbol.
func (idx *SymbolIndex) FindCallers(symbolName string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	refs := idx.referencesBySym[symbolName]
	callerSet := make(map[string]struct{})
	for _, r := range refs {
		if r.CallerName != "" && r.CallerName != symbolName {
			callerSet[r.CallerName] = struct{}{}
		}
	}

	callers := make([]string, 0, len(callerSet))
	for c := range callerSet {
		callers = append(callers, c)
	}
	return callers
}

// FindCallees returns distinct function/method names called by the given caller.
func (idx *SymbolIndex) FindCallees(callerName string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	refs := idx.referencesByCall[callerName]
	calleeSet := make(map[string]struct{})
	for _, r := range refs {
		if r.SymbolName != "" && r.SymbolName != callerName {
			calleeSet[r.SymbolName] = struct{}{}
		}
	}

	callees := make([]string, 0, len(calleeSet))
	for c := range calleeSet {
		callees = append(callees, c)
	}
	return callees
}

// IndexFile parses the file according to its language extension and indexes its definitions and calls.
func (idx *SymbolIndex) IndexFile(filePath string, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		idx.indexGoFile(filePath, content)
	case ".ts", ".tsx", ".js", ".jsx":
		idx.indexTypeScriptFile(filePath, content)
	case ".py":
		idx.indexPythonFile(filePath, content)
	default:
		// Fallback simple line-based regex indexer
		idx.indexGenericFile(filePath, content)
	}
}

// indexGoFile uses Go's standard library AST parser for high precision.
func (idx *SymbolIndex) indexGoFile(filePath string, content string) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, content, parser.ParseComments)
	if err != nil {
		idx.indexGenericFile(filePath, content)
		return
	}

	pkgName := node.Name.Name

	// Walk declarations
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			funcName := d.Name.Name
			startPos := fset.Position(d.Pos())
			endPos := fset.Position(d.End())

			kind := KindFunction
			var receiver string
			if d.Recv != nil && len(d.Recv.List) > 0 {
				kind = KindMethod
				receiver = typeString(d.Recv.List[0].Type)
			}

			isExported := unicode.IsUpper(rune(funcName[0]))

			doc := ""
			if d.Doc != nil {
				doc = d.Doc.Text()
			}

			def := SymbolDefinition{
				ID:       uuid.New(),
				Name:     funcName,
				Package:  pkgName,
				Kind:     kind,
				Location: SymbolLocation{FilePath: filePath, StartLine: startPos.Line, EndLine: endPos.Line},
				Docstring: strings.TrimSpace(doc),
				Exported: isExported,
				Language: "go",
				Receiver: receiver,
			}
			idx.AddDefinition(def)

			// Walk body to extract call sites
			if d.Body != nil {
				ast.Inspect(d.Body, func(n ast.Node) bool {
					if call, ok := n.(*ast.CallExpr); ok {
						calleeName := exprName(call.Fun)
						if calleeName != "" {
							callPos := fset.Position(call.Pos())
							idx.AddReference(SymbolReference{
								SymbolName: calleeName,
								CallerName: funcName,
								Location: SymbolLocation{
									FilePath:  filePath,
									StartLine: callPos.Line,
									EndLine:   callPos.Line,
								},
							})
						}
					}
					return true
				})
			}

		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						typeName := typeSpec.Name.Name
						startPos := fset.Position(typeSpec.Pos())
						endPos := fset.Position(typeSpec.End())

						kind := KindType
						switch typeSpec.Type.(type) {
						case *ast.StructType:
							kind = KindStruct
						case *ast.InterfaceType:
							kind = KindInterface
						}

						idx.AddDefinition(SymbolDefinition{
							ID:       uuid.New(),
							Name:     typeName,
							Package:  pkgName,
							Kind:     kind,
							Location: SymbolLocation{FilePath: filePath, StartLine: startPos.Line, EndLine: endPos.Line},
							Exported: unicode.IsUpper(rune(typeName[0])),
							Language: "go",
						})
					}
				}
			}
		}
	}
}

func typeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return typeString(t.X)
	default:
		return ""
	}
}

func exprName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	default:
		return ""
	}
}

// indexTypeScriptFile extracts definitions and calls in TypeScript/JavaScript.
func (idx *SymbolIndex) indexTypeScriptFile(filePath string, content string) {
	lines := strings.Split(content, "\n")
	var currentFunc string

	for lineIdx, line := range lines {
		lineNo := lineIdx + 1

		// Function definition
		if m := tsFuncRegex.FindStringSubmatch(line); len(m) > 1 {
			funcName := m[1]
			currentFunc = funcName
			idx.AddDefinition(SymbolDefinition{
				ID:       uuid.New(),
				Name:     funcName,
				Kind:     KindFunction,
				Location: SymbolLocation{FilePath: filePath, StartLine: lineNo, EndLine: lineNo},
				Exported: strings.Contains(line, "export"),
				Language: "typescript",
			})
			continue
		}

		// Class definition
		if m := tsClassRegex.FindStringSubmatch(line); len(m) > 1 {
			className := m[1]
			idx.AddDefinition(SymbolDefinition{
				ID:       uuid.New(),
				Name:     className,
				Kind:     KindClass,
				Location: SymbolLocation{FilePath: filePath, StartLine: lineNo, EndLine: lineNo},
				Exported: strings.Contains(line, "export"),
				Language: "typescript",
			})
			continue
		}

		// Method definition inside class
		if m := tsMethodRegex.FindStringSubmatch(line); len(m) > 1 {
			methodName := m[1]
			currentFunc = methodName
			idx.AddDefinition(SymbolDefinition{
				ID:       uuid.New(),
				Name:     methodName,
				Kind:     KindMethod,
				Location: SymbolLocation{FilePath: filePath, StartLine: lineNo, EndLine: lineNo},
				Language: "typescript",
			})
			continue
		}

		// Call sites
		for _, m := range callRegex.FindAllStringSubmatch(line, -1) {
			if len(m) > 1 {
				callee := m[1]
				if callee != "if" && callee != "for" && callee != "while" && callee != "switch" {
					idx.AddReference(SymbolReference{
						SymbolName: callee,
						CallerName: currentFunc,
						Location:   SymbolLocation{FilePath: filePath, StartLine: lineNo, EndLine: lineNo},
					})
				}
			}
		}
	}
}

// indexPythonFile extracts definitions and calls in Python.
func (idx *SymbolIndex) indexPythonFile(filePath string, content string) {
	lines := strings.Split(content, "\n")
	var currentFunc string

	for lineIdx, line := range lines {
		lineNo := lineIdx + 1

		if m := pyFuncRegex.FindStringSubmatch(line); len(m) > 1 {
			funcName := m[1]
			currentFunc = funcName
			idx.AddDefinition(SymbolDefinition{
				ID:       uuid.New(),
				Name:     funcName,
				Kind:     KindFunction,
				Location: SymbolLocation{FilePath: filePath, StartLine: lineNo, EndLine: lineNo},
				Language: "python",
			})
			continue
		}

		if m := pyClassRegex.FindStringSubmatch(line); len(m) > 1 {
			className := m[1]
			idx.AddDefinition(SymbolDefinition{
				ID:       uuid.New(),
				Name:     className,
				Kind:     KindClass,
				Location: SymbolLocation{FilePath: filePath, StartLine: lineNo, EndLine: lineNo},
				Language: "python",
			})
			continue
		}

		for _, m := range callRegex.FindAllStringSubmatch(line, -1) {
			if len(m) > 1 {
				callee := m[1]
				if callee != "if" && callee != "for" && callee != "while" && callee != "print" {
					idx.AddReference(SymbolReference{
						SymbolName: callee,
						CallerName: currentFunc,
						Location:   SymbolLocation{FilePath: filePath, StartLine: lineNo, EndLine: lineNo},
					})
				}
			}
		}
	}
}

// indexGenericFile performs heuristic fallback indexing for other languages.
func (idx *SymbolIndex) indexGenericFile(filePath string, content string) {
	lines := strings.Split(content, "\n")
	for lineIdx, line := range lines {
		lineNo := lineIdx + 1
		for _, m := range callRegex.FindAllStringSubmatch(line, -1) {
			if len(m) > 1 {
				callee := m[1]
				idx.AddReference(SymbolReference{
					SymbolName: callee,
					Location:   SymbolLocation{FilePath: filePath, StartLine: lineNo, EndLine: lineNo},
				})
			}
		}
	}
}
