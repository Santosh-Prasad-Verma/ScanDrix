// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package agentloop

import (
	"strings"
)

// CacheHint represents vendor-specific prompt caching metadata.
type CacheHint map[string]any

// AnthropicEphemeralCache returns the ephemeral cache control block for Anthropic Claude.
func AnthropicEphemeralCache() CacheHint {
	return CacheHint{
		"type": "ephemeral",
	}
}

// ModelSupportsPromptCache checks whether the provider/model honors inline prompt caching markers.
func ModelSupportsPromptCache(provider, model string) bool {
	lowerProv := strings.ToLower(provider)
	lowerModel := strings.ToLower(model)

	if strings.Contains(lowerProv, "anthropic") || strings.Contains(lowerModel, "claude") {
		return true
	}
	if strings.Contains(lowerProv, "deepseek") || strings.Contains(lowerModel, "deepseek") {
		return true
	}
	if strings.Contains(lowerProv, "gemini") || strings.Contains(lowerModel, "gemini") {
		return true
	}
	return false
}

// ApplyCacheBreakpoints marks the static system prompt, latest user message, and last tool
// with ephemeral cache control breakpoints for multi-step agent loops.
func ApplyCacheBreakpoints(
	systemPrompt string,
	messages []map[string]any,
	tools []map[string]any,
	maxSteps int,
	provider, model string,
) (systemWithCache any, messagesWithCache []map[string]any, toolsWithCache []map[string]any) {
	// Cache breakpoints only pay back on multi-step loops
	if maxSteps <= 1 || !ModelSupportsPromptCache(provider, model) {
		return systemPrompt, messages, tools
	}

	hint := AnthropicEphemeralCache()

	// 1. Mark system prompt
	if systemPrompt != "" {
		systemWithCache = []map[string]any{
			{
				"type":          "text",
				"text":          systemPrompt,
				"cache_control": hint,
			},
		}
	} else {
		systemWithCache = systemPrompt
	}

	// 2. Mark latest user message
	messagesWithCache = make([]map[string]any, len(messages))
	copy(messagesWithCache, messages)

	for i := len(messagesWithCache) - 1; i >= 0; i-- {
		role, _ := messagesWithCache[i]["role"].(string)
		if role == "user" {
			target := make(map[string]any)
			for k, v := range messagesWithCache[i] {
				target[k] = v
			}
			target["cache_control"] = hint
			messagesWithCache[i] = target
			break
		}
	}

	// 3. Mark last tool definition
	toolsWithCache = make([]map[string]any, len(tools))
	copy(toolsWithCache, tools)

	if len(toolsWithCache) > 0 {
		lastIdx := len(toolsWithCache) - 1
		target := make(map[string]any)
		for k, v := range toolsWithCache[lastIdx] {
			target[k] = v
		}
		target["cache_control"] = hint
		toolsWithCache[lastIdx] = target
	}

	return systemWithCache, messagesWithCache, toolsWithCache
}
