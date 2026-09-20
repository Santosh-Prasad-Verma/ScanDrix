// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package observers

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

type mockWebhookObserver struct {
	mu            sync.Mutex
	receivedCount int
	receivedTypes map[PipelineEventType]bool
}

func newMockWebhookObserver() *mockWebhookObserver {
	return &mockWebhookObserver{
		receivedTypes: make(map[PipelineEventType]bool),
	}
}

func (w *mockWebhookObserver) Name() string {
	return "MockWebhookObserver"
}

func (w *mockWebhookObserver) OnEvent(ctx context.Context, ev PipelineEvent) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.receivedCount++
	w.receivedTypes[ev.Type] = true
	return nil
}

func TestPipelineEventDispatcher_LifecycleBroadcast(t *testing.T) {
	dispatcher := NewPipelineEventDispatcher(100, 2)
	metrics := NewMetricsObserver()
	webhook := newMockWebhookObserver()

	dispatcher.RegisterObserver(metrics)
	dispatcher.RegisterObserver(webhook)

	reviewID := uuid.New()

	// 1. Emit Review Started
	dispatcher.Emit(PipelineEvent{
		Type:       EventReviewStarted,
		ReviewID:   reviewID,
		PullNumber: 42,
		RepoID:     "repo-alpha",
	})

	// 2. Emit Finding Emitted
	dispatcher.Emit(PipelineEvent{
		Type:       EventFindingEmitted,
		ReviewID:   reviewID,
		PullNumber: 42,
		Finding: &models.CodeFinding{
			Title: "Unchecked error return",
		},
	})

	// 3. Emit Stage Started
	dispatcher.Emit(PipelineEvent{
		Type:       EventStageStarted,
		ReviewID:   reviewID,
		PullNumber: 42,
		StageName:  "SecurityAuditor",
	})

	// 4. Emit Review Completed
	dispatcher.Emit(PipelineEvent{
		Type:       EventReviewCompleted,
		ReviewID:   reviewID,
		PullNumber: 42,
		Duration:   1250 * time.Millisecond,
	})

	// Shutdown dispatcher gracefully
	err := dispatcher.Close(2 * time.Second)
	if err != nil {
		t.Fatalf("unexpected error closing dispatcher: %v", err)
	}

	// Verify metrics observer received all 4 events
	if metrics.TotalEvents != 4 {
		t.Errorf("expected 4 events in metrics observer, got %d", metrics.TotalEvents)
	}
	if metrics.GetCount(EventReviewStarted) != 1 {
		t.Errorf("expected 1 REVIEW_STARTED event")
	}
	if metrics.GetCount(EventFindingEmitted) != 1 {
		t.Errorf("expected 1 FINDING_EMITTED event")
	}

	// Verify webhook observer received events
	webhook.mu.Lock()
	defer webhook.mu.Unlock()
	if webhook.receivedCount != 4 {
		t.Errorf("expected 4 events in webhook observer, got %d", webhook.receivedCount)
	}
	if !webhook.receivedTypes[EventReviewCompleted] {
		t.Errorf("expected EventReviewCompleted to be recorded in receivedTypes")
	}
}

func TestPipelineEventDispatcher_BufferSaturationNonBlocking(t *testing.T) {
	// Tiny buffer of size 2, 1 slow worker
	dispatcher := NewPipelineEventDispatcher(2, 1)

	// Rapidly emit 20 events without blocking
	start := time.Now()
	emitted := 0
	for i := 0; i < 20; i++ {
		if ok := dispatcher.Emit(PipelineEvent{
			Type:       EventStageStarted,
			StageName:  fmt.Sprintf("Stage_%d", i),
			PullNumber: i,
		}); ok {
			emitted++
		}
	}
	elapsed := time.Since(start)

	// Emit must return immediately (< 100ms) even if queue fills
	if elapsed > 100*time.Millisecond {
		t.Errorf("emit should be non-blocking, took %v", elapsed)
	}

	_ = dispatcher.Close(1 * time.Second)
}

func TestPipelineEventDispatcher_ConcurrentStress(t *testing.T) {
	dispatcher := NewPipelineEventDispatcher(500, 4)
	metrics := NewMetricsObserver()
	dispatcher.RegisterObserver(metrics)

	var wg sync.WaitGroup
	workers := 50

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				dispatcher.Emit(PipelineEvent{
					Type:       EventFindingEmitted,
					ReviewID:   uuid.New(),
					PullNumber: id,
				})
			}
		}(w)
	}

	wg.Wait()
	_ = dispatcher.Close(2 * time.Second)

	if metrics.TotalEvents == 0 {
		t.Errorf("expected events to be processed under concurrent stress")
	}
}
