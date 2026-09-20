// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package byok

import (
	"os"
	"strings"
)

// EnvLLMConfig captures the system-managed AI provider environment configuration.
type EnvLLMConfig struct {
	AnthropicKey    string
	OpenAIKey       string
	GeminiKey       string
	VertexKey       string
	VertexLocation  string
	VertexProject   string
	BedrockToken    string
	BedrockRegion   string
	OpenRouterKey   string
	NovitaKey       string
	MoonshotKey     string
	ZaiKey          string
	AzureKey        string
	AzureEndpoint   string
	FireworksKey    string
	OpenAIBaseURL   string
	DefaultModel    string
	DefaultProvider string
}

func getEnvFirst(keys ...string) string {
	for _, k := range keys {
		v := strings.TrimSpace(os.Getenv(k))
		if v != "" {
			return v
		}
	}
	return ""
}

// LoadEnvLLMConfig reads all supported system-managed API keys and provider configurations.
func LoadEnvLLMConfig() EnvLLMConfig {
	return EnvLLMConfig{
		AnthropicKey:    getEnvFirst("ANTHROPIC_API_KEY", "API_ANTHROPIC_API_KEY"),
		OpenAIKey:       getEnvFirst("OPENAI_API_KEY", "API_OPEN_AI_API_KEY"),
		GeminiKey:       getEnvFirst("GEMINI_API_KEY", "API_GOOGLE_AI_API_KEY", "GOOGLE_GENERATIVE_AI_API_KEY"),
		VertexKey:       getEnvFirst("VERTEX_ACCESS_TOKEN", "API_VERTEX_AI_API_KEY"),
		VertexLocation:  getEnvFirst("VERTEX_LOCATION", "API_VERTEX_AI_LOCATION"),
		VertexProject:   getEnvFirst("VERTEX_PROJECT_ID", "GOOGLE_CLOUD_PROJECT", "GCLOUD_PROJECT"),
		BedrockToken:    getEnvFirst("BEDROCK_BEARER_TOKEN", "API_BEDROCK_BEARER_TOKEN"),
		BedrockRegion:   getEnvFirst("BEDROCK_REGION", "AWS_REGION", "AWS_DEFAULT_REGION"),
		OpenRouterKey:   getEnvFirst("OPENROUTER_API_KEY", "API_OPEN_ROUTER_API_KEY"),
		NovitaKey:       getEnvFirst("NOVITA_API_KEY", "API_NOVITA_AI_API_KEY"),
		MoonshotKey:     getEnvFirst("MOONSHOT_API_KEY", "API_MOONSHOT_API_KEY"),
		ZaiKey:          getEnvFirst("ZAI_API_KEY", "API_ZAI_API_KEY"),
		AzureKey:        getEnvFirst("AZURE_OPENAI_API_KEY", "API_AZURE_OPENAI_API_KEY"),
		AzureEndpoint:   getEnvFirst("AZURE_OPENAI_ENDPOINT", "API_AZURE_OPENAI_ENDPOINT"),
		FireworksKey:    getEnvFirst("FIREWORKS_API_KEY", "API_FIREWORKS_API_KEY"),
		OpenAIBaseURL:   getEnvFirst("OPENAI_BASE_URL", "API_OPENAI_FORCE_BASE_URL"),
		DefaultModel:    getEnvFirst("API_LLM_PROVIDER_MODEL", "AI_MODEL_DEFAULT", "SCANDRIX_DEFAULT_MODEL"),
		DefaultProvider: getEnvFirst("API_LLM_PROVIDER", "AI_PROVIDER_DEFAULT", "SCANDRIX_DEFAULT_PROVIDER"),
	}
}

// HasManagedCredentials returns true if at least one system-managed provider key is set.
func (c EnvLLMConfig) HasManagedCredentials() bool {
	return c.AnthropicKey != "" ||
		c.OpenAIKey != "" ||
		c.GeminiKey != "" ||
		c.VertexKey != "" ||
		c.BedrockToken != "" ||
		c.OpenRouterKey != "" ||
		c.NovitaKey != "" ||
		c.MoonshotKey != "" ||
		c.ZaiKey != "" ||
		c.AzureKey != "" ||
		c.FireworksKey != ""
}
