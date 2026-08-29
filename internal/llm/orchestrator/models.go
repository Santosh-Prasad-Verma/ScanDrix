package orchestrator

import (
	"time"

	"github.com/google/uuid"
)

// LLMProviderType enumerates supported generative AI inference clouds.
type LLMProviderType string

const (
	ProviderAnthropic LLMProviderType = "anthropic"
	ProviderOpenAI    LLMProviderType = "openai"
	ProviderGemini    LLMProviderType = "gemini"
	ProviderNovita    LLMProviderType = "novita"
)

// MessageRole defines the conversation turn actor.
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
)

// ChatMessage represents a single prompt turn.
type ChatMessage struct {
	Role    MessageRole `json:"role"`
	Content string      `json:"content"`
}

// ModelProfile defines the specifications and pricing for an LLM model.
type ModelProfile struct {
	ModelID          string          `json:"model_id"`
	Provider         LLMProviderType `json:"provider"`
	ContextWindow    int             `json:"context_window"`
	InputPerMillion  float64         `json:"input_per_million"`  // USD per 1M tokens
	OutputPerMillion float64         `json:"output_per_million"` // USD per 1M tokens
	SupportsThinking bool            `json:"supports_thinking"`
}

// InferenceRequest defines the input to the multi-provider orchestrator.
type InferenceRequest struct {
	WorkspaceID     uuid.UUID     `json:"workspace_id"`
	Messages        []ChatMessage `json:"messages"`
	PreferredModel  string        `json:"preferred_model"`
	FallbackChain   []string      `json:"fallback_chain,omitempty"`
	MaxTokens       int           `json:"max_tokens"`
	Temperature     float64       `json:"temperature"`
	TenantAPIKey    string        `json:"tenant_api_key,omitempty"` // For BYOK
	EnableReasoning bool          `json:"enable_reasoning"`
}

// InferenceResponse aggregates model output and token accounting.
type InferenceResponse struct {
	Content          string          `json:"content"`
	ModelUsed        string          `json:"model_used"`
	ProviderUsed     LLMProviderType `json:"provider_used"`
	PromptTokens     int             `json:"prompt_tokens"`
	CompletionTokens int             `json:"completion_tokens"`
	TotalTokens      int             `json:"total_tokens"`
	CostUSD          float64         `json:"cost_usd"`
	Latency          time.Duration   `json:"latency"`
	FallbackOccurred bool            `json:"fallback_occurred"`
}

// BYOKCredential stores encrypted customer-supplied LLM API keys.
type BYOKCredential struct {
	WorkspaceID    uuid.UUID       `json:"workspace_id"`
	Provider       LLMProviderType `json:"provider"`
	EncryptedKey   string          `json:"encrypted_key"`
	Nonce          string          `json:"nonce"`
	KeyFingerprint string          `json:"key_fingerprint"`
	MaskedKey      string          `json:"masked_key"`
	UpdatedAt      time.Time       `json:"updated_at"`
}
