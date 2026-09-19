// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package byok_test

import (
	"context"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/llm/byok"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryption(t *testing.T) {
	rawKey := "sk-ant-api03-test-token-1234567890"
	ciphertext, err := byok.EncryptKey(rawKey)
	require.NoError(t, err)
	assert.NotEmpty(t, ciphertext)
	assert.NotEqual(t, rawKey, ciphertext)

	decrypted, err := byok.DecryptKey(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, rawKey, decrypted)
}

func TestResolveModelSlot(t *testing.T) {
	cfg := &byok.BYOKConfig{
		Version: 2,
		Credentials: []byok.BYOKCredential{
			{
				ID:       "cred-1",
				Provider: "anthropic",
				APIKey:   "enc-key-1",
				Settings: map[string]any{
					"baseURL": "https://api.anthropic.com",
				},
			},
		},
		Models: []byok.BYOKModelConfig{
			{
				ID:           "model-1",
				CredentialID: "cred-1",
				Model:        "claude-sonnet-4.5",
				RPM:          60,
				TPM:          100_000,
			},
		},
		Routing: byok.BYOKRouting{
			DefaultModelID: "model-1",
		},
	}

	slot := byok.ResolveModelSlot(cfg, "model-1")
	require.NotNil(t, slot)
	assert.Equal(t, byok.BYOKProvider("anthropic"), slot.Provider)
	assert.Equal(t, "claude-sonnet-4.5", slot.Model)
	assert.Equal(t, "https://api.anthropic.com", slot.BaseURL)
	assert.Equal(t, 60, slot.RPM)

	defaultSlot := byok.ResolveDefaultSlot(cfg)
	require.NotNil(t, defaultSlot)
	assert.Equal(t, "model-1", defaultSlot.BYOKModelID)
}

func TestLimiterAcquireAndRelease(t *testing.T) {
	limiter := byok.NewBYOKConcurrencyLimiter(2, 600, 50_000, "anthropic", "claude")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1st acquire
	err := limiter.Acquire(ctx, 1000)
	require.NoError(t, err)

	// 2nd acquire
	err = limiter.Acquire(ctx, 1000)
	require.NoError(t, err)

	// Release 1st
	limiter.Release(1000, 1200)

	// 3rd acquire passes
	err = limiter.Acquire(ctx, 500)
	require.NoError(t, err)

	limiter.Release(1000, 1000)
	limiter.Release(500, 500)
}

func TestLimiterCooldown(t *testing.T) {
	limiter := byok.NewBYOKConcurrencyLimiter(5, 0, 0, "anthropic", "claude")
	limiter.ArmCooldown(100 * time.Millisecond)

	start := time.Now()
	ctx := context.Background()
	err := limiter.Acquire(ctx, 0)
	require.NoError(t, err)
	elapsed := time.Since(start)
	assert.GreaterOrEqual(t, elapsed, 90*time.Millisecond)
	limiter.Release(0, 0)
}
