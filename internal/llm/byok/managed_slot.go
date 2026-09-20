// Copyright 2026 ScanDrix AI. All rights reserved.
// Use of this source code is governed by an enterprise license.

package byok

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Model-name protocol patterns.
var (
	claudeModelPattern    = regexp.MustCompile(`(?i)^claude[-_]`)
	geminiModelPattern    = regexp.MustCompile(`(?i)^gemini[-_]`)
	fireworksPattern      = regexp.MustCompile(`(?i)^accounts/fireworks/models/`)
	deepseekPattern       = regexp.MustCompile(`(?i)^deepseek[-_.]`)
	kimiPattern           = regexp.MustCompile(`(?i)^kimi[-_.]`)
	proxyAnthropicPattern = regexp.MustCompile(`(?i)(^|//)api\.anthropic\.com\b`)
)

const (
	// DefaultModelID is the trial default model.
	DefaultModelID = "accounts/fireworks/models/deepseek-v4-flash-0731"
	// TrialModelID is an alias for DefaultModelID.
	TrialModelID = "accounts/fireworks/models/deepseek-v4-flash-0731"
)

// ByokModelOptions configures model invocation behavior.
type ByokModelOptions struct {
	StructuredOutputs bool
}

// EnvProviderResolution is the single source of truth for self-hosted provider selection.
type EnvProviderResolution struct {
	Kind           string // "gemini_studio" | "gemini_vertex" | "claude_anthropic" | "claude_vertex" | "openai_compat" | "vertex_adc"
	Name           string // "google_ai_studio" | "google_vertex" | "anthropic" | "openai_compatible"
	APIKey         string
	BaseURL        string
	VertexLocation string
	Project        string
	IsClaude       bool
}

// EnvKindToProvider maps resolution kind to BYOKProvider enum.
var EnvKindToProvider = map[string]BYOKProvider{
	"gemini_studio":   ProviderGoogleGemini,
	"gemini_vertex":   ProviderGoogleVertex,
	"claude_vertex":   ProviderGoogleVertex,
	"claude_anthropic": ProviderAnthropic,
	"openai_compat":   ProviderOpenAICompatible,
	"vertex_adc":      ProviderGoogleVertex,
}

// EnvReasoningDescriptor describes the self-hosted model provider and name.
type EnvReasoningDescriptor struct {
	Provider BYOKProvider
	Model    string
}

// ManagedResolutionKind defines whether resolution produced a slot or an inline exception.
type ManagedResolutionKind string

const (
	ManagedKindSlot   ManagedResolutionKind = "slot"
	ManagedKindInline ManagedResolutionKind = "inline"
)

// InlineModelConfig captures configuration for inline exception models.
// Contains connection parameters: name, apiKey, baseURL, model, and structuredOutputs flag.
// ADC-specific fields (vertexLocation) are included for keyless execution.
type InlineModelConfig struct {
	Name                      string
	APIKey                    string
	BaseURL                   string
	Model                     string
	SupportsStructuredOutputs bool
	VertexLocation            string
}

// ManagedResolution encapsulates the resolved model slot or inline exception:
//   - kind: 'slot'   → a NormalizedModel carrying a plaintext env apiKey
//   - kind: 'inline' → config for an OpenAI-compatible or ADC model
type ManagedResolution struct {
	Kind   ManagedResolutionKind
	Slot   *NormalizedModel
	Inline *InlineModelConfig
}

// SlotFromResolution converts the resolution into a NormalizedModel for
// downstream provider dispatch. For 'slot' resolutions, returns the slot
// directly. For 'inline' resolutions, builds a NormalizedModel with the
// appropriate provider.
func (r *ManagedResolution) SlotFromResolution() *NormalizedModel {
	if r == nil {
		return nil
	}
	if r.Kind == ManagedKindSlot && r.Slot != nil {
		return r.Slot
	}
	if r.Kind == ManagedKindInline && r.Inline != nil {
		return &NormalizedModel{
			Provider:       ProviderOpenAICompatible,
			APIKey:         r.Inline.APIKey,
			BaseURL:        r.Inline.BaseURL,
			Model:          r.Inline.Model,
			VertexLocation: r.Inline.VertexLocation,
		}
	}
	return nil
}

// ADCModelBuilder is an extensible builder hook for ADC models.
// If it returns nil, ADC build fails and falls through to the managed/cloud default.
var ADCModelBuilder = func(model, project, location string) *InlineModelConfig {
	if project == "" {
		return nil
	}
	return &InlineModelConfig{
		Name:           "google_vertex",
		Model:          model,
		VertexLocation: location,
	}
}

// isProxyBaseURL determines if a forced base URL points to an OpenAI-compatible proxy.
func isProxyBaseURL(baseURL string) bool {
	if baseURL == "" {
		return false
	}
	return !proxyAnthropicPattern.MatchString(baseURL)
}

// vertexProjectFromEnv resolves the Google Cloud project ID for ambient ADC.
func vertexProjectFromEnv() string {
	project := strings.TrimSpace(os.Getenv("GOOGLE_CLOUD_PROJECT"))
	if project == "" {
		project = strings.TrimSpace(os.Getenv("GCLOUD_PROJECT"))
	}
	return project
}

// ResolveEnvProvider selects the provider and credentials from environment variables.
// Returns nil for "auto" (cloud mode) or when self-hosted has no usable key.
func ResolveEnvProvider() *EnvProviderResolution {
	envMode := os.Getenv("API_LLM_PROVIDER_MODEL")
	if envMode == "" {
		envMode = "auto"
	}
	if envMode == "auto" {
		return nil
	}

	isGemini := geminiModelPattern.MatchString(envMode)
	isClaude := claudeModelPattern.MatchString(envMode)
	openaiKey := os.Getenv("API_OPEN_AI_API_KEY")
	openaiBaseURL := os.Getenv("API_OPENAI_FORCE_BASE_URL")
	vertexKey := os.Getenv("API_VERTEX_AI_API_KEY")
	googleAiStudioKey := os.Getenv("API_GOOGLE_AI_API_KEY")
	if googleAiStudioKey == "" {
		googleAiStudioKey = os.Getenv("GOOGLE_GENERATIVE_AI_API_KEY")
	}
	viaProxy := isProxyBaseURL(openaiBaseURL)
	vertexLocation := os.Getenv("API_VERTEX_AI_LOCATION")

	if isGemini && !viaProxy {
		// 1. AI Studio key wins over Vertex key for gemini-*
		if googleAiStudioKey != "" {
			return &EnvProviderResolution{
				Kind:   "gemini_studio",
				Name:   "google_ai_studio",
				APIKey: googleAiStudioKey,
			}
		}
		// 2. Vertex SA JSON key
		if vertexKey != "" {
			return &EnvProviderResolution{
				Kind:           "gemini_vertex",
				Name:           "google_vertex",
				APIKey:         vertexKey,
				VertexLocation: vertexLocation,
			}
		}
		// 3. Keyless ambient ADC (GOOGLE_CLOUD_PROJECT).
		// An explicit OpenAI key WINS.
		geminiProject := vertexProjectFromEnv()
		if openaiKey == "" && geminiProject != "" {
			return &EnvProviderResolution{
				Kind:           "vertex_adc",
				Name:           "google_vertex",
				Project:        geminiProject,
				VertexLocation: vertexLocation,
				IsClaude:       false,
			}
		}
	}

	// Native Anthropic key takes precedence over Claude-on-Vertex.
	if isClaude && openaiKey != "" && !viaProxy {
		return &EnvProviderResolution{
			Kind:    "claude_anthropic",
			Name:    "anthropic",
			APIKey:  openaiKey,
			BaseURL: openaiBaseURL,
		}
	}

	if isClaude && vertexKey != "" && !viaProxy {
		return &EnvProviderResolution{
			Kind:           "claude_vertex",
			Name:           "google_vertex",
			APIKey:         vertexKey,
			VertexLocation: vertexLocation,
		}
	}

	// Keyless Claude-on-Vertex via ambient ADC.
	if isClaude && !viaProxy {
		claudeProject := vertexProjectFromEnv()
		if claudeProject != "" {
			return &EnvProviderResolution{
				Kind:           "vertex_adc",
				Name:           "google_vertex",
				Project:        claudeProject,
				VertexLocation: vertexLocation,
				IsClaude:       true,
			}
		}
	}

	// Any other self-hosted model with an OpenAI key or via proxy -> OpenAI-compatible.
	if openaiKey != "" {
		baseURL := openaiBaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		return &EnvProviderResolution{
			Kind:    "openai_compat",
			Name:    "openai_compatible",
			APIKey:  openaiKey,
			BaseURL: baseURL,
		}
	}

	return nil
}

// EnvManagedReasoningDescriptor returns provider and model for reasoning configuration.
func EnvManagedReasoningDescriptor() *EnvReasoningDescriptor {
	env := ResolveEnvProvider()
	if env == nil {
		return nil
	}
	model := os.Getenv("API_LLM_PROVIDER_MODEL")
	if model == "" {
		return nil
	}
	prov, ok := EnvKindToProvider[env.Kind]
	if !ok {
		return nil
	}
	return &EnvReasoningDescriptor{
		Provider: prov,
		Model:    model,
	}
}

// ResolveManagedSlot resolves the env/managed/self-host default to a managed slot or inline exception.
func ResolveManagedSlot(defaultModel string, options ByokModelOptions) *ManagedResolution {
	env := ResolveEnvProvider()
	if env != nil {
		envMode := os.Getenv("API_LLM_PROVIDER_MODEL")
		switch env.Kind {
		case "gemini_studio":
			return &ManagedResolution{
				Kind: ManagedKindSlot,
				Slot: &NormalizedModel{
					Provider: ProviderGoogleGemini,
					APIKey:   env.APIKey,
					Model:    envMode,
				},
			}
		case "gemini_vertex", "claude_vertex":
			return &ManagedResolution{
				Kind: ManagedKindSlot,
				Slot: &NormalizedModel{
					Provider:       ProviderGoogleVertex,
					APIKey:         env.APIKey,
					Model:          envMode,
					VertexLocation: env.VertexLocation,
				},
			}
		case "claude_anthropic":
			return &ManagedResolution{
				Kind: ManagedKindSlot,
				Slot: &NormalizedModel{
					Provider: ProviderAnthropic,
					APIKey:   env.APIKey,
					Model:    envMode,
					BaseURL:  env.BaseURL,
				},
			}
		case "openai_compat":
			return &ManagedResolution{
				Kind: ManagedKindInline,
				Inline: &InlineModelConfig{
					Name:                      "self-hosted",
					APIKey:                    env.APIKey,
					BaseURL:                   env.BaseURL,
					Model:                     envMode,
					SupportsStructuredOutputs: options.StructuredOutputs,
				},
			}
		case "vertex_adc":
			if ADCModelBuilder != nil {
				adcModel := ADCModelBuilder(envMode, env.Project, env.VertexLocation)
				if adcModel != nil {
					return &ManagedResolution{
						Kind:   ManagedKindInline,
						Inline: adcModel,
					}
				}
			}
			// Failed ADC build falls through to the managed/cloud default below
		}
	}

	// Fireworks AI — the managed default model for the trial / no-BYOK flow.
	if fireworksPattern.MatchString(defaultModel) {
		fireworksKey := os.Getenv("API_FIREWORKS_API_KEY")
		if fireworksKey == "" {
			fireworksKey = os.Getenv("FIREWORKS_API_KEY")
		}
		fireworksBaseURL := os.Getenv("API_FIREWORKS_BASE_URL")
		if fireworksBaseURL == "" {
			fireworksBaseURL = "https://api.fireworks.ai/inference/v1"
		}
		return &ManagedResolution{
			Kind: ManagedKindInline,
			Inline: &InlineModelConfig{
				Name:                      "fireworks",
				APIKey:                    fireworksKey,
				BaseURL:                   fireworksBaseURL,
				Model:                     defaultModel,
				SupportsStructuredOutputs: true,
			},
		}
	}

	// Legacy DeepSeek fallback
	if deepseekPattern.MatchString(defaultModel) {
		deepseekKey := os.Getenv("API_DEEPSEEK_API_KEY")
		if deepseekKey == "" {
			deepseekKey = os.Getenv("DEEPSEEK_API_KEY")
		}
		deepseekBaseURL := os.Getenv("API_DEEPSEEK_BASE_URL")
		if deepseekBaseURL == "" {
			deepseekBaseURL = "https://api.deepseek.com/v1"
		}
		return &ManagedResolution{
			Kind: ManagedKindInline,
			Inline: &InlineModelConfig{
				Name:    "deepseek",
				APIKey:  deepseekKey,
				BaseURL: deepseekBaseURL,
				Model:   defaultModel,
			},
		}
	}

	// Legacy Kimi fallback
	if kimiPattern.MatchString(defaultModel) {
		moonshotKey := os.Getenv("API_MOONSHOT_API_KEY")
		if moonshotKey == "" {
			moonshotKey = os.Getenv("MOONSHOT_API_KEY")
		}
		return &ManagedResolution{
			Kind: ManagedKindSlot,
			Slot: &NormalizedModel{
				Provider: ProviderMoonshot,
				APIKey:   moonshotKey,
				Model:    defaultModel,
			},
		}
	}

	// Cloud default (gemini)
	googleKey := os.Getenv("API_GOOGLE_AI_API_KEY")
	if googleKey == "" {
		googleKey = os.Getenv("GOOGLE_GENERATIVE_AI_API_KEY")
	}
	return &ManagedResolution{
		Kind: ManagedKindSlot,
		Slot: &NormalizedModel{
			Provider: ProviderGoogleGemini,
			APIKey:   googleKey,
			Model:    defaultModel,
		},
	}
}

// HasManagedModelKey returns true if credentials exist to back the managed model.
func HasManagedModelKey() bool {
	envMode := os.Getenv("API_LLM_PROVIDER_MODEL")
	if envMode == "" {
		envMode = "auto"
	}
	selfHosted := envMode != "auto"

	if selfHosted {
		adcBacksModel := (geminiModelPattern.MatchString(envMode) || claudeModelPattern.MatchString(envMode)) &&
			vertexProjectFromEnv() != ""

		return os.Getenv("API_OPEN_AI_API_KEY") != "" ||
			os.Getenv("API_GOOGLE_AI_API_KEY") != "" ||
			os.Getenv("GOOGLE_GENERATIVE_AI_API_KEY") != "" ||
			os.Getenv("API_VERTEX_AI_API_KEY") != "" ||
			adcBacksModel
	}

	fireworksKey := os.Getenv("API_FIREWORKS_API_KEY")
	if fireworksKey == "" {
		fireworksKey = os.Getenv("FIREWORKS_API_KEY")
	}
	return fireworksKey != ""
}

// GetModelName returns the human-readable model name for telemetry and logs.
func GetModelName(slot *NormalizedModel, defaultModelOverride string) string {
	if slot != nil {
		return fmt.Sprintf("%s:%s", slot.Provider, slot.Model)
	}

	env := ResolveEnvProvider()
	if env != nil {
		return fmt.Sprintf("%s:%s", env.Name, os.Getenv("API_LLM_PROVIDER_MODEL"))
	}

	if defaultModelOverride != "" {
		return defaultModelOverride
	}

	return DefaultModelID
}
