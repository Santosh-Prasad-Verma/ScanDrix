package settings

import (
	"time"

	"github.com/google/uuid"
)

// ByokResultCode classifies live connection probe outcomes.
type ByokResultCode string

const (
	ByokResultOK          ByokResultCode = "ok"
	ByokResultAuth        ByokResultCode = "auth"
	ByokResultNotFound    ByokResultCode = "not_found"
	ByokResultBadRequest  ByokResultCode = "bad_request"
	ByokResultPayment     ByokResultCode = "payment"
	ByokResultRateLimit   ByokResultCode = "rate_limit"
	ByokResultServerError ByokResultCode = "server_error"
	ByokResultNetwork     ByokResultCode = "network"
	ByokResultTimeout     ByokResultCode = "timeout"
)

// ByokTestInput carries credentials and configuration for a live probe.
type ByokTestInput struct {
	Provider    string   `json:"provider"` // openai, anthropic, gemini, novita, custom
	APIKey      string   `json:"api_key"`
	BaseURL     string   `json:"base_url,omitempty"`
	Model       string   `json:"model,omitempty"`
	Region      string   `json:"region,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
}

// ByokTestResult details the connectivity and latency outcome.
type ByokTestResult struct {
	Success         bool           `json:"success"`
	Code            ByokResultCode `json:"code"`
	Latency         time.Duration  `json:"latency"`
	LatencyMs       int64          `json:"latency_ms"`
	Message         string         `json:"message"`
	ProviderMessage string         `json:"provider_message,omitempty"`
	HTTPStatus      int            `json:"http_status,omitempty"`
}

// OverrideScope defines the hierarchical granularity of a model setting.
type OverrideScope string

const (
	ScopeGlobal     OverrideScope = "global"
	ScopeRepository OverrideScope = "repository"
	ScopeDirectory  OverrideScope = "directory"
)

// ModelOverride records a designated AI model for a scope.
type ModelOverride struct {
	Scope          OverrideScope `json:"scope"`
	WorkspaceID    uuid.UUID     `json:"workspace_id"`
	RepositoryID   *uuid.UUID    `json:"repository_id,omitempty"`
	RepositoryPath string        `json:"repository_path,omitempty"`
	DirectoryPath  string        `json:"directory_path,omitempty"`
	ModelID        string        `json:"model_id"`
}

// BotIgnoreConfig holds workspace rules for automated author suppression.
type BotIgnoreConfig struct {
	WorkspaceID     uuid.UUID `json:"workspace_id"`
	IgnoreKnownBots bool      `json:"ignore_known_bots"`
	CustomBotNames  []string  `json:"custom_bot_names"`
	IgnoredUserIDs  []string  `json:"ignored_user_ids"`
}
