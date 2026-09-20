package log

import (
	"regexp"
	"strings"
)

// TokenUsageArea defines the low-cardinality process dimension where tokens were spent.
type TokenUsageArea string

const (
	AreaReview       TokenUsageArea = "review"
	AreaDrixyRules   TokenUsageArea = "drixy_rules"
	AreaSuggestions  TokenUsageArea = "suggestions"
	AreaSummary      TokenUsageArea = "summary"
	AreaConversation TokenUsageArea = "conversation"
	AreaSystem       TokenUsageArea = "system"
	AreaOther        TokenUsageArea = "other"
)

// SuggestionRunNames mirrors SUGGESTION_RUN_NAMES from ScanDrix.
var SuggestionRunNames = map[string]bool{
	"severity-classifier":                    true,
	"suggestion-formatter":                   true,
	"severityAnalysis":                       true,
	"validateWithLLM":                        true,
	"checkSuggestionSimplicity":              true,
	"repeatedCodeReviewSuggestionClustering": true,
}

// SystemRunNames mirrors SYSTEM_RUN_NAMES from ScanDrix.
var SystemRunNames = map[string]bool{
	"selectReviewMode":               true,
	"validateImplementedSuggestions": true,
	"generateCodeSuggestions":        true,
}

// RoutingTasks mirrors ROUTING_TASKS from ScanDrix.
var RoutingTasks = map[string]bool{
	"codeReview":         true,
	"drixyRulesReview":   true,
	"ruleGeneration":     true,
	"businessValidation": true,
	"prSummary":          true,
	"conversation":       true,
}

var drixyRulesPattern = regexp.MustCompile(`(?i)(?:drixy|scandrix).?rules?`)

// TokenUsageTu mirrors ScanDrix TokenUsageTu contract for indexable nested sub-documents.
type TokenUsageTu struct {
	IsByok       bool           `json:"isByok"`
	Sys          bool           `json:"sys"`
	Model        string         `json:"model"`
	CredentialID string         `json:"credentialId"`
	Input        int64          `json:"input"`
	Output       int64          `json:"output"`
	Total        int64          `json:"total"`
	Reasoning    int64          `json:"reasoning"`
	CacheRead    int64          `json:"cacheRead"`
	CacheWrite   int64          `json:"cacheWrite"`
	Area         TokenUsageArea `json:"area"`
	Route        string         `json:"route"`
}

// DeriveArea maps a span's run/agent identifiers onto the fixed TokenUsageArea set.
func DeriveArea(runName string, phase string) TokenUsageArea {
	if SystemRunNames[runName] {
		return AreaSystem
	}
	if drixyRulesPattern.MatchString(runName) || strings.HasPrefix(runName, "drixyMemory") {
		return AreaDrixyRules
	}
	if strings.HasPrefix(runName, "code-review") || strings.HasPrefix(runName, "analyzeCodeWithAI") {
		return AreaReview
	}
	if SuggestionRunNames[runName] || strings.HasPrefix(runName, "safeguard") {
		return AreaSuggestions
	}
	if strings.HasPrefix(runName, "generateSummaryPR") {
		return AreaSummary
	}
	if runName == "conversationAgent" || phase == "conversation" {
		return AreaConversation
	}
	return AreaOther
}

// RouteFromArea infers the routing task from the process TokenUsageArea.
func RouteFromArea(area TokenUsageArea) string {
	switch area {
	case AreaReview, AreaSuggestions:
		return "codeReview"
	case AreaDrixyRules:
		return "drixyRulesReview"
	case AreaSummary:
		return "prSummary"
	case AreaConversation:
		return "conversation"
	default:
		return ""
	}
}

// DeriveTu derives indexable TU metrics from span flat dotted-key attributes.
func DeriveTu(attrs map[string]any) *TokenUsageTu {
	if attrs == nil {
		return nil
	}

	total := getInt64(attrs, "gen_ai.usage.total_tokens")
	if total <= 0 {
		return nil
	}

	rawModel, _ := attrs["gen_ai.response.model"].(string)
	model := CanonicalModelID(rawModel)

	runName, _ := attrs["gen_ai.run.name"].(string)
	phase, _ := attrs["agent.phase"].(string)
	area := DeriveArea(runName, phase)

	credentialID, _ := attrs["credentialId"].(string)
	routeAttr, _ := attrs["route"].(string)

	route := routeAttr
	if !RoutingTasks[route] {
		route = RouteFromArea(area)
	}

	typeAttr, _ := attrs["type"].(string)

	return &TokenUsageTu{
		IsByok:       typeAttr == "byok",
		Sys:          SystemRunNames[runName],
		Model:        model,
		CredentialID: credentialID,
		Input:        getInt64(attrs, "gen_ai.usage.input_tokens"),
		Output:       getInt64(attrs, "gen_ai.usage.output_tokens"),
		Total:        total,
		Reasoning:    getInt64(attrs, "gen_ai.usage.reasoning_tokens"),
		CacheRead:    getInt64(attrs, "gen_ai.usage.cache_read_input_tokens"),
		CacheWrite:   getInt64(attrs, "gen_ai.usage.cache_creation_input_tokens"),
		Area:         area,
		Route:        route,
	}
}

// CanonicalModelID strips provider prefixes (e.g. "anthropic:claude-3-5-sonnet" -> "claude-3-5-sonnet").
func CanonicalModelID(raw string) string {
	if raw == "" {
		return ""
	}
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) == 2 && strings.Contains(parts[0], "google") || strings.Contains(parts[0], "anthropic") || strings.Contains(parts[0], "openai") {
		return parts[1]
	}
	return raw
}

func getInt64(m map[string]any, key string) int64 {
	val, ok := m[key]
	if !ok {
		return 0
	}
	switch v := val.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}
