package taskcontext

import (
	"regexp"
	"sort"
	"strings"
)

var (
	camelRegex      = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	nonAlphaTokenRe = regexp.MustCompile(`[^a-z0-9]+`)
)

// BuildToolAliasKey creates a normalized, sorted alias key for tool matching.
func BuildToolAliasKey(value string) string {
	normalized := camelRegex.ReplaceAllString(value, "${1} ${2}")
	normalized = strings.ToLower(normalized)

	rawTokens := nonAlphaTokenRe.Split(normalized, -1)
	var tokens []string
	for _, raw := range rawTokens {
		token := normalizeToolAliasToken(raw)
		if len(token) > 0 && !isAliasNoiseToken(token) {
			tokens = append(tokens, token)
		}
	}

	sort.Strings(tokens)
	return strings.Join(tokens, " ")
}

func normalizeToolAliasToken(token string) string {
	if len(token) > 3 && strings.HasSuffix(token, "ies") {
		return token[:len(token)-3] + "y"
	}
	if len(token) > 3 && strings.HasSuffix(token, "s") {
		return token[:len(token)-1]
	}
	return token
}

func isAliasNoiseToken(token string) bool {
	switch token {
	case "provider", "workspace", "workspaces", "plugin", "integration":
		return true
	default:
		return false
	}
}
