package languages

import (
	"path/filepath"
	"strings"
)

// Language represents a supported programming language or file format.
type Language string

const (
	LangGo         Language = "go"
	LangTypeScript Language = "typescript"
	LangJavaScript Language = "javascript"
	LangPython     Language = "python"
	LangJava       Language = "java"
	LangKotlin     Language = "kotlin"
	LangRust       Language = "rust"
	LangCpp        Language = "cpp"
	LangC          Language = "c"
	LangCSharp     Language = "csharp"
	LangRuby       Language = "ruby"
	LangPHP        Language = "php"
	LangSwift      Language = "swift"
	LangScala      Language = "scala"
	LangSQL        Language = "sql"
	LangDockerfile Language = "dockerfile"
	LangTerraform  Language = "terraform"
	LangYAML       Language = "yaml"
	LangJSON       Language = "json"
	LangMarkdown   Language = "markdown"
	LangUnknown    Language = "unknown"
)

var extensionMap = map[string]Language{
	".go":     LangGo,
	".ts":     LangTypeScript,
	".tsx":    LangTypeScript,
	".js":     LangJavaScript,
	".jsx":    LangJavaScript,
	".mjs":    LangJavaScript,
	".cjs":    LangJavaScript,
	".py":     LangPython,
	".pyw":    LangPython,
	".java":   LangJava,
	".kt":     LangKotlin,
	".kts":    LangKotlin,
	".rs":     LangRust,
	".cpp":    LangCpp,
	".cc":     LangCpp,
	".cxx":    LangCpp,
	".hpp":    LangCpp,
	".h":      LangC,
	".c":      LangC,
	".cs":     LangCSharp,
	".rb":     LangRuby,
	".php":    LangPHP,
	".swift":  LangSwift,
	".scala":  LangScala,
	".sql":    LangSQL,
	".tf":     LangTerraform,
	".tfvars": LangTerraform,
	".yaml":   LangYAML,
	".yml":    LangYAML,
	".json":   LangJSON,
	".md":     LangMarkdown,
}

// DetectLanguage identifies the programming language from file path and content.
func DetectLanguage(filePath string, sampleContent string) Language {
	base := filepath.Base(filePath)
	lowerBase := strings.ToLower(base)

	if lowerBase == "dockerfile" || strings.HasPrefix(lowerBase, "dockerfile.") {
		return LangDockerfile
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	if lang, found := extensionMap[ext]; found {
		return lang
	}

	// Heuristic shebang detection
	if strings.HasPrefix(sampleContent, "#!") {
		firstLine := strings.Split(sampleContent, "\n")[0]
		switch {
		case strings.Contains(firstLine, "python"):
			return LangPython
		case strings.Contains(firstLine, "node") || strings.Contains(firstLine, "bun") || strings.Contains(firstLine, "deno"):
			return LangJavaScript
		case strings.Contains(firstLine, "ruby"):
			return LangRuby
		case strings.Contains(firstLine, "php"):
			return LangPHP
		}
	}

	return LangUnknown
}

// IsCodeFile checks if the detected language represents runnable code or infrastructure as code.
func (l Language) IsCodeFile() bool {
	switch l {
	case LangGo, LangTypeScript, LangJavaScript, LangPython, LangJava, LangKotlin,
		LangRust, LangCpp, LangC, LangCSharp, LangRuby, LangPHP, LangSwift,
		LangScala, LangSQL, LangDockerfile, LangTerraform:
		return true
	default:
		return false
	}
}
