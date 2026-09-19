// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Agent Architecture
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package conversation

import (
	"encoding/json"
	"strings"
)

// Fallback user-facing messages.
const (
	ConversationFallbackMessage = "I wasn't able to put together an answer for that. Could you rephrase your question or add a bit more context?"
	ConversationProviderErrorMessage = "I hit a technical issue while processing this and could not generate a reply. Please try again in a few minutes."
)

// NormalizeConversationResponse extracts the plain answer from complex JSON or envelope responses.
// Unwraps up to 4 levels of nested JSON or { "content": ... } wrappers produced by LLMs.
func NormalizeConversationResponse(raw any) string {
	if raw == nil {
		return ""
	}

	current := raw
	for depth := 0; depth < 4; depth++ {
		switch v := current.(type) {
		case string:
			trimmed := strings.TrimSpace(v)
			if trimmed == "" {
				return ""
			}

			// If wrapped in JSON code fence, strip it
			if strings.HasPrefix(trimmed, "```") {
				lines := strings.Split(trimmed, "\n")
				if len(lines) >= 2 && strings.HasPrefix(lines[0], "```") && strings.HasSuffix(lines[len(lines)-1], "```") {
					inner := strings.Join(lines[1:len(lines)-1], "\n")
					trimmed = strings.TrimSpace(inner)
				}
			}

			// Check if it's a JSON object envelope
			if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
				var parsed map[string]any
				if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
					if content, hasContent := parsed["content"]; hasContent {
						current = content
						continue
					}
					if answer, hasAnswer := parsed["answer"]; hasAnswer {
						current = answer
						continue
					}
					if response, hasResponse := parsed["response"]; hasResponse {
						current = response
						continue
					}
				}
			}
			return trimmed

		case map[string]any:
			if content, hasContent := v["content"]; hasContent {
				current = content
				continue
			}
			if answer, hasAnswer := v["answer"]; hasAnswer {
				current = answer
				continue
			}
			if response, hasResponse := v["response"]; hasResponse {
				current = response
				continue
			}
			return ""

		default:
			bytes, err := json.Marshal(v)
			if err != nil {
				return ""
			}
			current = string(bytes)
		}
	}

	return ""
}
