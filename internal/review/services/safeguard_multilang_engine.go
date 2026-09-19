package services

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// SupportedLanguage identifies programming languages verified by the AST safeguard engine.
type SupportedLanguage string

const (
	LangGo         SupportedLanguage = "go"
	LangTypeScript SupportedLanguage = "typescript"
	LangJavaScript SupportedLanguage = "javascript"
	LangPython     SupportedLanguage = "python"
	LangJava       SupportedLanguage = "java"
	LangCpp        SupportedLanguage = "cpp"
	LangRust       SupportedLanguage = "rust"
	LangCSharp     SupportedLanguage = "csharp"
	LangPHP        SupportedLanguage = "php"
	LangRuby       SupportedLanguage = "ruby"
	LangKotlin     SupportedLanguage = "kotlin"
	LangSwift      SupportedLanguage = "swift"
	LangScala      SupportedLanguage = "scala"
	LangUnknown    SupportedLanguage = "unknown"
)

// ASTSymbol represents a declared identifier in a source file.
type ASTSymbol struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"` // "function", "class", "interface", "variable", "import", "type"
	Line      int    `json:"line"`
	IsExported bool  `json:"is_exported"`
}

// ASTFileIndex holds all extracted symbols and imports for a single source file.
type ASTFileIndex struct {
	FilePath string
	Language SupportedLanguage
	Symbols  map[string]ASTSymbol
	Imports  map[string]bool
}

// ReplacementSyntaxReport captures syntax and hallucination check findings.
type ReplacementSyntaxReport struct {
	IsValid             bool     `json:"is_valid"`
	SyntaxErrors        []string `json:"syntax_errors,omitempty"`
	HallucinatedSymbols []string `json:"hallucinated_symbols,omitempty"`
	IndentationIssue    string   `json:"indentation_issue,omitempty"`
	LineEndingIssue     string   `json:"line_ending_issue,omitempty"`
}

// SafeguardMultiLangEngine performs AST boundary verification and hallucination detection across 12 languages.
type SafeguardMultiLangEngine struct {
	mu           sync.RWMutex
	builtins     map[SupportedLanguage]map[string]bool
	keywordMap   map[SupportedLanguage]map[string]bool
	identRegex   *regexp.Regexp
}

// NewSafeguardMultiLangEngine constructs an engine initialized with prelude tables for 12 languages.
func NewSafeguardMultiLangEngine() *SafeguardMultiLangEngine {
	e := &SafeguardMultiLangEngine{
		builtins:   make(map[SupportedLanguage]map[string]bool),
		keywordMap: make(map[SupportedLanguage]map[string]bool),
		identRegex: regexp.MustCompile(`\b[a-zA-Z_][a-zA-Z0-9_]*\b`),
	}
	e.initLanguagePreludes()
	return e
}

func (e *SafeguardMultiLangEngine) initLanguagePreludes() {
	// Go
	e.builtins[LangGo] = toMap([]string{
		"append", "cap", "close", "complex", "copy", "delete", "imag", "len",
		"make", "new", "panic", "print", "println", "real", "recover",
		"bool", "byte", "complex64", "complex128", "error", "float32", "float64",
		"int", "int8", "int16", "int32", "int64", "rune", "string",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"true", "false", "iota", "nil",
		"fmt", "strings", "errors", "time", "context", "sync", "os", "io",
	})
	e.keywordMap[LangGo] = toMap([]string{
		"break", "case", "chan", "const", "continue", "default", "defer",
		"else", "fallthrough", "for", "func", "go", "goto", "if", "import",
		"interface", "map", "package", "range", "return", "select", "struct",
		"switch", "type", "var",
	})

	// TypeScript / JavaScript
	tsBuiltins := toMap([]string{
		"console", "Promise", "Array", "Object", "String", "Number", "Boolean",
		"Date", "RegExp", "Error", "Map", "Set", "WeakMap", "WeakSet", "JSON",
		"Math", "Intl", "Symbol", "BigInt", "Proxy", "Reflect", "ArrayBuffer",
		"Uint8Array", "Int32Array", "Float64Array", "undefined", "null", "NaN",
		"Infinity", "isNaN", "isFinite", "parseInt", "parseFloat", "encodeURI",
		"decodeURI", "encodeURIComponent", "decodeURIComponent", "require", "process",
		"setTimeout", "clearTimeout", "setInterval", "clearInterval", "fetch",
	})
	e.builtins[LangTypeScript] = tsBuiltins
	e.builtins[LangJavaScript] = tsBuiltins
	tsKeywords := toMap([]string{
		"break", "case", "catch", "class", "const", "continue", "debugger",
		"default", "delete", "do", "else", "export", "extends", "finally",
		"for", "function", "if", "import", "in", "instanceof", "new", "return",
		"super", "switch", "this", "throw", "try", "typeof", "var", "void",
		"while", "with", "yield", "let", "static", "enum", "await", "async",
		"implements", "interface", "package", "private", "protected", "public",
		"type", "as", "from", "of",
	})
	e.keywordMap[LangTypeScript] = tsKeywords
	e.keywordMap[LangJavaScript] = tsKeywords

	// Python
	e.builtins[LangPython] = toMap([]string{
		"abs", "all", "any", "ascii", "bin", "bool", "breakpoint", "bytearray",
		"bytes", "callable", "chr", "classmethod", "compile", "complex", "delattr",
		"dict", "dir", "divmod", "enumerate", "eval", "exec", "filter", "float",
		"format", "frozenset", "getattr", "globals", "hasattr", "hash", "help",
		"hex", "id", "input", "int", "isinstance", "issubclass", "iter", "len",
		"list", "locals", "map", "max", "memoryview", "min", "next", "object",
		"oct", "open", "ord", "pow", "print", "property", "range", "repr",
		"reversed", "round", "set", "setattr", "slice", "sorted", "staticmethod",
		"str", "sum", "super", "tuple", "type", "vars", "zip", "__import__",
		"True", "False", "None", "self", "cls",
		"os", "sys", "json", "re", "math", "time", "datetime", "typing", "collections",
	})
	e.keywordMap[LangPython] = toMap([]string{
		"and", "as", "assert", "async", "await", "break", "class", "continue",
		"def", "del", "elif", "else", "except", "finally", "for", "from",
		"global", "if", "import", "in", "is", "lambda", "nonlocal", "not",
		"or", "pass", "raise", "return", "try", "while", "with", "yield",
	})

	// Java
	e.builtins[LangJava] = toMap([]string{
		"System", "String", "Object", "Class", "Math", "Integer", "Long", "Double",
		"Float", "Boolean", "Byte", "Short", "Character", "StringBuilder", "StringBuffer",
		"Thread", "Runnable", "Throwable", "Exception", "RuntimeException", "Error",
		"Iterable", "Collection", "List", "ArrayList", "LinkedList", "Set", "HashSet",
		"Map", "HashMap", "Arrays", "Collections", "Objects", "Optional", "Stream",
		"true", "false", "null",
	})
	e.keywordMap[LangJava] = toMap([]string{
		"abstract", "assert", "boolean", "break", "byte", "case", "catch", "char",
		"class", "const", "continue", "default", "do", "double", "else", "enum",
		"extends", "final", "finally", "float", "for", "goto", "if", "implements",
		"import", "instanceof", "int", "interface", "long", "native", "new",
		"package", "private", "protected", "public", "return", "short", "static",
		"strictfp", "super", "switch", "synchronized", "this", "throw", "throws",
		"transient", "try", "void", "volatile", "while", "record",
	})

	// C++
	e.builtins[LangCpp] = toMap([]string{
		"std", "cout", "cin", "cerr", "endl", "vector", "string", "map", "set",
		"unordered_map", "unordered_set", "pair", "tuple", "shared_ptr", "unique_ptr",
		"weak_ptr", "make_shared", "make_unique", "size_t", "nullptr", "true", "false",
		"printf", "scanf", "malloc", "free", "memcpy", "memset",
	})
	e.keywordMap[LangCpp] = toMap([]string{
		"auto", "bool", "break", "case", "catch", "char", "class", "const",
		"constexpr", "continue", "default", "delete", "do", "double", "else",
		"enum", "explicit", "export", "extern", "false", "float", "for", "friend",
		"goto", "if", "inline", "int", "long", "mutable", "namespace", "new",
		"noexcept", "nullptr", "operator", "private", "protected", "public",
		"register", "reinterpret_cast", "return", "short", "signed", "sizeof",
		"static", "static_assert", "static_cast", "struct", "switch", "template",
		"this", "thread_local", "throw", "true", "try", "typedef", "typeid",
		"typename", "union", "unsigned", "using", "virtual", "void", "volatile",
		"while",
	})

	// Rust
	e.builtins[LangRust] = toMap([]string{
		"Option", "Some", "None", "Result", "Ok", "Err", "Vec", "String", "str",
		"Box", "Rc", "Arc", "Cell", "RefCell", "Mutex", "RwLock", "Clone", "Copy",
		"Debug", "Default", "Drop", "Fn", "FnMut", "FnOnce", "Iterator", "Into",
		"From", "AsRef", "println", "eprintln", "format", "panic", "vec", "assert",
		"assert_eq", "assert_ne", "todo", "unimplemented", "unreachable",
		"true", "false", "self", "Self",
	})
	e.keywordMap[LangRust] = toMap([]string{
		"as", "break", "const", "continue", "crate", "else", "enum", "extern",
		"false", "fn", "for", "if", "impl", "in", "let", "loop", "match", "mod",
		"move", "mut", "pub", "ref", "return", "self", "Self", "static", "struct",
		"super", "trait", "true", "type", "unsafe", "use", "where", "while",
		"async", "await", "dyn",
	})

	// C#
	e.builtins[LangCSharp] = toMap([]string{
		"Console", "String", "Object", "Int32", "Int64", "Boolean", "Double",
		"List", "Dictionary", "HashSet", "IEnumerable", "Task", "Action", "Func",
		"Math", "DateTime", "TimeSpan", "Guid", "Exception", "Nullable",
		"true", "false", "null",
	})
	e.keywordMap[LangCSharp] = toMap([]string{
		"abstract", "as", "base", "bool", "break", "byte", "case", "catch", "char",
		"checked", "class", "const", "continue", "decimal", "default", "delegate",
		"do", "double", "else", "enum", "event", "explicit", "extern", "false",
		"finally", "fixed", "float", "for", "foreach", "goto", "if", "implicit",
		"in", "int", "interface", "internal", "is", "lock", "long", "namespace",
		"new", "null", "object", "operator", "out", "override", "params", "private",
		"protected", "public", "readonly", "record", "ref", "return", "sbyte",
		"sealed", "short", "sizeof", "stackalloc", "static", "string", "struct",
		"switch", "this", "throw", "true", "try", "typeof", "uint", "ulong",
		"unchecked", "unsafe", "ushort", "using", "virtual", "void", "volatile",
		"while", "var", "async", "await",
	})

	// PHP
	e.builtins[LangPHP] = toMap([]string{
		"echo", "print", "var_dump", "isset", "empty", "die", "exit", "count",
		"strlen", "strpos", "substr", "in_array", "array_merge", "array_map",
		"array_filter", "json_encode", "json_decode", "sprintf", "Exception",
		"PDO", "DateTime", "true", "false", "null",
	})
	e.keywordMap[LangPHP] = toMap([]string{
		"abstract", "and", "array", "as", "break", "callable", "case", "catch",
		"class", "clone", "const", "continue", "declare", "default", "do", "else",
		"elseif", "enddeclare", "endfor", "endforeach", "endif", "endswitch",
		"endwhile", "extends", "final", "finally", "fn", "for", "foreach",
		"function", "global", "goto", "if", "implements", "include", "include_once",
		"instanceof", "insteadof", "interface", "match", "namespace", "new", "or",
		"print", "private", "protected", "public", "readonly", "require",
		"require_once", "return", "static", "switch", "throw", "trait", "try",
		"use", "var", "while", "xor", "yield",
	})

	// Ruby
	e.builtins[LangRuby] = toMap([]string{
		"puts", "print", "p", "raise", "fail", "require", "require_relative",
		"attr_accessor", "attr_reader", "attr_writer", "include", "extend",
		"Array", "Hash", "String", "Integer", "Float", "Symbol", "Proc", "Lambda",
		"true", "false", "nil", "self",
	})
	e.keywordMap[LangRuby] = toMap([]string{
		"alias", "and", "begin", "break", "case", "class", "def", "defined?",
		"do", "else", "elsif", "end", "ensure", "false", "for", "if", "in",
		"module", "next", "nil", "not", "or", "redo", "rescue", "retry",
		"return", "self", "super", "then", "true", "undef", "unless", "until",
		"when", "while", "yield",
	})

	// Kotlin
	e.builtins[LangKotlin] = toMap([]string{
		"println", "print", "listOf", "mutableListOf", "mapOf", "mutableMapOf",
		"setOf", "mutableSetOf", "arrayOf", "Int", "Long", "Double", "Float",
		"Boolean", "String", "Any", "Unit", "Nothing", "true", "false", "null",
	})
	e.keywordMap[LangKotlin] = toMap([]string{
		"as", "break", "class", "continue", "do", "else", "false", "for", "fun",
		"if", "in", "interface", "is", "null", "object", "package", "return",
		"super", "this", "throw", "true", "try", "typealias", "val", "var",
		"when", "while", "data", "sealed", "suspend", "override",
	})

	// Swift
	e.builtins[LangSwift] = toMap([]string{
		"print", "Int", "Double", "Float", "Bool", "String", "Array", "Dictionary",
		"Set", "Optional", "fatalError", "precondition", "assert", "true", "false", "nil",
	})
	e.keywordMap[LangSwift] = toMap([]string{
		"associatedtype", "class", "deinit", "enum", "extension", "fileprivate",
		"func", "import", "init", "inout", "internal", "let", "open", "operator",
		"private", "protocol", "public", "rethrows", "static", "struct", "subscript",
		"typealias", "var", "break", "case", "continue", "default", "defer",
		"do", "else", "fallthrough", "for", "guard", "if", "in", "repeat",
		"return", "switch", "where", "while", "as", "catch", "throw", "throws",
		"try", "async", "await",
	})

	// Scala
	e.builtins[LangScala] = toMap([]string{
		"println", "print", "List", "Map", "Set", "Seq", "Vector", "Array",
		"Option", "Some", "None", "Either", "Left", "Right", "Try", "Success",
		"Failure", "Future", "Int", "Long", "Double", "Float", "Boolean", "String",
		"Unit", "Any", "true", "false", "null",
	})
	e.keywordMap[LangScala] = toMap([]string{
		"abstract", "case", "catch", "class", "def", "do", "else", "extends",
		"false", "final", "finally", "for", "forSome", "if", "implicit", "import",
		"lazy", "match", "new", "null", "object", "override", "package", "private",
		"protected", "return", "sealed", "super", "this", "throw", "trait",
		"try", "true", "type", "val", "var", "while", "with", "yield",
	})
}

func toMap(items []string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it] = true
	}
	return m
}

// DetectLanguage resolves the language from file extension or explicit metadata.
func DetectLanguage(filePath, explicitLang string) SupportedLanguage {
	if explicitLang != "" {
		switch strings.ToLower(explicitLang) {
		case "go", "golang":
			return LangGo
		case "ts", "typescript":
			return LangTypeScript
		case "js", "javascript":
			return LangJavaScript
		case "py", "python":
			return LangPython
		case "java":
			return LangJava
		case "cpp", "c++", "cxx", "cc":
			return LangCpp
		case "rs", "rust":
			return LangRust
		case "cs", "csharp", "c#":
			return LangCSharp
		case "php":
			return LangPHP
		case "rb", "ruby":
			return LangRuby
		case "kt", "kotlin":
			return LangKotlin
		case "swift":
			return LangSwift
		case "scala":
			return LangScala
		}
	}

	lower := strings.ToLower(filePath)
	switch {
	case strings.HasSuffix(lower, ".go"):
		return LangGo
	case strings.HasSuffix(lower, ".ts"), strings.HasSuffix(lower, ".tsx"):
		return LangTypeScript
	case strings.HasSuffix(lower, ".js"), strings.HasSuffix(lower, ".jsx"), strings.HasSuffix(lower, ".mjs"), strings.HasSuffix(lower, ".cjs"):
		return LangJavaScript
	case strings.HasSuffix(lower, ".py"):
		return LangPython
	case strings.HasSuffix(lower, ".java"):
		return LangJava
	case strings.HasSuffix(lower, ".cpp"), strings.HasSuffix(lower, ".cc"), strings.HasSuffix(lower, ".cxx"), strings.HasSuffix(lower, ".hpp"), strings.HasSuffix(lower, ".h"):
		return LangCpp
	case strings.HasSuffix(lower, ".rs"):
		return LangRust
	case strings.HasSuffix(lower, ".cs"):
		return LangCSharp
	case strings.HasSuffix(lower, ".php"):
		return LangPHP
	case strings.HasSuffix(lower, ".rb"):
		return LangRuby
	case strings.HasSuffix(lower, ".kt"), strings.HasSuffix(lower, ".kts"):
		return LangKotlin
	case strings.HasSuffix(lower, ".swift"):
		return LangSwift
	case strings.HasSuffix(lower, ".scala"):
		return LangScala
	default:
		return LangUnknown
	}
}

// IndexFileSymbols extracts declared functions, classes, types, and variables from the source file.
func (e *SafeguardMultiLangEngine) IndexFileSymbols(filePath, content string) ASTFileIndex {
	lang := DetectLanguage(filePath, "")
	index := ASTFileIndex{
		FilePath: filePath,
		Language: lang,
		Symbols:  make(map[string]ASTSymbol),
		Imports:  make(map[string]bool),
	}

	if content == "" {
		return index
	}

	switch lang {
	case LangGo:
		e.indexGoSymbols(&index, content)
	default:
		e.indexGenericSymbols(&index, content, lang)
	}

	return index
}

func (e *SafeguardMultiLangEngine) indexGoSymbols(index *ASTFileIndex, content string) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, index.FilePath, content, parser.ImportsOnly)
	if err == nil {
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			index.Imports[path] = true
			if imp.Name != nil {
				index.Symbols[imp.Name.Name] = ASTSymbol{Name: imp.Name.Name, Kind: "import", Line: fset.Position(imp.Pos()).Line}
			} else {
				parts := strings.Split(path, "/")
				pkgName := parts[len(parts)-1]
				index.Symbols[pkgName] = ASTSymbol{Name: pkgName, Kind: "import", Line: fset.Position(imp.Pos()).Line}
			}
		}
	}

	// Parse declarations
	declF, err := parser.ParseFile(fset, index.FilePath, content, 0)
	if err == nil {
		for _, d := range declF.Decls {
			// Extract exported and unexported identifiers
			for _, ident := range extractGoDeclSymbols(d) {
				index.Symbols[ident.Name] = ident
			}
		}
	} else {
		// Fallback to regex
		e.indexGenericSymbols(index, content, LangGo)
	}
}

func (e *SafeguardMultiLangEngine) indexGenericSymbols(index *ASTFileIndex, content string, lang SupportedLanguage) {
	lines := strings.Split(content, "\n")

	// Regular expressions for common patterns
	funcRegex := regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:(?:public|private|protected|static|async|function|def|func|fn)\s+)*(?:[a-zA-Z0-9_<>[\]]+\s+)?([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)
	classRegex := regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:class|struct|interface|type|enum|record|trait)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	importRegex := regexp.MustCompile(`(?m)(?:import|from|use|require)\s+['"]?([a-zA-Z0-9_./\-]+)['"]?`)

	for lineIdx, line := range lines {
		lineNum := lineIdx + 1

		// Imports
		if matches := importRegex.FindAllStringSubmatch(line, -1); len(matches) > 0 {
			for _, m := range matches {
				if len(m) > 1 {
					index.Imports[m[1]] = true
				}
			}
		}

		// Classes / Types
		if m := classRegex.FindStringSubmatch(line); len(m) > 1 {
			name := m[1]
			index.Symbols[name] = ASTSymbol{Name: name, Kind: "class", Line: lineNum, IsExported: unicode.IsUpper(rune(name[0]))}
		}

		// Functions
		if m := funcRegex.FindStringSubmatch(line); len(m) > 1 {
			name := m[1]
			if !e.isKeyword(lang, name) {
				index.Symbols[name] = ASTSymbol{Name: name, Kind: "function", Line: lineNum, IsExported: unicode.IsUpper(rune(name[0]))}
			}
		}
	}
}

func (e *SafeguardMultiLangEngine) isKeyword(lang SupportedLanguage, ident string) bool {
	if kw, ok := e.keywordMap[lang]; ok {
		return kw[ident]
	}
	return false
}

// ValidateSuggestionReplacement validates syntax tree integrity and detects hallucinated identifiers.
func (e *SafeguardMultiLangEngine) ValidateSuggestionReplacement(
	fileIndex ASTFileIndex,
	fileContent string,
	existingCode string,
	improvedCode string,
) ReplacementSyntaxReport {
	report := ReplacementSyntaxReport{IsValid: true}

	if strings.TrimSpace(improvedCode) == "" {
		report.IsValid = false
		report.SyntaxErrors = append(report.SyntaxErrors, "improvedCode is empty or blank")
		return report
	}

	// 1. Delimiter balance check (parentheses, braces, brackets, quotes)
	delimErrors := checkDelimiterBalancing(improvedCode)
	if len(delimErrors) > 0 {
		report.IsValid = false
		report.SyntaxErrors = append(report.SyntaxErrors, delimErrors...)
	}

	// 2. Indentation style verification
	indentIssue := checkIndentationConsistency(fileContent, improvedCode)
	if indentIssue != "" {
		report.IndentationIssue = indentIssue
	}

	// 3. Hallucination check for referenced symbols
	hallucinated := e.findHallucinatedSymbols(fileIndex, existingCode, improvedCode)
	if len(hallucinated) > 0 {
		report.HallucinatedSymbols = hallucinated
		// If more than 2 completely unknown symbols are introduced, flag as invalid
		if len(hallucinated) >= 2 {
			report.IsValid = false
		}
	}

	return report
}

func (e *SafeguardMultiLangEngine) findHallucinatedSymbols(
	fileIndex ASTFileIndex,
	existingCode, improvedCode string,
) []string {
	lang := fileIndex.Language

	cleanExisting := stripStringLiterals(existingCode)
	cleanImproved := stripStringLiterals(improvedCode)

	// Extract all words from cleanExisting to know what was already present
	existingIdents := make(map[string]bool)
	for _, word := range e.identRegex.FindAllString(cleanExisting, -1) {
		existingIdents[word] = true
	}

	preludes := e.builtins[lang]
	keywords := e.keywordMap[lang]

	// Extract package member accesses (e.g. fmt.Errorf or console.log)
	pkgCallRegex := regexp.MustCompile(`\b([a-zA-Z_][a-zA-Z0-9_]*)\.([a-zA-Z_][a-zA-Z0-9_]*)\b`)
	pkgMembers := make(map[string]bool)
	for _, m := range pkgCallRegex.FindAllStringSubmatch(cleanImproved, -1) {
		if len(m) > 2 {
			pkg := m[1]
			member := m[2]
			if (preludes != nil && preludes[pkg]) || fileIndex.Imports[pkg] {
				pkgMembers[member] = true
			}
		}
	}

	// Extract all identifiers from cleanImproved
	improvedWords := e.identRegex.FindAllString(cleanImproved, -1)
	var unknown []string
	seen := make(map[string]bool)

	for _, word := range improvedWords {
		if len(word) <= 1 || seen[word] {
			continue
		}
		seen[word] = true

		// Skip if already existed in existingCode
		if existingIdents[word] {
			continue
		}

		// Skip language keywords
		if keywords != nil && keywords[word] {
			continue
		}

		// Skip standard library builtins
		if preludes != nil && preludes[word] {
			continue
		}

		// Skip if member of imported package
		if pkgMembers[word] {
			continue
		}

		// Skip if declared in target file AST symbols or imports
		if _, ok := fileIndex.Symbols[word]; ok {
			continue
		}
		if _, ok := fileIndex.Imports[word]; ok {
			continue
		}

		// Check if it's a common acronym, number, or local identifier (e.g. i, j, err, ctx, res, req)
		if isCommonLocalVar(word) {
			continue
		}

		unknown = append(unknown, word)
	}

	return unknown
}

func isCommonLocalVar(name string) bool {
	switch strings.ToLower(name) {
	case "i", "j", "k", "n", "m", "x", "y", "z", "idx", "val", "key",
		"err", "ok", "ctx", "req", "res", "resp", "buf", "b", "w", "r",
		"tmp", "ret", "item", "el", "elem", "node", "cur", "prev", "next",
		"data", "msg", "out", "in", "cfg", "opt", "opts", "args", "params":
		return true
	default:
		return false
	}
}

func checkDelimiterBalancing(code string) []string {
	var errors []string
	var stack []rune
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	escaped := false

	chars := []rune(code)
	for i := 0; i < len(chars); i++ {
		ch := chars[i]

		if escaped {
			escaped = false
			continue
		}

		if ch == '\\' {
			escaped = true
			continue
		}

		if ch == '\'' && !inDoubleQuote && !inBacktick {
			inSingleQuote = !inSingleQuote
			continue
		}
		if ch == '"' && !inSingleQuote && !inBacktick {
			inDoubleQuote = !inDoubleQuote
			continue
		}
		if ch == '`' && !inSingleQuote && !inDoubleQuote {
			inBacktick = !inBacktick
			continue
		}

		if inSingleQuote || inDoubleQuote || inBacktick {
			continue
		}

		switch ch {
		case '(', '{', '[':
			stack = append(stack, ch)
		case ')':
			if len(stack) == 0 || stack[len(stack)-1] != '(' {
				errors = append(errors, "unmatched closing parenthesis ')'")
			} else {
				stack = stack[:len(stack)-1]
			}
		case '}':
			if len(stack) == 0 || stack[len(stack)-1] != '{' {
				errors = append(errors, "unmatched closing brace '}'")
			} else {
				stack = stack[:len(stack)-1]
			}
		case ']':
			if len(stack) == 0 || stack[len(stack)-1] != '[' {
				errors = append(errors, "unmatched closing bracket ']'")
			} else {
				stack = stack[:len(stack)-1]
			}
		}
	}

	if inSingleQuote {
		errors = append(errors, "unclosed single quote")
	}
	if inDoubleQuote {
		errors = append(errors, "unclosed double quote")
	}
	if inBacktick {
		errors = append(errors, "unclosed backtick quote")
	}
	if len(stack) > 0 {
		errors = append(errors, fmt.Sprintf("unclosed delimiters: %c", stack[len(stack)-1]))
	}

	return errors
}

func checkIndentationConsistency(fileContent, improvedCode string) string {
	fileHasTabs := strings.Contains(fileContent, "\t")
	codeHasTabs := strings.Contains(improvedCode, "\t")
	codeHasSpaces := strings.Contains(improvedCode, "  ")

	if fileHasTabs && !codeHasTabs && codeHasSpaces {
		return "file uses tabs for indentation, but improved code uses spaces"
	}
	if !fileHasTabs && codeHasTabs {
		return "file uses spaces for indentation, but improved code contains tabs"
	}
	return ""
}

func extractGoDeclSymbols(d ast.Decl) []ASTSymbol {
	var symbols []ASTSymbol

	switch decl := d.(type) {
	case *ast.FuncDecl:
		if decl.Name != nil {
			name := decl.Name.Name
			symbols = append(symbols, ASTSymbol{
				Name:       name,
				Kind:       "function",
				IsExported: ast.IsExported(name),
			})
		}
	case *ast.GenDecl:
		for _, spec := range decl.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				if s.Name != nil {
					name := s.Name.Name
					symbols = append(symbols, ASTSymbol{
						Name:       name,
						Kind:       "type",
						IsExported: ast.IsExported(name),
					})
				}
			case *ast.ValueSpec:
				for _, name := range s.Names {
					if name != nil {
						symbols = append(symbols, ASTSymbol{
							Name:       name.Name,
							Kind:       "variable",
							IsExported: ast.IsExported(name.Name),
						})
					}
				}
			}
		}
	}

	return symbols
}

func stripStringLiterals(code string) string {
	re := regexp.MustCompile(`(?s)"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|` + "`" + `[^` + "`" + `]*` + "`")
	return re.ReplaceAllString(code, " ")
}


