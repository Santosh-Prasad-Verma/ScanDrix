// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package byok

import "strings"

// ResolveManagedSlotFromConfig provides backwards compatibility for callers
// using EnvLLMConfig (e.g., gateway.go).
func ResolveManagedSlotFromConfig(env EnvLLMConfig) *NormalizedModel {
	if env.DefaultModel == "" {
		if env.OpenAIBaseURL != "" {
			env.DefaultModel = "custom-model"
		} else {
			return nil
		}
	}
	mLower := strings.ToLower(env.DefaultModel)

	if strings.HasPrefix(mLower, "claude-") {
		if env.AnthropicKey != "" {
			return createManagedSlot(ProviderAnthropic, env.AnthropicKey, env.DefaultModel, nil)
		}
		if env.VertexKey != "" {
			return createManagedSlot(ProviderGoogleVertex, env.VertexKey, env.DefaultModel, map[string]string{
				"vertexLocation": env.VertexLocation,
			})
		}
	}

	if strings.HasPrefix(mLower, "gemini-") || strings.HasPrefix(mLower, "google/gemini") {
		modelName := env.DefaultModel
		if strings.HasPrefix(mLower, "google/") {
			modelName = env.DefaultModel[7:]
		}
		if env.GeminiKey != "" {
			return createManagedSlot(ProviderGoogleGemini, env.GeminiKey, modelName, nil)
		}
		if env.VertexKey != "" {
			return createManagedSlot(ProviderGoogleVertex, env.VertexKey, modelName, map[string]string{
				"vertexLocation": env.VertexLocation,
			})
		}
	}

	if strings.HasPrefix(mLower, "mistral") || strings.HasPrefix(mLower, "codestral") {
		if env.MistralKey != "" {
			modelName := env.DefaultModel
			if strings.HasPrefix(mLower, "mistral/") {
				modelName = env.DefaultModel[8:]
			}
			return createManagedSlot(ProviderOpenAICompatible, env.MistralKey, modelName, map[string]string{
				"baseURL": "https://api.mistral.ai/v1",
			})
		}
	}

	if strings.HasPrefix(mLower, "groq/") || (env.GroqKey != "" && (strings.HasPrefix(mLower, "llama-") || strings.Contains(mLower, "distill-llama"))) {
		if env.GroqKey != "" {
			modelName := env.DefaultModel
			if strings.HasPrefix(mLower, "groq/") {
				modelName = env.DefaultModel[5:]
			}
			return createManagedSlot(ProviderOpenAICompatible, env.GroqKey, modelName, map[string]string{
				"baseURL": "https://api.groq.com/openai/v1",
			})
		}
	}

	if strings.HasPrefix(mLower, "cohere/") || strings.HasPrefix(mLower, "command") {
		if env.CohereKey != "" {
			modelName := env.DefaultModel
			if strings.HasPrefix(mLower, "cohere/") {
				modelName = env.DefaultModel[7:]
			}
			return createManagedSlot(ProviderOpenAICompatible, env.CohereKey, modelName, map[string]string{
				"baseURL": "https://api.cohere.com/v2",
			})
		}
	}

	if strings.HasPrefix(mLower, "gpt-") || strings.HasPrefix(mLower, "o1") || strings.HasPrefix(mLower, "o3") || strings.HasPrefix(mLower, "o4") {
		if env.OpenAIKey != "" {
			return createManagedSlot(ProviderOpenAI, env.OpenAIKey, env.DefaultModel, nil)
		}
	}

	if strings.HasPrefix(mLower, "kimi-") || strings.HasPrefix(mLower, "moonshot") {
		if env.MoonshotKey != "" {
			return createManagedSlot(ProviderMoonshot, env.MoonshotKey, env.DefaultModel, nil)
		}
	}

	if strings.HasPrefix(mLower, "glm-") {
		if env.ZaiKey != "" {
			return createManagedSlot(ProviderZAI, env.ZaiKey, env.DefaultModel, nil)
		}
	}

	if env.OpenAIBaseURL != "" && env.OpenAIKey != "" {
		return createManagedSlot(ProviderOpenAICompatible, env.OpenAIKey, env.DefaultModel, map[string]string{
			"baseURL": env.OpenAIBaseURL,
		})
	}

	if env.OpenAIKey != "" {
		return createManagedSlot(ProviderOpenAI, env.OpenAIKey, env.DefaultModel, nil)
	}

	if env.OpenRouterKey != "" {
		return createManagedSlot(ProviderOpenRouter, env.OpenRouterKey, env.DefaultModel, nil)
	}

	if env.NovitaKey != "" {
		return createManagedSlot(ProviderNovita, env.NovitaKey, env.DefaultModel, nil)
	}

	if env.AzureKey != "" && env.AzureEndpoint != "" {
		return createManagedSlot(ProviderAzure, env.AzureKey, env.DefaultModel, map[string]string{
			"baseURL": env.AzureEndpoint,
		})
	}

	if env.MistralKey != "" {
		return createManagedSlot(ProviderOpenAICompatible, env.MistralKey, env.DefaultModel, map[string]string{
			"baseURL": "https://api.mistral.ai/v1",
		})
	}

	if env.GroqKey != "" {
		return createManagedSlot(ProviderOpenAICompatible, env.GroqKey, env.DefaultModel, map[string]string{
			"baseURL": "https://api.groq.com/openai/v1",
		})
	}

	if env.CohereKey != "" {
		return createManagedSlot(ProviderOpenAICompatible, env.CohereKey, env.DefaultModel, map[string]string{
			"baseURL": "https://api.cohere.com/v2",
		})
	}

	return nil
}

func createManagedSlot(provider BYOKProvider, apiKey, model string, settings map[string]string) *NormalizedModel {
	slot := &NormalizedModel{
		Provider: provider,
		APIKey:   apiKey,
		Model:    model,
	}
	if settings != nil {
		if u, ok := settings["baseURL"]; ok {
			slot.BaseURL = u
		}
		if loc, ok := settings["vertexLocation"]; ok {
			slot.VertexLocation = loc
		}
	}
	return slot
}
