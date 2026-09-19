package llm

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// WorkspaceTokenBudget defines token quotas and BYOK configuration per organization.
type WorkspaceTokenBudget struct {
	WorkspaceID       uuid.UUID `json:"workspace_id"`
	BYOKEnabled       bool      `json:"byok_enabled"`
	MonthlyTokenLimit int64     `json:"monthly_token_limit"` // e.g. 10,000,000 tokens
	BurstLimitPerMin  int64     `json:"burst_limit_per_min"` // e.g. 500,000 tokens/min
	UsedThisMonth     int64     `json:"used_this_month"`
	UsedThisMinute    int64     `json:"used_this_minute"`
	LastMinuteWindow  time.Time `json:"last_minute_window"`
	LastMonthWindow   time.Time `json:"last_month_window"`
}

// QuotaSnapshot details real-time token headroom and billing period progress.
type QuotaSnapshot struct {
	WorkspaceID       uuid.UUID `json:"workspace_id"`
	BYOKEnabled       bool      `json:"byok_enabled"`
	MonthlyTokenLimit int64     `json:"monthly_token_limit"`
	UsedThisMonth     int64     `json:"used_this_month"`
	RemainingMonth    int64     `json:"remaining_month"`
	PercentUsed       float64   `json:"percent_used"`
	BurstLimitPerMin  int64     `json:"burst_limit_per_min"`
	UsedThisMinute    int64     `json:"used_this_minute"`
	IsExhausted       bool      `json:"is_exhausted"`
}

// TokenBudgetLimiter enforces consumption quotas and BYOK isolation across multi-tenant review workflows.
type TokenBudgetLimiter struct {
	mu      sync.Mutex
	budgets map[uuid.UUID]*WorkspaceTokenBudget
}

func NewTokenBudgetLimiter() *TokenBudgetLimiter {
	return &TokenBudgetLimiter{
		budgets: make(map[uuid.UUID]*WorkspaceTokenBudget),
	}
}

// SetBudget registers or updates token quotas for a workspace.
func (l *TokenBudgetLimiter) SetBudget(b WorkspaceTokenBudget) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now().UTC()
	if b.LastMinuteWindow.IsZero() {
		b.LastMinuteWindow = now
	}
	if b.LastMonthWindow.IsZero() {
		b.LastMonthWindow = now
	}
	l.budgets[b.WorkspaceID] = &b
}

// SetBYOK updates the Bring-Your-Own-Key state for a workspace.
func (l *TokenBudgetLimiter) SetBYOK(workspaceID uuid.UUID, enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if b, ok := l.budgets[workspaceID]; ok {
		b.BYOKEnabled = enabled
	} else {
		now := time.Now().UTC()
		l.budgets[workspaceID] = &WorkspaceTokenBudget{
			WorkspaceID:      workspaceID,
			BYOKEnabled:      enabled,
			LastMinuteWindow: now,
			LastMonthWindow:  now,
		}
	}
}

// SyncUsageFromDB synchronizes active in-memory counters with real PostgreSQL token usage records.
func (l *TokenBudgetLimiter) SyncUsageFromDB(workspaceID uuid.UUID, usedThisMonth int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if b, ok := l.budgets[workspaceID]; ok {
		b.UsedThisMonth = usedThisMonth
	}
}

// ConsumeTokens verifies and reserves tokens for an LLM review invocation.
func (l *TokenBudgetLimiter) ConsumeTokens(workspaceID uuid.UUID, requestedTokens int64) error {
	return l.ConsumeTokensWithBYOK(workspaceID, requestedTokens, false)
}

// ConsumeTokensWithBYOK verifies quota against platform limits vs BYOK custom credentials.
func (l *TokenBudgetLimiter) ConsumeTokensWithBYOK(workspaceID uuid.UUID, requestedTokens int64, isBYOKInvocation bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	budget, exists := l.budgets[workspaceID]
	if !exists {
		// Apply baseline Community tier quota (500,000 monthly tokens, 50,000 burst tokens/min)
		now := time.Now().UTC()
		budget = &WorkspaceTokenBudget{
			WorkspaceID:       workspaceID,
			MonthlyTokenLimit: 500000,
			BurstLimitPerMin:  50000,
			LastMinuteWindow:  now,
			LastMonthWindow:   now,
		}
		l.budgets[workspaceID] = budget
	}

	now := time.Now().UTC()

	// 1. Reset 1-minute burst window if elapsed
	if now.Sub(budget.LastMinuteWindow) >= time.Minute {
		budget.UsedThisMinute = 0
		budget.LastMinuteWindow = now
	}

	// 2. Reset monthly window if month rolled over
	if now.Month() != budget.LastMonthWindow.Month() || now.Year() != budget.LastMonthWindow.Year() {
		budget.UsedThisMonth = 0
		budget.LastMonthWindow = now
	}

	// 3. Check Burst Cap (Applies to both BYOK and Platform tokens to prevent runaway loops)
	if budget.BurstLimitPerMin > 0 && (budget.UsedThisMinute+requestedTokens) > budget.BurstLimitPerMin {
		return fmt.Errorf("workspace burst rate limit exceeded (%d/%d tokens/min); wait 60 seconds",
			budget.UsedThisMinute+requestedTokens, budget.BurstLimitPerMin)
	}

	// 4. Check Monthly Hard Cap
	// If tenant is using BYOK or this invocation is marked BYOK, bypass platform monthly allowance
	if !budget.BYOKEnabled && !isBYOKInvocation {
		if budget.MonthlyTokenLimit > 0 && (budget.UsedThisMonth+requestedTokens) > budget.MonthlyTokenLimit {
			return fmt.Errorf("workspace monthly token quota exhausted (%d/%d tokens); upgrade plan or configure BYOK",
				budget.UsedThisMonth+requestedTokens, budget.MonthlyTokenLimit)
		}
		budget.UsedThisMonth += requestedTokens
	}

	// 5. Track minute burst
	budget.UsedThisMinute += requestedTokens

	return nil
}

// GetUsage retrieves current consumption metrics for a workspace.
func (l *TokenBudgetLimiter) GetUsage(workspaceID uuid.UUID) (usedMonth, maxMonth, usedMin, maxMin int64, exists bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.budgets[workspaceID]
	if !ok {
		return 0, 0, 0, 0, false
	}
	return b.UsedThisMonth, b.MonthlyTokenLimit, b.UsedThisMinute, b.BurstLimitPerMin, true
}

// GetQuotaSnapshot returns a full real-time snapshot of the tenant's token quota and BYOK status.
func (l *TokenBudgetLimiter) GetQuotaSnapshot(workspaceID uuid.UUID) (QuotaSnapshot, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.budgets[workspaceID]
	if !ok {
		return QuotaSnapshot{
			WorkspaceID: workspaceID,
		}, false
	}

	remaining := b.MonthlyTokenLimit - b.UsedThisMonth
	if remaining < 0 {
		remaining = 0
	}
	pctUsed := 0.0
	if b.MonthlyTokenLimit > 0 {
		pctUsed = (float64(b.UsedThisMonth) / float64(b.MonthlyTokenLimit)) * 100.0
	}

	isExhausted := !b.BYOKEnabled && b.MonthlyTokenLimit > 0 && b.UsedThisMonth >= b.MonthlyTokenLimit

	return QuotaSnapshot{
		WorkspaceID:       b.WorkspaceID,
		BYOKEnabled:       b.BYOKEnabled,
		MonthlyTokenLimit: b.MonthlyTokenLimit,
		UsedThisMonth:     b.UsedThisMonth,
		RemainingMonth:    remaining,
		PercentUsed:       pctUsed,
		BurstLimitPerMin:  b.BurstLimitPerMin,
		UsedThisMinute:    b.UsedThisMinute,
		IsExhausted:       isExhausted,
	}, true
}

const (
	PreflightCharsPerToken        = 4
	PreflightMinOutputReserve     = 2048
	PreflightOutputReservePercent = 0.15
)

// AssertPromptFitsContext verifies that the input prompt does not exceed the model context window,
// reserving at least 15% (min 2048 tokens) for model reasoning and structured JSON output.
func AssertPromptFitsContext(prompt string, contextWindowTokens int) error {
	if contextWindowTokens <= 0 {
		return nil
	}

	promptChars := len(prompt)
	estimatedPromptTokens := (promptChars + PreflightCharsPerToken - 1) / PreflightCharsPerToken

	outputReserve := int(float64(contextWindowTokens) * PreflightOutputReservePercent)
	if outputReserve < PreflightMinOutputReserve {
		outputReserve = PreflightMinOutputReserve
	}

	if estimatedPromptTokens+outputReserve > contextWindowTokens {
		return fmt.Errorf("prompt exceeds context window: estimated %d prompt tokens + %d output reserve > %d max context tokens",
			estimatedPromptTokens, outputReserve, contextWindowTokens)
	}

	return nil
}
