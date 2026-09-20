// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package contextwin

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
)

//go:embed model-context-windows.json
var embeddedModelData []byte

// DefaultContextWindowTokens is the conservative default when the model is unknown.
const DefaultContextWindowTokens = 128_000

var (
	manualOverrides = map[string]int{
		// OpenAI
		"gpt54":      1_000_000,
		"gpt54mini":  272_000,
		"gpt5":       400_000,
		"gpt5mini":   400_000,
		"gptoss":     131_072,
		"gptoss120b": 131_072,
		// Anthropic
		"claudesonnet45": 200_000,
		"claudeopus45":   200_000,
		"claudeopus4":    200_000,
		"claudesonnet4":  200_000,
		// Google
		"gemini31pro":   1_048_576,
		"gemini3pro":    1_048_576,
		"gemini3flash":  1_048_576,
		"gemini25pro":   1_048_576,
		"gemini25flash": 1_048_576,
		// Moonshot
		"kimik27": 262_144,
		"kimik25": 262_144,
		"kimik2":  262_144,
		// DeepSeek
		"deepseekv4flash": 128_000,
		"deepseekr1":      128_000,
		"deepseekv3":      128_000,
		// Z.ai
		"glm51": 200_000,
		"glm5":  200_000,
		"glm47": 202_752,
		"glm46": 200_000,
		"glm45": 131_072,
		// Alibaba Qwen
		"qwen35":     262_144,
		"qwen3coder": 262_144,
	}

	providerPrefixRegex = regexp.MustCompile(`^(openai|anthropic|google|gemini|vertex_ai|bedrock|azure|together_ai|openrouter|novita|fireworks_ai|deepseek|mistral|moonshot|hf|huggingface)/`)
	separatorRegex      = regexp.MustCompile(`[-_.\s/:]`)

	initOnce        sync.Once
	directIndex     map[string]int
	normalizedIndex map[string]int
)

type modelJSONEntry struct {
	MaxInputTokens   int    `json:"max_input_tokens"`
	LiteLLMProvider  string `json:"litellm_provider,omitempty"`
}

func normalize(name string) string {
	s := strings.ToLower(name)
	s = providerPrefixRegex.ReplaceAllString(s, "")
	s = separatorRegex.ReplaceAllString(s, "")
	return s
}

func ensureInitialized() {
	initOnce.Do(func() {
		directIndex = make(map[string]int)
		normalizedIndex = make(map[string]int)

		var raw map[string]modelJSONEntry
		if err := json.Unmarshal(embeddedModelData, &raw); err == nil {
			for name, entry := range raw {
				if entry.MaxInputTokens > 0 {
					directIndex[name] = entry.MaxInputTokens
					norm := normalize(name)
					normalizedIndex[norm] = entry.MaxInputTokens
				}
			}
		}
	})
}

// GetModelContextWindow resolves the maximum input tokens for a given model.
// Resolution order:
//  1. Manual overrides (highest priority)
//  2. Exact match in database
//  3. Normalized match
//  4. Substring match on normalized names
//  5. Default (128,000 tokens)
func GetModelContextWindow(modelName string) int {
	cleanName := strings.TrimSpace(modelName)
	if cleanName == "" {
		return DefaultContextWindowTokens
	}

	norm := normalize(cleanName)

	// 1. Manual overrides (exact & substring)
	if val, ok := manualOverrides[norm]; ok {
		return val
	}
	for overrideKey, tokens := range manualOverrides {
		if strings.Contains(norm, overrideKey) {
			return tokens
		}
	}

	ensureInitialized()

	// 2. Exact match in database
	if val, ok := directIndex[cleanName]; ok {
		return val
	}

	// 3. Normalized match
	if val, ok := normalizedIndex[norm]; ok {
		return val
	}

	// 4. Substring match on database
	bestMatch := 0
	bestKeyLen := 0
	for key, tokens := range normalizedIndex {
		if len(key) > bestKeyLen && (strings.Contains(norm, key) || strings.Contains(key, norm)) {
			bestMatch = tokens
			bestKeyLen = len(key)
		}
	}
	if bestMatch > 0 {
		return bestMatch
	}

	// 5. Fallback default
	return DefaultContextWindowTokens
}

// ResolveContextWindow resolves the effective context window with preference to user-configured max tokens.
func ResolveContextWindow(byokMaxInputTokens int, modelName string) int {
	if byokMaxInputTokens > 0 {
		return byokMaxInputTokens
	}
	return GetModelContextWindow(modelName)
}
