// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package workflow_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/review/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestByokConcurrencyGate_BasicAcquireAndRelease(t *testing.T) {
	gate := workflow.NewByokConcurrencyGate(3)
	ctx := context.Background()

	orgID := "org-alpha"
	provider := "anthropic"
	model := "claude-3-7-sonnet"

	release, err := gate.AcquireSlot(ctx, orgID, provider, model)
	require.NoError(t, err)
	require.NotNil(t, release)

	// Release slot
	release()
}

func TestByokConcurrencyGate_ConcurrencyLimitAndBlocking(t *testing.T) {
	limit := 2
	gate := workflow.NewByokConcurrencyGate(limit)
	ctx := context.Background()

	orgID := "org-beta"
	provider := "openai"
	model := "gpt-4o"

	rel1, err := gate.AcquireSlot(ctx, orgID, provider, model)
	require.NoError(t, err)
	defer rel1()

	rel2, err := gate.AcquireSlot(ctx, orgID, provider, model)
	require.NoError(t, err)

	// Third acquire should block until one is released
	var acquiredThird atomic.Bool
	ctxTimeout, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()

	go func() {
		rel3, err := gate.AcquireSlot(ctx, orgID, provider, model)
		if err == nil && rel3 != nil {
			acquiredThird.Store(true)
			rel3()
		}
	}()

	// Verify it has not acquired immediately
	time.Sleep(50 * time.Millisecond)
	assert.False(t, acquiredThird.Load())

	// Release rel2
	rel2()

	// Wait and verify third acquire unblocks
	assert.Eventually(t, func() bool {
		return acquiredThird.Load()
	}, 300*time.Millisecond, 20*time.Millisecond)

	_ = ctxTimeout
}

func TestByokConcurrencyGate_CustomLimitPerScope(t *testing.T) {
	gate := workflow.NewByokConcurrencyGate(10) // default 10
	ctx := context.Background()

	orgA := "org-tenant-a"
	orgB := "org-tenant-b"
	provider := "anthropic"
	model := "claude-3-5-sonnet"

	// Restrict orgA to only 1 slot
	gate.SetLimit(orgA, provider, model, 1)

	relA1, err := gate.AcquireSlot(ctx, orgA, provider, model)
	require.NoError(t, err)
	defer relA1()

	// orgA second slot should time out
	ctxTimeoutA, cancelA := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancelA()
	_, errA2 := gate.AcquireSlot(ctxTimeoutA, orgA, provider, model)
	assert.ErrorIs(t, errA2, context.DeadlineExceeded)

	// orgB should have independent limit (uses default 10)
	relB1, errB1 := gate.AcquireSlot(ctx, orgB, provider, model)
	require.NoError(t, errB1)
	defer relB1()

	relB2, errB2 := gate.AcquireSlot(ctx, orgB, provider, model)
	require.NoError(t, errB2)
	defer relB2()
}

func TestByokConcurrencyGate_ReleaseIdempotence(t *testing.T) {
	gate := workflow.NewByokConcurrencyGate(1)
	ctx := context.Background()

	orgID := "org-gamma"
	provider := "google"
	model := "gemini-2.5-pro"

	rel, err := gate.AcquireSlot(ctx, orgID, provider, model)
	require.NoError(t, err)

	// Multiple releases must not panic or corrupt counter
	rel()
	rel()
	rel()

	// Should be able to acquire again immediately
	relNew, errNew := gate.AcquireSlot(ctx, orgID, provider, model)
	require.NoError(t, errNew)
	relNew()
}

func TestPrReviewDeferralService_LifecycleAndBackoff(t *testing.T) {
	service := workflow.NewPrReviewDeferralService()
	ctx := context.Background()

	repoID := "repo-scandrix-api"
	prNumber := 42
	headSHA := "c0ffee123456"

	// 1. Should not defer when no active review
	deferred, err := service.ShouldDefer(ctx, repoID, prNumber, headSHA)
	require.NoError(t, err)
	assert.False(t, deferred)

	// 2. Mark review started (takes lease)
	err = service.MarkStarted(ctx, repoID, prNumber, headSHA)
	require.NoError(t, err)

	// 3. Subsequent collision attempt triggers deferral and increments counter
	deferred, err = service.ShouldDefer(ctx, repoID, prNumber, headSHA)
	require.NoError(t, err)
	assert.True(t, deferred)

	// 4. Calculate backoff delay
	delay1, count1, ok1 := service.CalculateDelay(repoID, prNumber)
	assert.True(t, ok1)
	assert.Equal(t, 1, count1)
	assert.Equal(t, 15*time.Second, delay1)

	// Second collision
	deferred, err = service.ShouldDefer(ctx, repoID, prNumber, headSHA)
	require.NoError(t, err)
	assert.True(t, deferred)

	delay2, count2, ok2 := service.CalculateDelay(repoID, prNumber)
	assert.True(t, ok2)
	assert.Equal(t, 2, count2)
	assert.Equal(t, 30*time.Second, delay2)

	// Third collision
	deferred, err = service.ShouldDefer(ctx, repoID, prNumber, headSHA)
	require.NoError(t, err)
	assert.True(t, deferred)

	delay3, count3, ok3 := service.CalculateDelay(repoID, prNumber)
	assert.True(t, ok3)
	assert.Equal(t, 3, count3)
	assert.Equal(t, 60*time.Second, delay3) // capped at 60s

	// 5. Mark completed releases lease and resets counters
	err = service.MarkCompleted(ctx, repoID, prNumber, headSHA)
	require.NoError(t, err)

	// 6. After completion, PR is free again
	deferredAfter, err := service.ShouldDefer(ctx, repoID, prNumber, headSHA)
	require.NoError(t, err)
	assert.False(t, deferredAfter)
}

func TestPrReviewDeferralService_MaxDeferralBoundary(t *testing.T) {
	service := workflow.NewPrReviewDeferralService()
	ctx := context.Background()

	repoID := "repo-busy"
	prNumber := 99
	headSHA := "abcdef99"

	err := service.MarkStarted(ctx, repoID, prNumber, headSHA)
	require.NoError(t, err)

	// Simulate 27 collisions (maxDeferrals is 26)
	for i := 0; i < 27; i++ {
		_, _ = service.ShouldDefer(ctx, repoID, prNumber, headSHA)
	}

	delay, count, ok := service.CalculateDelay(repoID, prNumber)
	assert.False(t, ok, "should reject further deferrals when max count is exceeded")
	assert.Equal(t, 0*time.Second, delay)
	assert.Greater(t, count, 26)
}

func TestWorkflow_ConcurrentContentionStress(t *testing.T) {
	gate := workflow.NewByokConcurrencyGate(5)
	deferralService := workflow.NewPrReviewDeferralService()
	ctx := context.Background()

	var wg sync.WaitGroup
	workers := 25
	iterations := 40

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			orgID := fmt.Sprintf("org-%d", workerID%3)
			repoID := fmt.Sprintf("repo-%d", workerID%4)
			prNumber := 100 + (workerID % 2)
			headSHA := fmt.Sprintf("sha-%d", workerID)

			for i := 0; i < iterations; i++ {
				// Concurrency gate slot
				release, err := gate.AcquireSlot(ctx, orgID, "anthropic", "claude-sonnet")
				if err == nil {
					time.Sleep(2 * time.Millisecond)
					release()
				}

				// Deferral service
				shouldDefer, err := deferralService.ShouldDefer(ctx, repoID, prNumber, headSHA)
				if err == nil {
					if !shouldDefer {
						_ = deferralService.MarkStarted(ctx, repoID, prNumber, headSHA)
						time.Sleep(3 * time.Millisecond)
						_ = deferralService.MarkCompleted(ctx, repoID, prNumber, headSHA)
					} else {
						_, _, _ = deferralService.CalculateDelay(repoID, prNumber)
					}
				}
			}
		}(w)
	}

	wg.Wait()
}
