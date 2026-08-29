package llm_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/llm"
)

func TestClassifyLLMError(t *testing.T) {
	// 1. Auth Invalid (401)
	err401 := llm.ClassifyLLMError(errors.New("Incorrect API key provided"), http.StatusUnauthorized)
	if err401.Category != llm.CategoryAuthInvalid || !err401.IsTerminal || err401.IsTransient {
		t.Fatalf("expected terminal CategoryAuthInvalid for 401, got %+v", err401)
	}

	// 2. Quota Exceeded (402)
	err402 := llm.ClassifyLLMError(errors.New("You exceeded your current quota"), http.StatusPaymentRequired)
	if err402.Category != llm.CategoryQuotaExceeded || !err402.IsTerminal {
		t.Fatalf("expected terminal CategoryQuotaExceeded for 402, got %+v", err402)
	}

	// 3. Rate Limit (429) -> Transient
	err429 := llm.ClassifyLLMError(errors.New("Rate limit reached for requests"), http.StatusTooManyRequests)
	if err429.Category != llm.CategoryRateLimit || err429.IsTerminal || !err429.IsTransient {
		t.Fatalf("expected transient CategoryRateLimit for 429, got %+v", err429)
	}

	// 4. Model Not Found (404)
	err404 := llm.ClassifyLLMError(errors.New("The model `claude-nonexistent` does not exist"), http.StatusNotFound)
	if err404.Category != llm.CategoryModelNotFound || !err404.IsTerminal {
		t.Fatalf("expected terminal CategoryModelNotFound for 404, got %+v", err404)
	}

	// 5. Model Access Denied (403 Model Garden)
	err403 := llm.ClassifyLLMError(errors.New("Permission denied: your project does not have access in Model Garden"), http.StatusForbidden)
	if err403.Category != llm.CategoryModelAccessDenied || !err403.IsTerminal {
		t.Fatalf("expected terminal CategoryModelAccessDenied for 403 Model Garden, got %+v", err403)
	}

	// 6. Context Overflow (Token length exceeded)
	errOverflow := llm.ClassifyLLMError(errors.New("maximum context length is 128000 tokens, but your prompt resulted in 142000 tokens"), http.StatusBadRequest)
	if errOverflow.Category != llm.CategoryContextOverflow || !errOverflow.IsTerminal {
		t.Fatalf("expected terminal CategoryContextOverflow for token limits, got %+v", errOverflow)
	}

	// 7. Transient 503 Outage
	err503 := llm.ClassifyLLMError(errors.New("Service Unavailable"), http.StatusServiceUnavailable)
	if err503.Category != llm.CategoryTransient || !err503.IsTransient {
		t.Fatalf("expected transient CategoryTransient for 503, got %+v", err503)
	}
}

func TestTokenBudgetLimiter(t *testing.T) {
	limiter := llm.NewTokenBudgetLimiter()
	wsID := uuid.New()

	limiter.SetBudget(llm.WorkspaceTokenBudget{
		WorkspaceID:       wsID,
		MonthlyTokenLimit: 100_000,
		BurstLimitPerMin:  10_000,
		LastMinuteWindow:  time.Now().UTC(),
		LastMonthWindow:   time.Now().UTC(),
	})

	// 1. Valid token consumption within burst limit
	err := limiter.ConsumeTokens(wsID, 5_000)
	if err != nil {
		t.Fatalf("unexpected error consuming 5,000 tokens: %v", err)
	}

	// 2. Burst limit violation (5k + 6k = 11k > 10k cap)
	errBurst := limiter.ConsumeTokens(wsID, 6_000)
	if errBurst == nil {
		t.Fatal("expected burst rate limit violation for exceeding 10,000 tokens/min")
	}

	// 3. Monthly cap violation
	limiter.SetBudget(llm.WorkspaceTokenBudget{
		WorkspaceID:       wsID,
		MonthlyTokenLimit: 50_000,
		BurstLimitPerMin:  100_000,
		UsedThisMonth:     48_000,
		LastMinuteWindow:  time.Now().UTC(),
		LastMonthWindow:   time.Now().UTC(),
	})

	errMonth := limiter.ConsumeTokens(wsID, 5_000)
	if errMonth == nil {
		t.Fatal("expected monthly quota exhausted error")
	}
}
