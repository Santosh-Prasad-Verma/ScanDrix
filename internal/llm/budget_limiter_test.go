package llm_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/llm"
)

func TestTokenBudgetLimiterBYOKAndPlatformQuota(t *testing.T) {
	limiter := llm.NewTokenBudgetLimiter()
	wsID := uuid.New()

	// 1. Configure workspace budget: 100k monthly limit, 10k burst limit/min, BYOK disabled
	limiter.SetBudget(llm.WorkspaceTokenBudget{
		WorkspaceID:       wsID,
		BYOKEnabled:       false,
		MonthlyTokenLimit: 100_000,
		BurstLimitPerMin:  10_000,
		LastMinuteWindow:  time.Now().UTC(),
		LastMonthWindow:   time.Now().UTC(),
	})

	// 2. Consume 5k tokens (within burst and monthly limits)
	if err := limiter.ConsumeTokens(wsID, 5000); err != nil {
		t.Fatalf("expected tokens to be consumed successfully, got error: %v", err)
	}

	snap, exists := limiter.GetQuotaSnapshot(wsID)
	if !exists || snap.UsedThisMonth != 5000 || snap.RemainingMonth != 95000 {
		t.Fatalf("unexpected quota snapshot: %+v", snap)
	}

	// 3. Exceed burst limit (request 6k tokens -> 5k + 6k = 11k > 10k burst cap)
	if err := limiter.ConsumeTokens(wsID, 6000); err == nil {
		t.Fatalf("expected burst limit error, got nil")
	}

	// 4. Test BYOK mode: Enable BYOK
	limiter.SetBYOK(wsID, true)
	snapBYOK, _ := limiter.GetQuotaSnapshot(wsID)
	if !snapBYOK.BYOKEnabled {
		t.Fatalf("expected BYOK to be enabled")
	}

	// 5. Consume tokens under BYOK (should bypass monthly limit check)
	limiter.SyncUsageFromDB(wsID, 120_000) // Already above 100k platform limit
	if err := limiter.ConsumeTokensWithBYOK(wsID, 1000, true); err != nil {
		t.Fatalf("expected BYOK token consumption to bypass platform monthly limit, got err: %v", err)
	}
}

func TestAssertPromptFitsContext(t *testing.T) {
	// 1. Small prompt fits in 8k context window (e.g. 100 chars ~ 25 tokens + 2048 reserve = 2073 < 8192)
	smallPrompt := "Analyze this small 10 line git diff"
	if err := llm.AssertPromptFitsContext(smallPrompt, 8192); err != nil {
		t.Fatalf("expected small prompt to fit context window, got %v", err)
	}

	// 2. Large prompt exceeds 4k context window (e.g. 16,000 chars ~ 4000 tokens + 2048 reserve = 6048 > 4096)
	largePrompt := string(make([]byte, 16_000))
	if err := llm.AssertPromptFitsContext(largePrompt, 4096); err == nil {
		t.Fatal("expected large prompt to exceed 4096 context window")
	}

	// 3. Zero or negative contextWindow skips check
	if err := llm.AssertPromptFitsContext(largePrompt, 0); err != nil {
		t.Fatalf("expected 0 context window to skip check, got %v", err)
	}
}
