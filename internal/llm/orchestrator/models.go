package orchestrator

import (
	"time"

	"github.com/google/uuid"
)

// LLMProviderType enumerates supported generative AI inference clouds.
type LLMProviderType string

const (
	ProviderAnthropic  LLMProviderType = "anthropic"
	ProviderOpenAI     LLMProviderType = "openai"
	ProviderGemini     LLMProviderType = "gemini"
	ProviderNovita     LLMProviderType = "novita"
	ProviderDeepSeek   LLMProviderType = "deepseek"
	ProviderBedrock    LLMProviderType = "bedrock"
	ProviderVertex     LLMProviderType = "vertex"
	ProviderOpenRouter LLMProviderType = "openrouter"
	ProviderOllama     LLMProviderType = "ollama"
	ProviderVLLM       LLMProviderType = "vllm"
	ProviderMoonshot   LLMProviderType = "moonshot"
	ProviderAlibaba    LLMProviderType = "alibaba"
	ProviderMiniMax    LLMProviderType = "minimax"
	ProviderTencent    LLMProviderType = "tencent"
	ProviderXAI        LLMProviderType = "xai"
	ProviderMistral    LLMProviderType = "mistral"
	ProviderZAI        LLMProviderType = "zai"        // Z.ai (Zhipu AI) — GLM model family
	ProviderMeta       LLMProviderType = "meta"       // Meta Llama direct API (llama.com)
	ProviderNvidia     LLMProviderType = "nvidia"     // NVIDIA NIM (integrate.api.nvidia.com)
	ProviderGroq       LLMProviderType = "groq"       // Groq hosted inference (fast, token-priced)
	ProviderTogether   LLMProviderType = "together"   // Together AI (Llama/open-weights hosting)
	ProviderFireworks  LLMProviderType = "fireworks"  // Fireworks AI
	ProviderDeepInfra  LLMProviderType = "deepinfra"  // DeepInfra (cheapest Llama hosting)
	ProviderCerebras   LLMProviderType = "cerebras"   // Cerebras (ultra-fast inference chips)
	ProviderSambaNova  LLMProviderType = "sambanova"  // SambaNova Cloud
	ProviderCohere     LLMProviderType = "cohere"     // Cohere (Command R family)
	ProviderPerplexity LLMProviderType = "perplexity" // Perplexity (sonar models)
)

// MessageRole defines the conversation turn actor.
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool" // for tool/function call result turns
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
	InputPerMillion  float64         `json:"input_per_million"`  // USD per 1M input tokens
	OutputPerMillion float64         `json:"output_per_million"` // USD per 1M output tokens
	SupportsThinking bool            `json:"supports_thinking"`
}

// InferenceRequest defines the input to the multi-provider orchestrator.
type InferenceRequest struct {
	WorkspaceID    uuid.UUID     `json:"workspace_id"`
	Messages       []ChatMessage `json:"messages"`
	PreferredModel string        `json:"preferred_model"`
	FallbackChain  []string      `json:"fallback_chain,omitempty"`
	MaxTokens      int           `json:"max_tokens"`
	Temperature    float64       `json:"temperature"`
	TopP           float64       `json:"top_p,omitempty"`
	Stream         bool          `json:"stream,omitempty"`

	// EnableReasoning activates extended thinking / chain-of-thought on
	// models that support it (SupportsThinking == true in ModelProfile).
	// ThinkingBudget caps how many tokens the model may spend on reasoning.
	// Zero means the provider default (typically 8 192 tokens).
	EnableReasoning bool `json:"enable_reasoning"`
	ThinkingBudget  int  `json:"thinking_budget,omitempty"`

	// TenantAPIKey carries a BYOK plaintext key resolved at request time.
	// NEVER serialize this to persistent storage or outbound logs —
	// json:"-" ensures it is stripped from any JSON marshal/unmarshal path.
	TenantAPIKey string `json:"-"`

	// PlanTier specifies the workspace license tier (COMMUNITY, TEAM, ENTERPRISE).
	// When empty, defaults to COMMUNITY (Free tier) with model gating.
	PlanTier string `json:"plan_tier,omitempty"`
}

// InferenceResponse aggregates model output and token accounting.
type InferenceResponse struct {
	// Content is the final assistant turn text.
	Content string `json:"content"`

	// ThinkingContent captures the raw reasoning trace emitted by thinking
	// models (Claude Sonnet 5, Gemini 3.x Flash, GPT-5.6, etc.) when
	// EnableReasoning is true. Empty for non-thinking models or if the
	// provider does not expose the chain-of-thought.
	ThinkingContent string `json:"thinking_content,omitempty"`

	ModelUsed        string          `json:"model_used"`
	ProviderUsed     LLMProviderType `json:"provider_used"`
	PromptTokens     int             `json:"prompt_tokens"`
	CompletionTokens int             `json:"completion_tokens"`
	TotalTokens      int             `json:"total_tokens"`
	CostUSD          float64         `json:"cost_usd"`

	// FinishReason is the normalized stop condition returned by the provider:
	// "stop", "length", "content_filter", "tool_calls", etc.
	FinishReason string `json:"finish_reason,omitempty"`

	// Latency is stored as int64 nanoseconds so JSON round-trips are exact.
	// Read it as time.Duration on the Go side; divide by 1e6 for ms in clients.
	Latency time.Duration `json:"latency_ns"`

	FallbackOccurred bool `json:"fallback_occurred"`
}

// BYOKCredential stores encrypted customer-supplied LLM API keys.
type BYOKCredential struct {
	WorkspaceID    uuid.UUID       `json:"workspace_id"`
	Provider       LLMProviderType `json:"provider"`
	EncryptedKey   string          `json:"encrypted_key"`
	Nonce          string          `json:"nonce"`
	KeyFingerprint string          `json:"key_fingerprint"`
	MaskedKey      string          `json:"masked_key"`

	// CreatedAt is set once on first encryption and preserved across rotations.
	// UpdatedAt is refreshed on every RotateKey call.
	// Both are required — byok.go's RotateKey references cred.CreatedAt and
	// EncryptKey sets newCred.CreatedAt, so omitting CreatedAt is a
	// guaranteed compile error.
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
