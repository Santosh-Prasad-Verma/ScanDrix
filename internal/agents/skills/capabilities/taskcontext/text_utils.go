package taskcontext

import (
	"encoding/json"
	"regexp"
	"strings"
)

var nonAlphaNumRegex = regexp.MustCompile(`[^a-z0-9]+`)

// TryParseJSONString attempts to parse JSON object or array string.
func TryParseJSONString(value string) (any, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, false
	}
	if !((strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"))) {
		return nil, false
	}

	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return nil, false
	}
	return parsed, true
}

// FirstNonEmptyString returns the first non-empty string in the list.
func FirstNonEmptyString(values ...any) string {
	for _, val := range values {
		if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// FirstNonEmptyValue returns the first non-nil, non-empty value.
func FirstNonEmptyValue(values ...any) any {
	for _, val := range values {
		if val == nil {
			continue
		}
		if s, ok := val.(string); ok {
			if strings.TrimSpace(s) != "" {
				return s
			}
			continue
		}
		return val
	}
	return nil
}

// NormalizeParamName strips to lowercase alphanumerics for loose matching.
func NormalizeParamName(value string) string {
	return nonAlphaNumRegex.ReplaceAllString(strings.ToLower(value), "")
}

// UniqueNonEmpty returns deduplicated non-empty strings.
func UniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	var result []string
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; !exists {
			seen[trimmed] = struct{}{}
			result = append(result, trimmed)
		}
	}
	return result
}
