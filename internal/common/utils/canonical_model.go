package utils

import (
	"regexp"
	"strings"
)

var bedrockVersionRegex = regexp.MustCompile(`:\d+$`)

// CanonicalModelID normalizes model identifiers so spending, pricing, and BYOK cost joins roll up into a stable bucket.
// It strips trailing :<digits> version suffixes (Amazon Bedrock) and leading provider: prefixes,
// while preserving provider/model slash prefixes (Vertex AI).
func CanonicalModelID(model string) string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return ""
	}
	stripped := bedrockVersionRegex.ReplaceAllString(trimmed, "")
	parts := strings.Split(stripped, ":")
	return strings.TrimSpace(parts[len(parts)-1])
}
