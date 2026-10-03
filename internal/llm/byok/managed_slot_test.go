// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package byok

import (
	"os"
	"testing"
)

var testEnvKeys = []string{
	"API_LLM_PROVIDER_MODEL",
	"API_OPEN_AI_API_KEY",
	"API_OPENAI_FORCE_BASE_URL",
	"API_VERTEX_AI_API_KEY",
	"API_VERTEX_AI_LOCATION",
	"API_GOOGLE_AI_API_KEY",
	"GOOGLE_GENERATIVE_AI_API_KEY",
	"API_FIREWORKS_API_KEY",
	"FIREWORKS_API_KEY",
	"API_FIREWORKS_BASE_URL",
	"API_DEEPSEEK_API_KEY",
	"DEEPSEEK_API_KEY",
	"API_MOONSHOT_API_KEY",
	"MOONSHOT_API_KEY",
	"GOOGLE_CLOUD_PROJECT",
	"GCLOUD_PROJECT",
}

func clearTestEnv(t *testing.T) func() {
	t.Helper()
	saved := make(map[string]string)
	for _, k := range testEnvKeys {
		if val, exists := os.LookupEnv(k); exists {
			saved[k] = val
		}
		_ = os.Unsetenv(k)
	}
	return func() {
		for _, k := range testEnvKeys {
			if val, exists := saved[k]; exists {
				_ = os.Setenv(k, val)
			} else {
				_ = os.Unsetenv(k)
			}
		}
	}
}

func TestResolveEnvProvider(t *testing.T) {
	t.Run("returns null in cloud (auto) mode", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		if res := ResolveEnvProvider(); res != nil {
			t.Fatalf("expected nil when unset, got %+v", res)
		}

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "auto")
		if res := ResolveEnvProvider(); res != nil {
			t.Fatalf("expected nil for 'auto', got %+v", res)
		}
	})

	t.Run("gemini-* + AI Studio key -> google_ai_studio", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "gemini-2.5-pro")
		_ = os.Setenv("API_GOOGLE_AI_API_KEY", "aistudio-key")

		res := ResolveEnvProvider()
		if res == nil {
			t.Fatalf("expected non-nil resolution")
		}
		if res.Kind != "gemini_studio" || res.Name != "google_ai_studio" || res.APIKey != "aistudio-key" {
			t.Fatalf("unexpected resolution: %+v", res)
		}
	})

	t.Run("gemini-* with only a Vertex key -> google_vertex (with location)", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "gemini-2.5-flash")
		_ = os.Setenv("API_VERTEX_AI_API_KEY", "vertex-sa-json")
		_ = os.Setenv("API_VERTEX_AI_LOCATION", "us-east5")

		res := ResolveEnvProvider()
		if res == nil {
			t.Fatalf("expected non-nil resolution")
		}
		if res.Kind != "gemini_vertex" || res.Name != "google_vertex" || res.APIKey != "vertex-sa-json" || res.VertexLocation != "us-east5" {
			t.Fatalf("unexpected resolution: %+v", res)
		}
	})

	t.Run("AI Studio key wins over Vertex key for gemini-*", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "gemini-2.5-pro")
		_ = os.Setenv("API_GOOGLE_AI_API_KEY", "studio")
		_ = os.Setenv("API_VERTEX_AI_API_KEY", "vertex")

		res := ResolveEnvProvider()
		if res == nil || res.Kind != "gemini_studio" {
			t.Fatalf("expected gemini_studio to win, got %+v", res)
		}
	})

	t.Run("claude-* + native OpenAI-slot key (no proxy) -> anthropic native", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "claude-sonnet-4-5")
		_ = os.Setenv("API_OPEN_AI_API_KEY", "sk-ant")

		res := ResolveEnvProvider()
		if res == nil {
			t.Fatalf("expected non-nil resolution")
		}
		if res.Kind != "claude_anthropic" || res.Name != "anthropic" || res.APIKey != "sk-ant" || res.BaseURL != "" {
			t.Fatalf("unexpected resolution: %+v", res)
		}
	})

	t.Run("native Anthropic key takes precedence over Claude-on-Vertex", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "claude-opus-4-5")
		_ = os.Setenv("API_OPEN_AI_API_KEY", "sk-ant")
		_ = os.Setenv("API_VERTEX_AI_API_KEY", "vertex")

		res := ResolveEnvProvider()
		if res == nil || res.Kind != "claude_anthropic" {
			t.Fatalf("expected claude_anthropic to win, got %+v", res)
		}
	})

	t.Run("claude-* with only a Vertex key -> claude_vertex", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "claude-3-5-sonnet")
		_ = os.Setenv("API_VERTEX_AI_API_KEY", "vertex-sa")
		_ = os.Setenv("API_VERTEX_AI_LOCATION", "us-central1")

		res := ResolveEnvProvider()
		if res == nil {
			t.Fatalf("expected non-nil resolution")
		}
		if res.Kind != "claude_vertex" || res.Name != "google_vertex" || res.APIKey != "vertex-sa" || res.VertexLocation != "us-central1" {
			t.Fatalf("unexpected resolution: %+v", res)
		}
	})

	t.Run("any other model + OpenAI-style key -> openai_compatible with default baseURL", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "llama-3.3-70b")
		_ = os.Setenv("API_OPEN_AI_API_KEY", "sk-x")

		res := ResolveEnvProvider()
		if res == nil {
			t.Fatalf("expected non-nil resolution")
		}
		if res.Kind != "openai_compat" || res.Name != "openai_compatible" || res.APIKey != "sk-x" || res.BaseURL != "https://api.openai.com/v1" {
			t.Fatalf("unexpected resolution: %+v", res)
		}
	})

	t.Run("a forced proxy baseURL routes a gemini/claude id through openai_compatible", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "claude-sonnet-4-5")
		_ = os.Setenv("API_OPEN_AI_API_KEY", "proxy-key")
		_ = os.Setenv("API_OPENAI_FORCE_BASE_URL", "https://openrouter.ai/api/v1")

		res := ResolveEnvProvider()
		if res == nil {
			t.Fatalf("expected non-nil resolution")
		}
		if res.Kind != "openai_compat" || res.Name != "openai_compatible" || res.APIKey != "proxy-key" || res.BaseURL != "https://openrouter.ai/api/v1" {
			t.Fatalf("unexpected resolution: %+v", res)
		}
	})

	t.Run("an explicit api.anthropic.com baseURL stays Anthropic native", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "claude-sonnet-4-5")
		_ = os.Setenv("API_OPEN_AI_API_KEY", "sk-ant")
		_ = os.Setenv("API_OPENAI_FORCE_BASE_URL", "https://api.anthropic.com")

		res := ResolveEnvProvider()
		if res == nil {
			t.Fatalf("expected non-nil resolution")
		}
		if res.Kind != "claude_anthropic" || res.BaseURL != "https://api.anthropic.com" {
			t.Fatalf("unexpected resolution: %+v", res)
		}
	})

	t.Run("self-hosted mode with no usable key -> null (falls through to cloud default)", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "gemini-2.5-pro")

		if res := ResolveEnvProvider(); res != nil {
			t.Fatalf("expected nil when no key is set, got %+v", res)
		}
	})
}

func TestResolveEnvProviderKeylessVertexADC(t *testing.T) {
	t.Run("gemini-* + ambient project, no keys -> vertex_adc (isClaude false)", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "gemini-2.5-pro")
		_ = os.Setenv("GOOGLE_CLOUD_PROJECT", "adc-proj")

		res := ResolveEnvProvider()
		if res == nil {
			t.Fatalf("expected non-nil resolution")
		}
		if res.Kind != "vertex_adc" || res.Name != "google_vertex" || res.Project != "adc-proj" || res.IsClaude != false {
			t.Fatalf("unexpected resolution: %+v", res)
		}
	})

	t.Run("accepts GCLOUD_PROJECT as an alias", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "gemini-2.5-pro")
		_ = os.Setenv("GCLOUD_PROJECT", "alias-proj")

		res := ResolveEnvProvider()
		if res == nil || res.Kind != "vertex_adc" || res.Project != "alias-proj" {
			t.Fatalf("unexpected resolution: %+v", res)
		}
	})

	t.Run("claude-* + ambient project, no keys -> vertex_adc (isClaude true)", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "claude-sonnet-4-5")
		_ = os.Setenv("GOOGLE_CLOUD_PROJECT", "env-proj")

		res := ResolveEnvProvider()
		if res == nil || res.Kind != "vertex_adc" || res.Project != "env-proj" || !res.IsClaude {
			t.Fatalf("unexpected resolution: %+v", res)
		}
	})

	t.Run("an explicit Vertex SA key WINS over ADC (gemini)", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "gemini-2.5-pro")
		_ = os.Setenv("GOOGLE_CLOUD_PROJECT", "adc-proj")
		_ = os.Setenv("API_VERTEX_AI_API_KEY", "sa-json")

		res := ResolveEnvProvider()
		if res == nil || res.Kind != "gemini_vertex" {
			t.Fatalf("expected gemini_vertex, got %+v", res)
		}
	})

	t.Run("an explicit API_OPEN_AI_API_KEY WINS over ambient ADC (gemini)", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "gemini-2.5-pro")
		_ = os.Setenv("GOOGLE_CLOUD_PROJECT", "ambient-proj")
		_ = os.Setenv("API_OPEN_AI_API_KEY", "sk-openai")

		res := ResolveEnvProvider()
		if res == nil || res.Kind != "openai_compat" {
			t.Fatalf("expected openai_compat, got %+v", res)
		}
	})

	t.Run("no project -> no ADC (falls through)", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "gemini-2.5-pro")

		if res := ResolveEnvProvider(); res != nil {
			t.Fatalf("expected nil when no project, got %+v", res)
		}
	})
}

func TestResolveManagedSlot(t *testing.T) {
	t.Run("env gemini_studio -> GOOGLE_GEMINI slot carrying the env model + plaintext key", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "gemini-2.5-pro")
		_ = os.Setenv("API_GOOGLE_AI_API_KEY", "studio-key")

		r := ResolveManagedSlot("unused-default", ByokModelOptions{})
		if r == nil || r.Kind != ManagedKindSlot || r.Slot == nil {
			t.Fatalf("expected slot resolution, got %+v", r)
		}
		if r.Slot.Provider != ProviderGoogleGemini || r.Slot.APIKey != "studio-key" || r.Slot.Model != "gemini-2.5-pro" {
			t.Fatalf("unexpected slot: %+v", r.Slot)
		}
	})

	t.Run("env vertex (gemini or claude) -> the single GOOGLE_VERTEX slot with location", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "gemini-2.5-flash")
		_ = os.Setenv("API_VERTEX_AI_API_KEY", "sa-json")
		_ = os.Setenv("API_VERTEX_AI_LOCATION", "us-east5")

		r := ResolveManagedSlot("x", ByokModelOptions{})
		if r == nil || r.Kind != ManagedKindSlot || r.Slot == nil {
			t.Fatalf("expected slot resolution, got %+v", r)
		}
		if r.Slot.Provider != ProviderGoogleVertex || r.Slot.Model != "gemini-2.5-flash" || r.Slot.VertexLocation != "us-east5" {
			t.Fatalf("unexpected slot: %+v", r.Slot)
		}
	})

	t.Run("env claude_anthropic -> ANTHROPIC slot (baseURL threaded)", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "claude-sonnet-4-5")
		_ = os.Setenv("API_OPEN_AI_API_KEY", "sk-ant")

		r := ResolveManagedSlot("x", ByokModelOptions{})
		if r == nil || r.Kind != ManagedKindSlot || r.Slot == nil {
			t.Fatalf("expected slot resolution, got %+v", r)
		}
		if r.Slot.Provider != ProviderAnthropic || r.Slot.APIKey != "sk-ant" || r.Slot.Model != "claude-sonnet-4-5" {
			t.Fatalf("unexpected slot: %+v", r.Slot)
		}
	})

	t.Run("env openai_compat -> INLINE exception built from the env baseURL/key", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "llama-3.3-70b")
		_ = os.Setenv("API_OPEN_AI_API_KEY", "sk-x")

		r := ResolveManagedSlot("x", ByokModelOptions{StructuredOutputs: true})
		if r == nil || r.Kind != ManagedKindInline || r.Inline == nil {
			t.Fatalf("expected inline resolution, got %+v", r)
		}
		if r.Inline.Name != "self-hosted" || r.Inline.APIKey != "sk-x" || r.Inline.BaseURL != "https://api.openai.com/v1" || !r.Inline.SupportsStructuredOutputs {
			t.Fatalf("unexpected inline config: %+v", r.Inline)
		}
	})

	t.Run("cloud + fireworks default model -> INLINE fireworks (the managed default)", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_FIREWORKS_API_KEY", "fw-key")

		r := ResolveManagedSlot("accounts/fireworks/models/deepseek-v4-flash-0731", ByokModelOptions{})
		if r == nil || r.Kind != ManagedKindInline || r.Inline == nil {
			t.Fatalf("expected inline resolution, got %+v", r)
		}
		if r.Inline.Name != "fireworks" || r.Inline.APIKey != "fw-key" || !r.Inline.SupportsStructuredOutputs {
			t.Fatalf("unexpected fireworks config: %+v", r.Inline)
		}
	})

	t.Run("deepseek-* default -> INLINE deepseek (legacy fallback)", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_DEEPSEEK_API_KEY", "ds-key")

		r := ResolveManagedSlot("deepseek-chat", ByokModelOptions{})
		if r == nil || r.Kind != ManagedKindInline || r.Inline == nil {
			t.Fatalf("expected inline resolution, got %+v", r)
		}
		if r.Inline.Name != "deepseek" || r.Inline.APIKey != "ds-key" {
			t.Fatalf("unexpected deepseek config: %+v", r.Inline)
		}
	})

	t.Run("kimi-* default -> MOONSHOT slot (routed through the registry, not inline)", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_MOONSHOT_API_KEY", "moon-key")

		r := ResolveManagedSlot("kimi-k2", ByokModelOptions{})
		if r == nil || r.Kind != ManagedKindSlot || r.Slot == nil {
			t.Fatalf("expected slot resolution, got %+v", r)
		}
		if r.Slot.Provider != ProviderMoonshot || r.Slot.APIKey != "moon-key" || r.Slot.Model != "kimi-k2" {
			t.Fatalf("unexpected slot: %+v", r.Slot)
		}
	})

	t.Run("any other cloud default -> GOOGLE_GEMINI slot", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_GOOGLE_AI_API_KEY", "g-key")

		r := ResolveManagedSlot("gemini-3-pro-preview", ByokModelOptions{})
		if r == nil || r.Kind != ManagedKindSlot || r.Slot == nil {
			t.Fatalf("expected slot resolution, got %+v", r)
		}
		if r.Slot.Provider != ProviderGoogleGemini || r.Slot.APIKey != "g-key" || r.Slot.Model != "gemini-3-pro-preview" {
			t.Fatalf("unexpected slot: %+v", r.Slot)
		}
	})

	t.Run("managed slots carry a PLAINTEXT env key", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_GOOGLE_AI_API_KEY", "plain-key")

		r := ResolveManagedSlot("gemini-3-pro-preview", ByokModelOptions{})
		if r == nil || r.Slot == nil || r.Slot.APIKey != "plain-key" {
			t.Fatalf("expected plaintext APIKey 'plain-key', got %+v", r)
		}
	})

	t.Run("builds the keyless Vertex model inline, passing project + location", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "gemini-2.5-pro")
		_ = os.Setenv("GOOGLE_CLOUD_PROJECT", "env-proj")
		_ = os.Setenv("API_VERTEX_AI_LOCATION", "us-east5")

		res := ResolveManagedSlot("gemini-2.5-pro", ByokModelOptions{})
		if res == nil || res.Kind != ManagedKindInline || res.Inline == nil {
			t.Fatalf("expected inline resolution, got %+v", res)
		}
		if res.Inline.Model != "gemini-2.5-pro" || res.Inline.VertexLocation != "us-east5" {
			t.Fatalf("unexpected inline config: %+v", res.Inline)
		}
	})

	t.Run("a failed ADC build falls through to the managed/cloud default", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		origHook := ADCModelBuilder
		ADCModelBuilder = func(model, project, location string) *InlineModelConfig {
			return nil // simulate failed ADC build
		}
		defer func() { ADCModelBuilder = origHook }()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "gemini-2.5-pro")
		_ = os.Setenv("GOOGLE_CLOUD_PROJECT", "env-proj")
		_ = os.Setenv("API_GOOGLE_AI_API_KEY", "fallback-key")

		res := ResolveManagedSlot("gemini-2.5-pro", ByokModelOptions{})
		if res == nil || res.Kind != ManagedKindSlot || res.Slot == nil {
			t.Fatalf("expected degraded slot resolution, got %+v", res)
		}
		if res.Slot.Provider != ProviderGoogleGemini || res.Slot.APIKey != "fallback-key" {
			t.Fatalf("expected degraded google_gemini slot, got %+v", res.Slot)
		}
	})
}

func TestHasManagedModelKey(t *testing.T) {
	t.Run("cloud: true only when a Fireworks key is present (the managed default)", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		if HasManagedModelKey() {
			t.Fatalf("expected false when no keys present")
		}

		_ = os.Setenv("API_FIREWORKS_API_KEY", "fw")
		if !HasManagedModelKey() {
			t.Fatalf("expected true when Fireworks key is present")
		}
	})

	t.Run("cloud: a stale DeepSeek key does NOT count", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_DEEPSEEK_API_KEY", "ds")
		if HasManagedModelKey() {
			t.Fatalf("expected false for stale deepseek key")
		}
	})

	t.Run("self-hosted: true when any relevant provider key is present", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "gemini-2.5-pro")
		if HasManagedModelKey() {
			t.Fatalf("expected false before setting provider key")
		}

		_ = os.Setenv("API_GOOGLE_AI_API_KEY", "g")
		if !HasManagedModelKey() {
			t.Fatalf("expected true after setting Google key")
		}
	})

	t.Run("self-hosted Gemini + ambient project (ADC), no keys -> true", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "gemini-2.5-pro")
		_ = os.Setenv("GOOGLE_CLOUD_PROJECT", "adc-proj")

		if !HasManagedModelKey() {
			t.Fatalf("expected true for Gemini + ADC project")
		}
	})

	t.Run("self-hosted NON-Google model + ambient project, no keys -> false", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "some-openai-model")
		_ = os.Setenv("GOOGLE_CLOUD_PROJECT", "ambient-proj")

		if HasManagedModelKey() {
			t.Fatalf("expected false for non-Google/Claude model with only ambient project")
		}
	})
}

func TestGetModelName(t *testing.T) {
	t.Run("reports slot provider:model when slot is provided", func(t *testing.T) {
		slot := &NormalizedModel{
			Provider: ProviderAnthropic,
			Model:    "claude-3-7-sonnet",
		}
		name := GetModelName(slot, "")
		if name != "anthropic:claude-3-7-sonnet" {
			t.Fatalf("expected 'anthropic:claude-3-7-sonnet', got %s", name)
		}
	})

	t.Run("reports google_vertex for a model on ADC", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		_ = os.Setenv("API_LLM_PROVIDER_MODEL", "gemini-2.5-pro")
		_ = os.Setenv("GOOGLE_CLOUD_PROJECT", "env-proj")

		name := GetModelName(nil, "")
		if name != "google_vertex:gemini-2.5-pro" {
			t.Fatalf("expected 'google_vertex:gemini-2.5-pro', got %s", name)
		}
	})

	t.Run("reports default model when neither slot nor env is set", func(t *testing.T) {
		restore := clearTestEnv(t)
		defer restore()

		name := GetModelName(nil, "")
		if name != DefaultModelID {
			t.Fatalf("expected '%s', got '%s'", DefaultModelID, name)
		}

		override := GetModelName(nil, "my-override-model")
		if override != "my-override-model" {
			t.Fatalf("expected 'my-override-model', got '%s'", override)
		}
	})
}

func TestResolveManagedSlotFromConfig(t *testing.T) {
	cfg := EnvLLMConfig{}
	if slot := ResolveManagedSlotFromConfig(cfg); slot != nil {
		t.Fatalf("expected nil slot for empty config, got %v", slot)
	}

	cfgAnthropic := EnvLLMConfig{
		DefaultModel: "claude-3-7-sonnet",
		AnthropicKey: "sk-ant-test",
		OpenAIKey:    "sk-openai-test",
	}
	slotAnthropic := ResolveManagedSlotFromConfig(cfgAnthropic)
	if slotAnthropic == nil || slotAnthropic.Provider != ProviderAnthropic || slotAnthropic.APIKey != "sk-ant-test" {
		t.Fatalf("unexpected anthropic slot: %+v", slotAnthropic)
	}

	cfgGemini := EnvLLMConfig{
		DefaultModel: "gemini-2.5-pro",
		GeminiKey:    "gemini-test-key",
	}
	slotGemini := ResolveManagedSlotFromConfig(cfgGemini)
	if slotGemini == nil || slotGemini.Provider != ProviderGoogleGemini || slotGemini.APIKey != "gemini-test-key" {
		t.Fatalf("unexpected gemini slot: %+v", slotGemini)
	}

	cfgMistral := EnvLLMConfig{
		DefaultModel: "mistral-large-latest",
		MistralKey:   "mistral-test-key",
	}
	slotMistral := ResolveManagedSlotFromConfig(cfgMistral)
	if slotMistral == nil || slotMistral.Provider != ProviderOpenAICompatible || slotMistral.APIKey != "mistral-test-key" || slotMistral.BaseURL != "https://api.mistral.ai/v1" {
		t.Fatalf("unexpected mistral slot: %+v", slotMistral)
	}

	cfgGroq := EnvLLMConfig{
		DefaultModel: "llama-3.3-70b-versatile",
		GroqKey:      "groq-test-key",
	}
	slotGroq := ResolveManagedSlotFromConfig(cfgGroq)
	if slotGroq == nil || slotGroq.Provider != ProviderOpenAICompatible || slotGroq.APIKey != "groq-test-key" || slotGroq.BaseURL != "https://api.groq.com/openai/v1" {
		t.Fatalf("unexpected groq slot: %+v", slotGroq)
	}

	cfgCohere := EnvLLMConfig{
		DefaultModel: "command-r-plus",
		CohereKey:    "cohere-test-key",
	}
	slotCohere := ResolveManagedSlotFromConfig(cfgCohere)
	if slotCohere == nil || slotCohere.Provider != ProviderOpenAICompatible || slotCohere.APIKey != "cohere-test-key" || slotCohere.BaseURL != "https://api.cohere.com/v2" {
		t.Fatalf("unexpected cohere slot: %+v", slotCohere)
	}
}
