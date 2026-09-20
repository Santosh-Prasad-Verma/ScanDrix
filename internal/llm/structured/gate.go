// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package structured

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

var (
	openRouterJSONSchemaPrefixes = []string{
		"openai/",
		"anthropic/",
		"google/",
		"moonshotai/",
	}

	port8000Regex = regexp.MustCompile(`:8000(/|$)`)

	unsupportedCache sync.Map
)

// OpenRouterHonorsJSONSchema returns true for OpenRouter model IDs proven to support json_schema.
func OpenRouterHonorsJSONSchema(model string) bool {
	lower := strings.ToLower(model)
	for _, prefix := range openRouterJSONSchemaPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// OpenAICompatibleHonorsJSONSchema uses endpoint heuristics to determine if an upstream supports json_schema.
func OpenAICompatibleHonorsJSONSchema(baseURL string) bool {
	if baseURL == "" {
		return false
	}
	// vLLM default port 8000
	if port8000Regex.MatchString(baseURL) {
		return true
	}
	// Fireworks AI endpoint
	if strings.Contains(strings.ToLower(baseURL), "api.fireworks.ai") {
		return true
	}
	// Trusted proxy allowlist via env var
	allowList := os.Getenv("SCANDRIX_TRUST_JSON_SCHEMA_BASE_URLS")
	if allowList != "" {
		needles := strings.Split(allowList, ",")
		for _, needle := range needles {
			clean := strings.TrimSpace(needle)
			if clean != "" && strings.Contains(baseURL, clean) {
				return true
			}
		}
	}
	return false
}

// IsNeverDowngradeModel returns true for models (e.g. Moonshot/Kimi) that must never be downgraded to json_object.
func IsNeverDowngradeModel(model string) bool {
	lower := strings.ToLower(model)
	return strings.Contains(lower, "kimi") || strings.Contains(lower, "moonshot")
}

func cacheKey(provider, model, baseURL string) string {
	return fmt.Sprintf("%s:%s:%s", provider, model, baseURL)
}

// MayUseJSONSchema checks whether this provider:model:endpoint combination has not failed json_schema in this process.
func MayUseJSONSchema(provider, model, baseURL string) bool {
	_, failed := unsupportedCache.Load(cacheKey(provider, model, baseURL))
	return !failed
}

// MarkJSONSchemaUnsupported records that a provider rejected json_schema so future calls skip directly to json_object.
func MarkJSONSchemaUnsupported(provider, model, baseURL string) {
	unsupportedCache.Store(cacheKey(provider, model, baseURL), true)
}

// IsJSONSchemaUnsupportedError checks whether an error message indicates rejection of response_format json_schema.
func IsJSONSchemaUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	mentionsSchema := strings.Contains(lower, "response_format") ||
		strings.Contains(lower, "json_schema") ||
		strings.Contains(lower, "structured output") ||
		strings.Contains(lower, "structured_output") ||
		strings.Contains(lower, "schema")

	mentionsUnsupported := strings.Contains(lower, "unsupported") ||
		strings.Contains(lower, "not supported") ||
		strings.Contains(lower, "invalid parameter") ||
		strings.Contains(lower, "additional properties") ||
		strings.Contains(lower, "unknown field") ||
		strings.Contains(lower, "400") ||
		strings.Contains(lower, "422")

	return mentionsSchema && mentionsUnsupported
}
