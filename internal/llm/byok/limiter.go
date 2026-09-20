// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package byok

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// BYOKConcurrencyLimiter governs concurrency, requests-per-minute (RPM),
// tokens-per-minute (TPM) reservoir, and 429 rate-limit cooldowns.
type BYOKConcurrencyLimiter struct {
	mu   sync.Mutex
	cond *sync.Cond

	concurrency int
	activeCount int

	// RPM rate gate
	rpm         int
	minInterval time.Duration
	lastStartAt time.Time

	// TPM token reservoir
	tpmCapacity       int
	reservoir         int
	reservoirRefillAt time.Time

	// Cooldown gate (armed on 429 RATE_LIMIT)
	cooldownUntil time.Time

	provider string
	model    string
}

// NewBYOKConcurrencyLimiter initializes a new multi-gate limiter.
func NewBYOKConcurrencyLimiter(concurrency, rpm, tpm int, provider, model string) *BYOKConcurrencyLimiter {
	l := &BYOKConcurrencyLimiter{
		concurrency:       concurrency,
		rpm:               rpm,
		tpmCapacity:       tpm,
		reservoir:         tpm,
		reservoirRefillAt: time.Now(),
		provider:          provider,
		model:             model,
	}
	l.cond = sync.NewCond(&l.mu)

	if rpm > 0 {
		l.minInterval = time.Minute / time.Duration(rpm)
	}

	return l
}

// ArmCooldown sets the cooldown gate duration upon receiving a provider 429.
func (l *BYOKConcurrencyLimiter) ArmCooldown(duration time.Duration) {
	if duration <= 0 {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	until := time.Now().Add(duration)
	if until.After(l.cooldownUntil) {
		l.cooldownUntil = until
		l.cond.Broadcast()
	}
}

// Acquire waits until concurrency, RPM, TPM, and cooldown constraints are met.
func (l *BYOKConcurrencyLimiter) Acquire(ctx context.Context, estimatedTokens int) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		now := time.Now()

		// 1. Check Cooldown
		if now.Before(l.cooldownUntil) {
			waitDuration := l.cooldownUntil.Sub(now)
			l.mu.Unlock()
			select {
			case <-ctx.Done():
				l.mu.Lock()
				return ctx.Err()
			case <-time.After(waitDuration):
				l.mu.Lock()
				continue
			}
		}

		// 2. Refill TPM reservoir linearly
		if l.tpmCapacity > 0 {
			elapsed := now.Sub(l.reservoirRefillAt)
			if elapsed > 0 {
				refillTokens := int(float64(l.tpmCapacity) * (float64(elapsed) / float64(time.Minute)))
				if refillTokens > 0 {
					l.reservoir += refillTokens
					if l.reservoir > l.tpmCapacity {
						l.reservoir = l.tpmCapacity
					}
					l.reservoirRefillAt = now
				}
			}

			// If reservoir is too low for this request, wait until refilled
			if estimatedTokens > 0 && l.reservoir < estimatedTokens {
				needed := estimatedTokens - l.reservoir
				waitSecs := float64(needed) / (float64(l.tpmCapacity) / 60.0)
				waitDuration := time.Duration(waitSecs * float64(time.Second))
				if waitDuration < 100*time.Millisecond {
					waitDuration = 100 * time.Millisecond
				}
				l.mu.Unlock()
				select {
				case <-ctx.Done():
					l.mu.Lock()
					return ctx.Err()
				case <-time.After(waitDuration):
					l.mu.Lock()
					continue
				}
			}
		}

		// 3. Check RPM min-interval
		if l.minInterval > 0 && !l.lastStartAt.IsZero() {
			elapsedSinceLast := now.Sub(l.lastStartAt)
			if elapsedSinceLast < l.minInterval {
				waitDuration := l.minInterval - elapsedSinceLast
				l.mu.Unlock()
				select {
				case <-ctx.Done():
					l.mu.Lock()
					return ctx.Err()
				case <-time.After(waitDuration):
					l.mu.Lock()
					continue
				}
			}
		}

		// 4. Check Concurrency
		if l.concurrency > 0 && l.activeCount >= l.concurrency {
			// Wait for a slot to free up
			l.cond.Wait()
			continue
		}

		// Admission granted!
		l.activeCount++
		l.lastStartAt = time.Now()
		if l.tpmCapacity > 0 && estimatedTokens > 0 {
			l.reservoir -= estimatedTokens
		}
		return nil
	}
}

// Release frees an active concurrency slot and reconciles estimated vs actual token usage.
func (l *BYOKConcurrencyLimiter) Release(estimatedTokens, actualTokens int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.activeCount > 0 {
		l.activeCount--
	}

	// Reconcile TPM token reservoir
	if l.tpmCapacity > 0 && estimatedTokens > 0 && actualTokens > 0 {
		diff := actualTokens - estimatedTokens
		if diff > 0 {
			// Overshoot: debit extra tokens
			l.reservoir -= diff
		} else if diff < 0 {
			// Undershoot: return unused tokens
			l.reservoir += (-diff)
			if l.reservoir > l.tpmCapacity {
				l.reservoir = l.tpmCapacity
			}
		}
	}

	l.cond.Broadcast()
}

// ═══════════════════════════════════════════════════════════════
// GLOBAL LIMITER REGISTRY
// ═══════════════════════════════════════════════════════════════

var (
	registryMu sync.RWMutex
	limiters   = make(map[string]*BYOKConcurrencyLimiter)
)

func getLimiterKey(slot *NormalizedModel) string {
	if slot == nil {
		return "managed-default"
	}
	key := string(slot.Provider) + ":" + slot.Model
	if slot.CredentialID != "" {
		key += ":" + slot.CredentialID
	}
	return key
}

// GetLimiterForSlot returns or instantiates the BYOKConcurrencyLimiter for a slot.
func GetLimiterForSlot(slot *NormalizedModel) *BYOKConcurrencyLimiter {
	key := getLimiterKey(slot)

	registryMu.RLock()
	limiter, exists := limiters[key]
	registryMu.RUnlock()
	if exists {
		return limiter
	}

	registryMu.Lock()
	defer registryMu.Unlock()
	if limiter, exists = limiters[key]; exists {
		return limiter
	}

	concurrency := 0
	rpm := 0
	tpm := 0
	provider := "managed"
	model := "default"

	if slot != nil {
		concurrency = slot.MaxConcurrentRequests
		rpm = slot.RPM
		tpm = slot.TPM
		provider = string(slot.Provider)
		model = slot.Model
	}

	limiter = NewBYOKConcurrencyLimiter(concurrency, rpm, tpm, provider, model)
	limiters[key] = limiter
	return limiter
}

// RunWithBYOKLimiter wraps an execution with slot rate, token, and concurrency limiting.
func RunWithBYOKLimiter[T any](ctx context.Context, slot *NormalizedModel, estimatedTokens int, fn func() (T, int, error)) (T, error) {
	limiter := GetLimiterForSlot(slot)
	if err := limiter.Acquire(ctx, estimatedTokens); err != nil {
		var zero T
		return zero, fmt.Errorf("byok limiter acquisition error: %w", err)
	}

	result, actualTokens, err := fn()

	// Only arm cooldown on rate-limit errors, not auth/model/timeout errors
	if err != nil && slot != nil && slot.CooldownMs > 0 {
		errLower := strings.ToLower(err.Error())
		if strings.Contains(errLower, "429") || strings.Contains(errLower, "rate limit") || strings.Contains(errLower, "too many requests") {
			limiter.ArmCooldown(time.Duration(slot.CooldownMs) * time.Millisecond)
		}
	}

	limiter.Release(estimatedTokens, actualTokens)
	return result, err
}
