// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package byok

// BYOKProvider identifies supported cloud inference platforms.
type BYOKProvider string

const (
	ProviderOpenAI              BYOKProvider = "openai"
	ProviderAnthropic           BYOKProvider = "anthropic"
	ProviderGoogleGemini        BYOKProvider = "google_gemini"
	ProviderGoogleVertex        BYOKProvider = "google_vertex"
	ProviderAmazonBedrock       BYOKProvider = "amazon_bedrock"
	ProviderOpenAICompatible    BYOKProvider = "openai_compatible"
	ProviderAnthropicCompatible BYOKProvider = "anthropic_compatible"
	ProviderOpenRouter          BYOKProvider = "open_router"
	ProviderNovita              BYOKProvider = "novita"
	ProviderMoonshot            BYOKProvider = "moonshot"
	ProviderZAI                 BYOKProvider = "zai"
	ProviderAzure               BYOKProvider = "azure"
)

// LlmTask defines specific review workloads routed through the routing strategy.
type LlmTask string

const (
	TaskCodeReview         LlmTask = "codeReview"
	TaskDrixyRulesReview   LlmTask = "drixyRulesReview"
	TaskRuleGeneration     LlmTask = "ruleGeneration"
	TaskBusinessValidation LlmTask = "businessValidation"
	TaskPRSummary          LlmTask = "prSummary"
	TaskConversation       LlmTask = "conversation"
)

// BYOKCredential stores encrypted customer-supplied provider credentials.
type BYOKCredential struct {
	ID        string         `json:"id"`
	Provider  string         `json:"provider"`
	APIKey    string         `json:"apiKey,omitempty"` // AES-256-GCM ciphertext
	Settings  map[string]any `json:"settings,omitempty"`
	IsManaged bool           `json:"managed,omitempty"`
}

// BYOKModelConfig connects a model configuration to a specific credential.
type BYOKModelConfig struct {
	ID                      string   `json:"id"`
	CredentialID            string   `json:"credentialId"`
	Model                   string   `json:"model"`
	ReasoningEffort         string   `json:"reasoningEffort,omitempty"` // "none"|"low"|"medium"|"high"
	ReasoningConfigOverride string   `json:"reasoningConfigOverride,omitempty"`
	Temperature             *float64 `json:"temperature,omitempty"`
	MaxInputTokens          int      `json:"maxInputTokens,omitempty"`
	MaxOutputTokens         int      `json:"maxOutputTokens,omitempty"`
	MaxConcurrentRequests   int      `json:"maxConcurrentRequests,omitempty"`
	RPM                     int      `json:"rpm,omitempty"`
	TPM                     int      `json:"tpm,omitempty"`
	CooldownMs              int      `json:"cooldownMs,omitempty"`
}

// BYOKRouting defines task overrides and default/fallback models.
type BYOKRouting struct {
	Mode            string             `json:"mode,omitempty"` // "manual" | "auto"
	TaskOverrides   map[LlmTask]string `json:"taskOverrides,omitempty"`
	DefaultModelID  string             `json:"defaultModelId,omitempty"`
	FallbackModelID string             `json:"fallbackModelId,omitempty"`
}

// BYOKConfig is the persistent multi-tenant BYOK configuration document.
type BYOKConfig struct {
	Version     int               `json:"version"` // 2
	Credentials []BYOKCredential  `json:"credentials"`
	Models      []BYOKModelConfig `json:"models"`
	Routing     BYOKRouting       `json:"routing,omitempty"`
}

// NormalizedModel is the flat runtime slot consumed by model clients and limiters.
type NormalizedModel struct {
	Provider                BYOKProvider
	APIKey                  string // Ciphertext; decrypted in execution scope
	Model                   string
	BYOKModelID             string
	CredentialID            string
	BaseURL                 string
	VertexLocation          string
	AWSBearerToken          string
	AWSAccessKeyID          string
	AWSSecretAccessKey      string
	AWSRegion               string
	AWSSessionToken         string
	OpenRouterProviderOrder []string
	OpenRouterAllowFallback *bool
	ReasoningEffort         string
	ReasoningConfigOverride string
	Temperature             *float64
	MaxInputTokens          int
	MaxOutputTokens         int
	MaxConcurrentRequests   int
	RPM                     int
	TPM                     int
	CooldownMs              int

	// Routing provenance
	Route        LlmTask
	UsedFallback bool
	// Fallback is the runtime failover target. Typed as *FallbackSlot (not
	// *NormalizedModel) to enforce a single-hop cascade at the type level.
	Fallback *FallbackSlot
}

// FallbackSlot is a NormalizedModel that cannot itself carry a nested fallback.
// This enforces a single-hop cascade at the type level — the compiler prevents
// recursive nesting.
type FallbackSlot struct {
	Provider                BYOKProvider
	APIKey                  string
	Model                   string
	BYOKModelID             string
	CredentialID            string
	BaseURL                 string
	VertexLocation          string
	AWSBearerToken          string
	AWSAccessKeyID          string
	AWSSecretAccessKey      string
	AWSRegion               string
	AWSSessionToken         string
	OpenRouterProviderOrder []string
	OpenRouterAllowFallback *bool
	ReasoningEffort         string
	ReasoningConfigOverride string
	Temperature             *float64
	MaxInputTokens          int
	MaxOutputTokens         int
	MaxConcurrentRequests   int
	RPM                     int
	TPM                     int
	CooldownMs              int
	Route                   LlmTask
	UsedFallback            bool
}

// ToFallbackSlot converts a NormalizedModel to a FallbackSlot, stripping the
// Fallback field to enforce the single-hop invariant.
func (m *NormalizedModel) ToFallbackSlot() *FallbackSlot {
	if m == nil {
		return nil
	}
	return &FallbackSlot{
		Provider:                m.Provider,
		APIKey:                  m.APIKey,
		Model:                   m.Model,
		BYOKModelID:             m.BYOKModelID,
		CredentialID:            m.CredentialID,
		BaseURL:                 m.BaseURL,
		VertexLocation:          m.VertexLocation,
		AWSBearerToken:          m.AWSBearerToken,
		AWSAccessKeyID:          m.AWSAccessKeyID,
		AWSSecretAccessKey:      m.AWSSecretAccessKey,
		AWSRegion:               m.AWSRegion,
		AWSSessionToken:         m.AWSSessionToken,
		OpenRouterProviderOrder: m.OpenRouterProviderOrder,
		OpenRouterAllowFallback: m.OpenRouterAllowFallback,
		ReasoningEffort:         m.ReasoningEffort,
		ReasoningConfigOverride: m.ReasoningConfigOverride,
		Temperature:             m.Temperature,
		MaxInputTokens:          m.MaxInputTokens,
		MaxOutputTokens:         m.MaxOutputTokens,
		MaxConcurrentRequests:   m.MaxConcurrentRequests,
		RPM:                     m.RPM,
		TPM:                     m.TPM,
		CooldownMs:              m.CooldownMs,
		Route:                   m.Route,
		UsedFallback:            m.UsedFallback,
	}
}

// FallbackToSlot converts a FallbackSlot back to a NormalizedModel (with nil Fallback)
// for use in provider dispatch where a full NormalizedModel is expected.
func (f *FallbackSlot) FallbackToSlot() *NormalizedModel {
	if f == nil {
		return nil
	}
	return &NormalizedModel{
		Provider:                f.Provider,
		APIKey:                  f.APIKey,
		Model:                   f.Model,
		BYOKModelID:             f.BYOKModelID,
		CredentialID:            f.CredentialID,
		BaseURL:                 f.BaseURL,
		VertexLocation:          f.VertexLocation,
		AWSBearerToken:          f.AWSBearerToken,
		AWSAccessKeyID:          f.AWSAccessKeyID,
		AWSSecretAccessKey:      f.AWSSecretAccessKey,
		AWSRegion:               f.AWSRegion,
		AWSSessionToken:         f.AWSSessionToken,
		OpenRouterProviderOrder: f.OpenRouterProviderOrder,
		OpenRouterAllowFallback: f.OpenRouterAllowFallback,
		ReasoningEffort:         f.ReasoningEffort,
		ReasoningConfigOverride: f.ReasoningConfigOverride,
		Temperature:             f.Temperature,
		MaxInputTokens:          f.MaxInputTokens,
		MaxOutputTokens:         f.MaxOutputTokens,
		MaxConcurrentRequests:   f.MaxConcurrentRequests,
		RPM:                     f.RPM,
		TPM:                     f.TPM,
		CooldownMs:              f.CooldownMs,
		Route:                   f.Route,
		UsedFallback:            f.UsedFallback,
	}
}

// ResolveModelSlot materializes one model configuration into a NormalizedModel slot.
func ResolveModelSlot(config *BYOKConfig, modelID string) *NormalizedModel {
	if config == nil || modelID == "" {
		return nil
	}

	var targetModel *BYOKModelConfig
	for i := range config.Models {
		if config.Models[i].ID == modelID {
			targetModel = &config.Models[i]
			break
		}
	}
	if targetModel == nil {
		return nil
	}

	var cred *BYOKCredential
	for i := range config.Credentials {
		if config.Credentials[i].ID == targetModel.CredentialID {
			cred = &config.Credentials[i]
			break
		}
	}
	if cred == nil || cred.IsManaged {
		return nil
	}

	slot := &NormalizedModel{
		Provider:                BYOKProvider(cred.Provider),
		APIKey:                  cred.APIKey,
		Model:                   targetModel.Model,
		BYOKModelID:             targetModel.ID,
		CredentialID:            cred.ID,
		ReasoningEffort:         targetModel.ReasoningEffort,
		ReasoningConfigOverride: targetModel.ReasoningConfigOverride,
		Temperature:             targetModel.Temperature,
		MaxInputTokens:          targetModel.MaxInputTokens,
		MaxOutputTokens:         targetModel.MaxOutputTokens,
		MaxConcurrentRequests:   targetModel.MaxConcurrentRequests,
		RPM:                     targetModel.RPM,
		TPM:                     targetModel.TPM,
		CooldownMs:              targetModel.CooldownMs,
	}

	if cred.Settings != nil {
		if u, ok := cred.Settings["baseURL"].(string); ok {
			slot.BaseURL = u
		}
		if loc, ok := cred.Settings["vertexLocation"].(string); ok {
			slot.VertexLocation = loc
		}
		if t, ok := cred.Settings["awsBearerToken"].(string); ok {
			slot.AWSBearerToken = t
		}
		if id, ok := cred.Settings["awsAccessKeyId"].(string); ok {
			slot.AWSAccessKeyID = id
		}
		if sec, ok := cred.Settings["awsSecretAccessKey"].(string); ok {
			slot.AWSSecretAccessKey = sec
		}
		if reg, ok := cred.Settings["awsRegion"].(string); ok {
			slot.AWSRegion = reg
		}
		if ses, ok := cred.Settings["awsSessionToken"].(string); ok {
			slot.AWSSessionToken = ses
		}
		if orders, ok := cred.Settings["openrouterProviderOrder"].([]any); ok {
			for _, ord := range orders {
				if s, ok := ord.(string); ok && s != "" {
					slot.OpenRouterProviderOrder = append(slot.OpenRouterProviderOrder, s)
				}
			}
		}
		if fb, ok := cred.Settings["openrouterAllowFallbacks"].(bool); ok {
			slot.OpenRouterAllowFallback = &fb
		}
	}

	return slot
}

// ResolveDefaultSlot returns the default configured slot for an organization.
func ResolveDefaultSlot(config *BYOKConfig) *NormalizedModel {
	if config == nil {
		return nil
	}
	defaultID := config.Routing.DefaultModelID
	if defaultID == "" && len(config.Models) > 0 {
		defaultID = config.Models[0].ID
	}
	return ResolveModelSlot(config, defaultID)
}
