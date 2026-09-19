// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package routing

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestDynamicModelRouter_DefaultTaskResolution(t *testing.T) {
	router := NewDynamicModelRouter(nil)

	// File triage default should be haiku
	slot, usedFallback, err := router.ResolveSlot(context.Background(), TaskFileTriage, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usedFallback {
		t.Errorf("expected primary slot, not fallback")
	}
	if slot.ID != "anthropic-haiku" {
		t.Errorf("expected anthropic-haiku for file triage, got %s", slot.ID)
	}

	// Deep deliberation should be reasoning
	slotDeep, _, err := router.ResolveSlot(context.Background(), TaskDeepDeliberation, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if slotDeep.ID != "anthropic-sonnet-reasoning" {
		t.Errorf("expected anthropic-sonnet-reasoning for deep deliberation, got %s", slotDeep.ID)
	}
}

func TestDynamicModelRouter_WorkspaceOverride(t *testing.T) {
	router := NewDynamicModelRouter(nil)
	wsID := "workspace-enterprise-99"

	// Override file triage to use gemini-flash
	err := router.SetWorkspaceTaskSlot(wsID, TaskFileTriage, "gemini-flash")
	if err != nil {
		t.Fatalf("unexpected error setting workspace slot: %v", err)
	}

	// Resolve for that workspace
	slot, usedFallback, err := router.ResolveSlot(context.Background(), TaskFileTriage, wsID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usedFallback {
		t.Errorf("expected primary override slot")
	}
	if slot.ID != "gemini-flash" {
		t.Errorf("expected gemini-flash, got %s", slot.ID)
	}

	// Other workspace should still resolve to default
	defaultSlot, _, _ := router.ResolveSlot(context.Background(), TaskFileTriage, "workspace-other")
	if defaultSlot.ID != "anthropic-haiku" {
		t.Errorf("expected anthropic-haiku for other workspace, got %s", defaultSlot.ID)
	}
}

func TestDynamicModelRouter_FailoverAndCircuitBreaker(t *testing.T) {
	cfg := &RouterConfig{
		FailureThreshold: 2,
		CircuitCooldown:  50 * time.Millisecond,
	}
	router := NewDynamicModelRouter(cfg)

	// 1. Initial resolution: anthropic-haiku
	slot, usedFallback, err := router.ResolveSlot(context.Background(), TaskFileTriage, "")
	if err != nil || usedFallback || slot.ID != "anthropic-haiku" {
		t.Fatalf("initial resolution failed: slot=%s, fallback=%v, err=%v", slot.ID, usedFallback, err)
	}

	// 2. Report 1st failure (503 Service Unavailable)
	tripped := router.RecordFailure("anthropic-haiku", errors.New("503 Service Unavailable"))
	if tripped {
		t.Errorf("circuit should not trip after 1 failure (threshold=2)")
	}

	// 3. Report 2nd failure
	tripped = router.RecordFailure("anthropic-haiku", errors.New("500 Internal Server Error"))
	if !tripped {
		t.Errorf("circuit should trip after 2 failures")
	}

	// 4. ResolveSlot should now cascade to fallback (gemini-flash)
	fbSlot, usedFallback, err := router.ResolveSlot(context.Background(), TaskFileTriage, "")
	if err != nil {
		t.Fatalf("unexpected error resolving fallback: %v", err)
	}
	if !usedFallback {
		t.Errorf("expected usedFallback to be true")
	}
	if fbSlot.ID != "gemini-flash" {
		t.Errorf("expected fallback gemini-flash, got %s", fbSlot.ID)
	}

	// 5. Wait for cooldown to expire
	time.Sleep(60 * time.Millisecond)

	// Next resolve should enter Half-Open and return anthropic-haiku for trial
	trialSlot, usedFallback, err := router.ResolveSlot(context.Background(), TaskFileTriage, "")
	if err != nil {
		t.Fatalf("unexpected error on half-open resolution: %v", err)
	}
	if usedFallback {
		t.Errorf("half-open trial should test primary slot")
	}
	if trialSlot.ID != "anthropic-haiku" {
		t.Errorf("expected trial slot anthropic-haiku, got %s", trialSlot.ID)
	}

	// 6. Report success on trial slot -> closes circuit
	router.RecordSuccess("anthropic-haiku", 1200)

	health, ok := router.GetSlotHealth("anthropic-haiku")
	if !ok || health.CircuitState != CircuitClosed {
		t.Errorf("expected circuit to be CLOSED after successful trial, got %s", health.CircuitState)
	}
	if health.TotalTokensConsumed != 1200 {
		t.Errorf("expected 1200 tokens consumed, got %d", health.TotalTokensConsumed)
	}
}

func TestDynamicModelRouter_NonTripErrors(t *testing.T) {
	router := NewDynamicModelRouter(&RouterConfig{FailureThreshold: 1})

	// Non-trip error: context_overflow
	tripped := router.RecordFailure("anthropic-haiku", errors.New("context_overflow: prompt is 250k tokens"))
	if tripped {
		t.Errorf("context_overflow should not trip circuit breaker")
	}

	health, _ := router.GetSlotHealth("anthropic-haiku")
	if health.ConsecutiveFailures != 0 {
		t.Errorf("consecutive failures should be 0, got %d", health.ConsecutiveFailures)
	}

	// Non-trip error: client cancel
	tripped = router.RecordFailure("anthropic-haiku", errors.New("context canceled"))
	if tripped {
		t.Errorf("context canceled should not trip circuit breaker")
	}
}

func TestDynamicModelRouter_ConcurrentStress(t *testing.T) {
	router := NewDynamicModelRouter(nil)

	var wg sync.WaitGroup
	tasks := []ReviewTaskType{
		TaskFileTriage,
		TaskRuleEvaluation,
		TaskDeepDeliberation,
		TaskSuggestionVerification,
		TaskDeduplication,
		TaskSummaryGeneration,
	}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			task := tasks[workerID%len(tasks)]
			wsID := fmt.Sprintf("ws-%d", workerID%4)

			slot, _, err := router.ResolveSlot(context.Background(), task, wsID)
			if err == nil {
				if workerID%3 == 0 {
					router.RecordFailure(slot.ID, errors.New("502 Bad Gateway"))
				} else {
					router.RecordSuccess(slot.ID, 500)
				}
			}
		}(i)
	}

	wg.Wait()
}
