package llm

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// WorkspaceTokenBudget defines token quotas per organization.
type WorkspaceTokenBudget struct {
	WorkspaceID       uuid.UUID `json:"workspace_id"`
	MonthlyTokenLimit int64     `json:"monthly_token_limit"` // e.g. 10,000,000 tokens
	BurstLimitPerMin  int64     `json:"burst_limit_per_min"` // e.g. 500,000 tokens/min
	UsedThisMonth     int64     `json:"used_this_month"`
	UsedThisMinute    int64     `json:"used_this_minute"`
	LastMinuteWindow  time.Time `json:"last_minute_window"`
	LastMonthWindow   time.Time `json:"last_month_window"`
}

// TokenBudgetLimiter enforces consumption quotas across multi-tenant review workflows.
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
	l.budgets[b.WorkspaceID] = &b
}

// ConsumeTokens verifies and reserves tokens for an LLM review invocation.
func (l *TokenBudgetLimiter) ConsumeTokens(workspaceID uuid.UUID, requestedTokens int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	budget, exists := l.budgets[workspaceID]
	if !exists {
		return nil // No quota restriction configured (unlimited)
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

	// 3. Check Burst Cap
	if budget.BurstLimitPerMin > 0 && (budget.UsedThisMinute+requestedTokens) > budget.BurstLimitPerMin {
		return fmt.Errorf("workspace burst rate limit exceeded (%d/%d tokens/min); wait 60 seconds",
			budget.UsedThisMinute+requestedTokens, budget.BurstLimitPerMin)
	}

	// 4. Check Monthly Hard Cap
	if budget.MonthlyTokenLimit > 0 && (budget.UsedThisMonth+requestedTokens) > budget.MonthlyTokenLimit {
		return fmt.Errorf("workspace monthly token quota exhausted (%d/%d tokens); upgrade plan or configure BYOK",
			budget.UsedThisMonth+requestedTokens, budget.MonthlyTokenLimit)
	}

	// 5. Deduct tokens
	budget.UsedThisMinute += requestedTokens
	budget.UsedThisMonth += requestedTokens

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
