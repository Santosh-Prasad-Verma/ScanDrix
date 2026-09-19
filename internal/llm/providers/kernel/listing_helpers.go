// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package kernel

import (
	"encoding/json"
	"regexp"
	"strings"
)

// BearerHeaders creates standard Authorization Bearer and Content-Type headers.
func BearerHeaders(apiKey string) map[string]string {
	h := map[string]string{
		"Content-Type": "application/json",
	}
	if apiKey != "" {
		h["Authorization"] = "Bearer " + apiKey
	}
	return h
}

// FormatModelLabel formats a model ID into a user-friendly label.
func FormatModelLabel(id string) string {
	parts := strings.Split(id, "/")
	raw := parts[len(parts)-1]
	raw = strings.ReplaceAll(raw, "-", " ")
	raw = strings.ReplaceAll(raw, "_", " ")
	words := strings.Fields(raw)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// ParseOpenAIIDs parses an OpenAI-style `{ "data": [{ "id": "..." }] }` JSON response.
func ParseOpenAIIDs(body []byte) ([]CatalogModel, error) {
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	result := make([]CatalogModel, 0, len(resp.Data))
	for _, m := range resp.Data {
		if m.ID != "" {
			result = append(result, CatalogModel{
				ID:   m.ID,
				Name: FormatModelLabel(m.ID),
			})
		}
	}
	return result, nil
}

var vVersionRegex = regexp.MustCompile(`(?i)/v\d+$`)

// OpenAICompatibleModelsURL builds a `/models` endpoint URL from a baseURL.
func OpenAICompatibleModelsURL(baseURL string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	if trimmed == "" {
		return "https://api.openai.com/v1/models"
	}
	if !vVersionRegex.MatchString(trimmed) {
		return trimmed + "/v1/models"
	}
	return trimmed + "/models"
}

// CatalogWithReasoning constructs a CatalogModel with reasoning annotations.
func CatalogWithReasoning(id, name, capKey string) CatalogModel {
	if name == "" {
		name = FormatModelLabel(id)
	}
	if capKey == "" {
		capKey = id
	}

	lower := strings.ToLower(capKey)
	supportsReasoning := strings.Contains(lower, "r1") ||
		strings.Contains(lower, "o1") ||
		strings.Contains(lower, "o3") ||
		strings.Contains(lower, "o4") ||
		strings.Contains(lower, "3-7") ||
		strings.Contains(lower, "3.7") ||
		strings.Contains(lower, "thinking") ||
		strings.Contains(lower, "reasoner") ||
		strings.Contains(lower, "k2") ||
		strings.Contains(lower, "k3") ||
		strings.Contains(lower, "glm-4") ||
		strings.Contains(lower, "glm-5")

	return CatalogModel{
		ID:                id,
		Name:              name,
		SupportsReasoning: supportsReasoning,
	}
}
