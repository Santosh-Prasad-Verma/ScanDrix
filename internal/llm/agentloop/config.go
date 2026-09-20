// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package agentloop

import (
	"os"
	"strconv"
	"time"
)

const (
	// DefaultMaxSteps is the fallback hard ceiling on agent loop steps.
	DefaultMaxSteps = 10
	// DefaultMaxRetries is the fallback per-step retry count for transient provider failures.
	DefaultMaxRetries = 3
	// DefaultRetryBaseDelay is the initial backoff delay between step retries.
	DefaultRetryBaseDelay = 500 * time.Millisecond
	// DefaultPromptCacheEnabled determines if inline prompt caching markers are stamped.
	DefaultPromptCacheEnabled = true
)

// AgentLoopConfig encapsulates all runtime parameters for agent loop execution.
// Every parameter is backed by environment variables with zero hardcoding.
type AgentLoopConfig struct {
	MaxSteps           int           `json:"max_steps"`
	MaxRetries         int           `json:"max_retries"`
	RetryBaseDelay     time.Duration `json:"retry_base_delay"`
	PromptCacheEnabled bool          `json:"prompt_cache_enabled"`
}

// LoadAgentLoopConfig reads runtime options from environment variables or returns defaults.
func LoadAgentLoopConfig() AgentLoopConfig {
	cfg := AgentLoopConfig{
		MaxSteps:           DefaultMaxSteps,
		MaxRetries:         DefaultMaxRetries,
		RetryBaseDelay:     DefaultRetryBaseDelay,
		PromptCacheEnabled: DefaultPromptCacheEnabled,
	}

	if val := os.Getenv("SCANDRIX_AGENT_MAX_STEPS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			cfg.MaxSteps = parsed
		}
	}

	if val := os.Getenv("SCANDRIX_AGENT_STEP_MAX_RETRIES"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed >= 0 {
			cfg.MaxRetries = parsed
		}
	}

	if val := os.Getenv("SCANDRIX_AGENT_RETRY_BASE_MS"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			cfg.RetryBaseDelay = time.Duration(parsed) * time.Millisecond
		}
	}

	if val := os.Getenv("SCANDRIX_PROMPT_CACHE_ENABLED"); val != "" {
		if parsed, err := strconv.ParseBool(val); err == nil {
			cfg.PromptCacheEnabled = parsed
		}
	}

	return cfg
}
