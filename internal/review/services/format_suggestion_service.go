package services

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/scandrix/backend/internal/review/domain"
)

// LanguageLocale identifies the target localization language.
type LanguageLocale string

const (
	LocaleEN LanguageLocale = "en"
	LocaleES LanguageLocale = "es"
	LocaleJA LanguageLocale = "ja"
	LocaleDE LanguageLocale = "de"
	LocaleFR LanguageLocale = "fr"
	LocaleZH LanguageLocale = "zh"
	LocalePT LanguageLocale = "pt"
)

// SuggestionFormattingOptions configures output rendering parameters.
type SuggestionFormattingOptions struct {
	Platform                SCMPlatformType
	Locale                  LanguageLocale
	IncludeBadges           bool
	IncludeSecurityTags     bool
	IncludeSLSAPosture      bool
	IncludeActionStatement  bool
	CustomWritingGuidelines string
	DryRun                  bool
}

// DefaultFormattingOptions provides default settings for GitHub with security badges.
func DefaultFormattingOptions(platform SCMPlatformType) SuggestionFormattingOptions {
	return SuggestionFormattingOptions{
		Platform:            platform,
		Locale:              LocaleEN,
		IncludeBadges:       true,
		IncludeSecurityTags: true,
		IncludeSLSAPosture:  true,
		IncludeActionStatement: true,
		DryRun:              false,
	}
}

// FormatSuggestionContentService handles presentation transformation for review comments.
type FormatSuggestionContentService struct {
	whatWhyHowRegex []*regexp.Regexp
	jsonBlockRegex  *regexp.Regexp
	translations    map[LanguageLocale]map[string]string
}

// NewFormatSuggestionContentService creates and initializes a formatting service.
func NewFormatSuggestionContentService() *FormatSuggestionContentService {
	s := &FormatSuggestionContentService{
		whatWhyHowRegex: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^\s*(?:WHAT|WHY|HOW|PROBLEM|FIX|IMPACT)\s*:\s*`),
			regexp.MustCompile(`(?i)^\s*[0-9]+[\.\)]\s*`),
		},
		jsonBlockRegex: regexp.MustCompile(`(?s)\[\s*\{.*\}\s*\]`),
	}
	s.initTranslations()
	return s
}

func (s *FormatSuggestionContentService) initTranslations() {
	s.translations = map[LanguageLocale]map[string]string{
		LocaleEN: {
			"security":           "Security",
			"performance":        "Performance",
			"bug":                "Defect",
			"architecture":       "Architecture",
			"drixy_rule":         "Drixy Rule",
			"critical":           "Critical",
			"high":               "High",
			"medium":             "Medium",
			"low":                "Low",
			"action":             "Suggested Action",
			"dry_run_notice":     "ScanDrix Dry Run Preview (No remote changes applied)",
			"slsa_verified":      "SLSA Level 3 Provenance Verified",
			"cwe":                "CWE",
			"owasp":              "OWASP",
			"blast_radius":       "Callers Blast Radius",
			"complexity_delta":   "Cognitive Complexity",
		},
		LocaleES: {
			"security":           "Seguridad",
			"performance":        "Rendimiento",
			"bug":                "Defecto",
			"architecture":       "Arquitectura",
			"drixy_rule":         "Regla Drixy",
			"critical":           "Crítico",
			"high":               "Alto",
			"medium":             "Medio",
			"low":                "Bajo",
			"action":             "Acción Sugerida",
			"dry_run_notice":     "Vista previa de ejecución en seco de ScanDrix (sin cambios remotos)",
			"slsa_verified":      "Procedencia SLSA Nivel 3 Verificada",
			"cwe":                "CWE",
			"owasp":              "OWASP",
			"blast_radius":       "Radio de Impacto",
			"complexity_delta":   "Complejidad Cognitiva",
		},
		LocaleJA: {
			"security":           "セキュリティ",
			"performance":        "パフォーマンス",
			"bug":                "バグ",
			"architecture":       "アーキテクチャ",
			"drixy_rule":         "Drixy ルール",
			"critical":           "緊急 (Critical)",
			"high":               "高 (High)",
			"medium":             "中 (Medium)",
			"low":                "低 (Low)",
			"action":             "推奨される対応",
			"dry_run_notice":     "ScanDrix ドライラン プレビュー (リモート変更は適用されません)",
			"slsa_verified":      "SLSA レベル 3 真正性検証済み",
			"cwe":                "CWE",
			"owasp":              "OWASP",
			"blast_radius":       "影響範囲",
			"complexity_delta":   "認知的複雑度",
		},
		LocalePT: {
			"security":           "Segurança",
			"performance":        "Desempenho",
			"bug":                "Defeito",
			"architecture":       "Arquitetura",
			"drixy_rule":         "Regra Drixy",
			"critical":           "Crítico",
			"high":               "Alto",
			"medium":             "Médio",
			"low":                "Baixo",
			"action":             "Ação Recomendada",
			"dry_run_notice":     "Pré-visualização Dry Run ScanDrix (nenhuma alteração remota)",
			"slsa_verified":      "Proveniência SLSA Nível 3 Verificada",
			"cwe":                "CWE",
			"owasp":              "OWASP",
			"blast_radius":       "Raio de Impacto",
			"complexity_delta":   "Complexidade Cognitiva",
		},
		LocaleDE: {
			"security":           "Sicherheit",
			"performance":        "Leistung",
			"bug":                "Fehler",
			"architecture":       "Architektur",
			"drixy_rule":         "Drixy-Regel",
			"critical":           "Kritisch",
			"high":               "Hoch",
			"medium":             "Mittel",
			"low":                "Niedrig",
			"action":             "Vorgeschlagene Maßnahme",
			"dry_run_notice":     "ScanDrix Testlauf-Vorschau (keine Remote-Änderungen)",
			"slsa_verified":      "SLSA Level 3 Verifiziert",
			"cwe":                "CWE",
			"owasp":              "OWASP",
			"blast_radius":       "Auswirkungsradius",
			"complexity_delta":   "Kognitive Komplexität",
		},
		LocaleFR: {
			"security":           "Sécurité",
			"performance":        "Performance",
			"bug":                "Anomalie",
			"architecture":       "Architecture",
			"drixy_rule":         "Règle Drixy",
			"critical":           "Critique",
			"high":               "Élevé",
			"medium":             "Moyen",
			"low":                "Faible",
			"action":             "Action Suggérée",
			"dry_run_notice":     "Aperçu Dry Run ScanDrix (aucune modification distante)",
			"slsa_verified":      "Provenance SLSA Niveau 3 Vérifiée",
			"cwe":                "CWE",
			"owasp":              "OWASP",
			"blast_radius":       "Rayon d'Impact",
			"complexity_delta":   "Complexité Cognitive",
		},
		LocaleZH: {
			"security":           "安全",
			"performance":        "性能",
			"bug":                "缺陷",
			"architecture":       "架构",
			"drixy_rule":         "Drixy 规范",
			"critical":           "严重",
			"high":               "高",
			"medium":             "中",
			"low":                "低",
			"action":             "建议操作",
			"dry_run_notice":     "ScanDrix 试运行预览 (未应用任何远程更改)",
			"slsa_verified":      "SLSA 级别 3 来源已验证",
			"cwe":                "CWE",
			"owasp":              "OWASP",
			"blast_radius":       "调用爆炸半径",
			"complexity_delta":   "认知复杂度",
		},
	}
}

func (s *FormatSuggestionContentService) tr(locale LanguageLocale, key string) string {
	if m, ok := s.translations[locale]; ok {
		if val, exists := m[key]; exists {
			return val
		}
	}
	if m, ok := s.translations[LocaleEN]; ok {
		if val, exists := m[key]; exists {
			return val
		}
	}
	return key
}

// CalculateCommentStartLine computes the starting line for inline comments.
// If the range exceeds 15 lines, returns 0 (omitted on GitHub) to prevent UI clutter and API errors.
func CalculateCommentStartLine(startLine, endLine int) int {
	if startLine <= 0 || startLine >= endLine {
		return 0
	}
	if startLine+15 > endLine {
		return startLine
	}
	return 0
}

// CalculateCommentEndLine computes the target end line for inline comments.
// If the span exceeds 15 lines, clamps to the start line to anchor safely.
func CalculateCommentEndLine(startLine, endLine int) int {
	if startLine <= 0 || startLine >= endLine {
		if endLine > 0 {
			return endLine
		}
		return startLine
	}
	if startLine+15 > endLine {
		return endLine
	}
	return startLine
}

// CleanWhatWhyHowLabels removes structured labels and normalizes content into natural prose.
func (s *FormatSuggestionContentService) CleanWhatWhyHowLabels(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}

	lines := strings.Split(text, "\n")
	var cleanedLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Strip WHAT:, WHY:, HOW:, 1., 2., etc.
		for _, re := range s.whatWhyHowRegex {
			trimmed = re.ReplaceAllString(trimmed, "")
		}

		cleaned := strings.TrimSpace(trimmed)
		if cleaned != "" {
			// Ensure first letter capitalized
			if len(cleaned) > 0 {
				r := []rune(cleaned)
				cleaned = strings.ToUpper(string(r[0])) + string(r[1:])
			}
			cleanedLines = append(cleanedLines, cleaned)
		}
	}

	if len(cleanedLines) == 0 {
		return text
	}

	// Merge into natural sentences
	merged := strings.Join(cleanedLines, " ")
	// Deduplicate accidental double spaces
	merged = strings.Join(strings.Fields(merged), " ")
	return merged
}

// BuildCommentBody formats a domain CodeSuggestion into full markdown for the specified platform.
func (s *FormatSuggestionContentService) BuildCommentBody(
	sug *domain.CodeSuggestion,
	opts SuggestionFormattingOptions,
) string {
	if sug == nil {
		return ""
	}

	var sb strings.Builder

	// Dry-run banner
	if opts.DryRun {
		sb.WriteString(fmt.Sprintf("> ℹ️ **%s**\n\n", s.tr(opts.Locale, "dry_run_notice")))
	}

	// Top badges row
	if opts.IncludeBadges {
		sb.WriteString(s.renderBadgesRow(sug, opts.Locale))
		sb.WriteString("\n\n")
	}

	// Explanation text
	cleanedExplanation := s.CleanWhatWhyHowLabels(sug.GetExplanation())
	sb.WriteString(cleanedExplanation)
	sb.WriteString("\n\n")

	// Action statement if present
	if opts.IncludeActionStatement && sug.Clustering != nil && sug.Clustering.ActionStatement != "" {
		sb.WriteString(fmt.Sprintf("**%s:** %s\n\n", s.tr(opts.Locale, "action"), sug.Clustering.ActionStatement))
	}

	// Suggested code replacement block
	replacement := sug.GetSuggestedReplacement()
	if strings.TrimSpace(replacement) != "" {
		sb.WriteString(s.renderCodeReplacement(replacement, sug.Language, opts.Platform))
		sb.WriteString("\n\n")
	}

	// Security & Standards footer
	if opts.IncludeSecurityTags {
		secFooter := s.renderSecurityAndComplexityFooter(sug, opts.Locale)
		if secFooter != "" {
			sb.WriteString(secFooter)
			sb.WriteString("\n\n")
		}
	}

	// SLSA Level 3 badge
	if opts.IncludeSLSAPosture {
		sb.WriteString(fmt.Sprintf("<div align=\"right\"><sub>🛡️ %s &bull; <a href=\"https://scandrix.dev\">ScanDrix</a></sub></div>", s.tr(opts.Locale, "slsa_verified")))
	}

	return strings.TrimSpace(sb.String())
}

func (s *FormatSuggestionContentService) renderBadgesRow(sug *domain.CodeSuggestion, locale LanguageLocale) string {
	var badges []string

	// Severity badge
	sev := strings.ToLower(string(sug.Severity))
	switch sev {
	case "critical":
		badges = append(badges, fmt.Sprintf("🔴 `%s`", s.tr(locale, "critical")))
	case "high", "major":
		badges = append(badges, fmt.Sprintf("🟠 `%s`", s.tr(locale, "high")))
	case "medium", "moderate":
		badges = append(badges, fmt.Sprintf("🟡 `%s`", s.tr(locale, "medium")))
	default:
		badges = append(badges, fmt.Sprintf("🔵 `%s`", s.tr(locale, "low")))
	}

	// Category badge
	cat := strings.ToLower(string(sug.Category))
	switch cat {
	case "security":
		badges = append(badges, fmt.Sprintf("🔒 `%s`", s.tr(locale, "security")))
	case "performance":
		badges = append(badges, fmt.Sprintf("⚡ `%s`", s.tr(locale, "performance")))
	case "architecture":
		badges = append(badges, fmt.Sprintf("📐 `%s`", s.tr(locale, "architecture")))
	case "drixy_rules":
		badges = append(badges, fmt.Sprintf("📏 `%s`", s.tr(locale, "drixy_rule")))
	default:
		badges = append(badges, fmt.Sprintf("🐛 `%s`", s.tr(locale, "bug")))
	}

	// Rule ID badge if present
	if len(sug.BrokenRuleIDs) > 0 {
		badges = append(badges, fmt.Sprintf("🏷️ `%s`", sug.BrokenRuleIDs[0]))
	} else if sug.RuleID != "" {
		badges = append(badges, fmt.Sprintf("🏷️ `%s`", sug.RuleID))
	}

	return strings.Join(badges, " &nbsp;|&nbsp; ")
}

func (s *FormatSuggestionContentService) renderCodeReplacement(
	replacement, lang string,
	platform SCMPlatformType,
) string {
	cleanCode := strings.TrimRight(replacement, "\r\n")

	switch platform {
	case PlatformGitHub:
		// GitHub native suggestion block
		return fmt.Sprintf("```suggestion\n%s\n```", cleanCode)
	case PlatformGitLab:
		// GitLab suggestion format
		return fmt.Sprintf("```suggestion:-0+0\n%s\n```", cleanCode)
	case PlatformBitbucket:
		// Bitbucket does not support ```suggestion, use ```diff or language block
		return fmt.Sprintf("```diff\n+ %s\n```", strings.ReplaceAll(cleanCode, "\n", "\n+ "))
	default:
		// Azure DevOps and generic
		codeLang := lang
		if codeLang == "" {
			codeLang = "diff"
		}
		return fmt.Sprintf("```%s\n%s\n```", codeLang, cleanCode)
	}
}

func (s *FormatSuggestionContentService) renderSecurityAndComplexityFooter(sug *domain.CodeSuggestion, locale LanguageLocale) string {
	var items []string

	// Check if this is a security issue to attach CWE / OWASP
	isSec := strings.EqualFold(string(sug.Category), "security") ||
		strings.Contains(strings.ToLower(sug.GetExplanation()), "injection") ||
		strings.Contains(strings.ToLower(sug.GetExplanation()), "xss") ||
		strings.Contains(strings.ToLower(sug.GetExplanation()), "leak")

	if isSec {
		cwe, owasp := extractCWEAndOWASP(sug.GetExplanation())
		if cwe != "" {
			items = append(items, fmt.Sprintf("**%s:** `%s`", s.tr(locale, "cwe"), cwe))
		}
		if owasp != "" {
			items = append(items, fmt.Sprintf("**%s:** `%s`", s.tr(locale, "owasp"), owasp))
		}
	}

	if sug.Confidence > 0 {
		items = append(items, fmt.Sprintf("**Confidence:** `%.0f%%`", sug.Confidence*100))
	}

	if len(items) == 0 {
		return ""
	}

	return "<details><summary><b>Security & Quality Context</b></summary>\n\n" +
		strings.Join(items, " &bull; ") +
		"\n</details>"
}

func extractCWEAndOWASP(text string) (string, string) {
	lower := strings.ToLower(text)
	cwe := ""
	owasp := ""

	if strings.Contains(lower, "sql injection") {
		cwe = "CWE-89"
		owasp = "A03:2021-Injection"
	} else if strings.Contains(lower, "command injection") || strings.Contains(lower, "exec") {
		cwe = "CWE-78"
		owasp = "A03:2021-Injection"
	} else if strings.Contains(lower, "cross-site scripting") || strings.Contains(lower, "xss") {
		cwe = "CWE-79"
		owasp = "A03:2021-Injection"
	} else if strings.Contains(lower, "leak") || strings.Contains(lower, "resource") {
		cwe = "CWE-775"
		owasp = "A04:2021-Insecure Design"
	} else if strings.Contains(lower, "path traversal") || strings.Contains(lower, "../") {
		cwe = "CWE-22"
		owasp = "A01:2021-Broken Access Control"
	} else if strings.Contains(lower, "deserialization") {
		cwe = "CWE-502"
		owasp = "A08:2021-Software and Data Integrity Failures"
	}

	return cwe, owasp
}

// CalculateCVSSScore computes an approximate CVSS v3.1 base score from severity and exploitability signals.
func CalculateCVSSScore(severity string, hasDataExposure, hasUnsafeDataFlow bool) float64 {
	base := 3.0
	switch strings.ToLower(severity) {
	case "critical":
		base = 9.0
	case "high":
		base = 7.5
	case "medium":
		base = 5.0
	case "low":
		base = 2.5
	}

	if hasDataExposure {
		base += 0.8
	}
	if hasUnsafeDataFlow {
		base += 0.5
	}

	return math.Min(10.0, math.Round(base*10)/10)
}

// BuildFormatPrompt constructs the batch rewrite prompt for suggestion formatting.
func (s *FormatSuggestionContentService) BuildFormatPrompt(
	suggestions []*domain.CodeSuggestion,
	customWritingGuidelines string,
	languageLabel string,
) string {
	var suggestionsText strings.Builder

	for i, sug := range suggestions {
		if i > 0 {
			suggestionsText.WriteString("\n\n---\n\n")
		}
		suggestionsText.WriteString(fmt.Sprintf(
			"[%d]\nFile: %s\nLanguage: %s\nContent: %s\nExisting code:\n```\n%s\n```\nImproved code:\n```\n%s\n```",
			i,
			sug.GetFilePath(),
			sug.Language,
			sug.GetExplanation(),
			sug.GetOriginalDiff(),
			sug.GetSuggestedReplacement(),
		))
	}

	guidelineBlock := ""
	if customWritingGuidelines != "" {
		guidelineBlock = fmt.Sprintf("\n\nAdditional writing guidelines from the team:\n%s", customWritingGuidelines)
	}

	langInstruction := ""
	if languageLabel != "" && !strings.EqualFold(languageLabel, "en") && !strings.EqualFold(languageLabel, "english") {
		langInstruction = fmt.Sprintf("\nIMPORTANT: Write all output in %s. Do not fall back to English.", languageLabel)
	}

	return fmt.Sprintf(`You are a code review comment editor. Rewrite each suggestion into clean, natural prose.

Rules:
- Remove labels like "WHAT:", "WHY:", "HOW:", "1.", "2.", "3." from the beginning of sentences.
- Merge the labeled sentences into a single natural paragraph (1-3 SHORT sentences). Aim for 2 sentences max: one describing the problem, one describing the fix.
- Keep every technical detail: function names, file names, variable names, error types, line numbers.
- Be concise: the code block already shows the fix, so the text should explain WHY, not repeat WHAT the code does.
- Do NOT touch existingCode or improvedCode — return them exactly as provided.%s%s

Example:
Input: "WHAT: The join method breaks out of the loop when the timeout expires. WHY: This leaves subsequent flusher processes running indefinitely as orphans. HOW: Remove the remaining_time check."
Output: "The join method breaks out of the loop when the timeout expires, leaving subsequent flusher processes running indefinitely as orphans. Remove the remaining_time check."

Respond with ONLY a JSON array:
`+"```json"+`
[
  {"index": 0, "suggestionContent": "cleaned text"}
]
`+"```"+`

Suggestions to clean:

%s`, guidelineBlock, langInstruction, suggestionsText.String())
}

// ParseFormatResponse parses the model output JSON array into a map of index -> formatted content.
func (s *FormatSuggestionContentService) ParseFormatResponse(text string) (map[int]string, bool) {
	result := make(map[int]string)
	if text == "" {
		return result, false
	}

	matches := s.jsonBlockRegex.FindString(text)
	if matches == "" {
		// Try fallback to any bracketed JSON
		start := strings.Index(text, "[")
		end := strings.LastIndex(text, "]")
		if start != -1 && end > start {
			matches = text[start : end+1]
		}
	}

	if matches == "" {
		return result, false
	}

	var parsed []struct {
		Index             int    `json:"index"`
		SuggestionContent string `json:"suggestionContent"`
	}

	if err := json.Unmarshal([]byte(matches), &parsed); err != nil {
		return result, false
	}

	for _, p := range parsed {
		if p.SuggestionContent != "" {
			result[p.Index] = p.SuggestionContent
		}
	}

	return result, len(result) > 0
}
