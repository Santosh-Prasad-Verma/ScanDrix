package integration_test

import (
	"os"
	"testing"
)

// TestMain makes this package hermetic with respect to LLM credentials.
//
// The gateway treats a configured base URL as an override only when no
// OPENAI_BASE_URL is present in the environment:
//
//	if g.localEndpoint != "" && env.OpenAIBaseURL == "" {
//	    env.OpenAIBaseURL = g.localEndpoint
//	}
//
// So a developer with real provider credentials in their shell or .env caused
// these tests to ignore their httptest mock and issue live API calls against a
// paid provider. That is slow, non-deterministic, costs money, and in CI can
// leak a real key to a third party.
//
// Clearing the variables here keeps every test in this package on its mock
// server. Tests that genuinely need a provider must opt in explicitly.
func TestMain(m *testing.M) {
	// Provider keys and endpoints. Empty values stop the gateway from
	// selecting a real backend.
	for _, key := range []string{
		"OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENAI_ORGANIZATION",
		"API_OPEN_AI_API_KEY", "API_OPENAI_FORCE_BASE_URL", "API_OPEN_ROUTER_API_KEY",
		"ANTHROPIC_API_KEY", "API_ANTHROPIC_API_KEY",
		"GEMINI_API_KEY", "GOOGLE_API_KEY", "API_GOOGLE_AI_API_KEY",
		"NVIDIA_API_KEY", "API_NVIDIA_API_KEY",
		"BEDROCK_BEARER_TOKEN", "BEDROCK_REGION", "AWS_BEARER_TOKEN_BEDROCK",
		"AZURE_OPENAI_API_KEY", "AZURE_OPENAI_ENDPOINT",
		"DEEPSEEK_API_KEY", "API_DEEPSEEK_API_KEY", "API_DEEPSEEK_BASE_URL",
		"FIREWORKS_API_KEY", "API_FIREWORKS_API_KEY", "API_FIREWORKS_BASE_URL",
		"MOONSHOT_API_KEY", "API_MOONSHOT_API_KEY",
		"NOVITA_API_KEY", "API_NOVITA_AI_API_KEY",
		"API_LLM_PROVIDER_MODEL",
	} {
		_ = os.Setenv(key, "")
	}

	os.Exit(m.Run())
}
