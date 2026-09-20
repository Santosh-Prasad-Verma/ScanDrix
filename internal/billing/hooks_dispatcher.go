// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev
// ═══════════════════════════════════════════════════════════════

package billing

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/enterprise/audit"
)

// BillingEventHandler defines the callback interface for billing lifecycle hooks.
type BillingEventHandler interface {
	HandleBillingEvent(ctx context.Context, event BillingWebhookEvent) error
}

// BillingHandlerFunc allows using plain functions as event handlers.
type BillingHandlerFunc func(ctx context.Context, event BillingWebhookEvent) error

func (f BillingHandlerFunc) HandleBillingEvent(ctx context.Context, event BillingWebhookEvent) error {
	return f(ctx, event)
}

// HooksDispatcherConfig configures hook worker pool and idempotency parameters.
type HooksDispatcherConfig struct {
	WorkerCount     int           `json:"worker_count"`
	QueueCapacity   int           `json:"queue_capacity"`
	LockTTL         time.Duration `json:"lock_ttl"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`
}

// DefaultHooksDispatcherConfig provides production defaults.
func DefaultHooksDispatcherConfig() HooksDispatcherConfig {
	return HooksDispatcherConfig{
		WorkerCount:     8,
		QueueCapacity:   500,
		LockTTL:         5 * time.Minute,
		ShutdownTimeout: 10 * time.Second,
	}
}

// HooksDispatcher coordinates incoming billing events to domain handlers with idempotency.
type HooksDispatcher struct {
	mu            sync.RWMutex
	handlers      map[BillingEventType][]BillingEventHandler
	idempotency   IdempotencyStore
	auditListener *audit.AuditLogListener
	config        HooksDispatcherConfig
	workQueue     chan BillingWebhookEvent
	wg            sync.WaitGroup
	quit          chan struct{}
	stopped       bool
}

// NewHooksDispatcher creates an enterprise billing hooks dispatcher.
func NewHooksDispatcher(
	idempotency IdempotencyStore,
	auditListener *audit.AuditLogListener,
	cfg HooksDispatcherConfig,
) *HooksDispatcher {
	d := &HooksDispatcher{
		handlers:      make(map[BillingEventType][]BillingEventHandler),
		idempotency:   idempotency,
		auditListener: auditListener,
		config:        cfg,
		workQueue:     make(chan BillingWebhookEvent, cfg.QueueCapacity),
		quit:          make(chan struct{}),
	}

	d.startWorkers()
	return d
}

// RegisterHandler binds an event type to a domain handler.
func (d *HooksDispatcher) RegisterHandler(eventType BillingEventType, handler BillingEventHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[eventType] = append(d.handlers[eventType], handler)
}

// Dispatch queues an event for background worker processing.
func (d *HooksDispatcher) Dispatch(ctx context.Context, event BillingWebhookEvent) error {
	d.mu.RLock()
	if d.stopped {
		d.mu.RUnlock()
		return fmt.Errorf("dispatcher is stopped")
	}
	d.mu.RUnlock()

	select {
	case d.workQueue <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("billing hooks queue is full (capacity %d)", d.config.QueueCapacity)
	}
}

// ProcessSync processes an event immediately in the calling goroutine (useful for testing and critical sync flows).
func (d *HooksDispatcher) ProcessSync(ctx context.Context, event BillingWebhookEvent) error {
	return d.executeEvent(ctx, event)
}

func (d *HooksDispatcher) startWorkers() {
	for i := 0; i < d.config.WorkerCount; i++ {
		d.wg.Add(1)
		go func(workerID int) {
			defer d.wg.Done()
			for {
				select {
				case <-d.quit:
					return
				case event, ok := <-d.workQueue:
					if !ok {
						return
					}
					if err := d.executeEvent(context.Background(), event); err != nil {
						slog.Error("Failed executing billing event hook",
							"worker_id", workerID,
							"event_id", event.ID,
							"event_type", event.EventType,
							"error", err,
						)
					}
				}
			}
		}(i)
	}
}

func (d *HooksDispatcher) executeEvent(ctx context.Context, event BillingWebhookEvent) error {
	// Idempotency check: attempt to acquire lock
	if d.idempotency != nil {
		acquired, err := d.idempotency.TryAcquire(ctx, event.ID, d.config.LockTTL)
		if err != nil {
			if err == ErrEventAlreadyProcessed {
				slog.Info("Skipping already processed billing event", "event_id", event.ID, "event_type", event.EventType)
				return nil
			}
			return fmt.Errorf("idempotency check error: %w", err)
		}
		if !acquired {
			slog.Warn("Could not acquire lock for billing event", "event_id", event.ID)
			return nil
		}
	}

	d.mu.RLock()
	handlers := append([]BillingEventHandler(nil), d.handlers[event.EventType]...)
	d.mu.RUnlock()

	var firstErr error
	for _, h := range handlers {
		if err := h.HandleBillingEvent(ctx, event); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			slog.Error("Billing handler failed", "event_id", event.ID, "event_type", event.EventType, "error", err)
		}
	}

	// Update idempotency status
	if d.idempotency != nil {
		if firstErr != nil {
			_ = d.idempotency.MarkFailed(ctx, event.ID, firstErr)
		} else {
			_ = d.idempotency.MarkCompleted(ctx, event.ID)
		}
	}

	// Emit Enterprise Audit Log Event
	if d.auditListener != nil && firstErr == nil {
		d.emitBillingAuditEvent(event)
	}

	return firstErr
}

func (d *HooksDispatcher) emitBillingAuditEvent(event BillingWebhookEvent) {
	actor := audit.ActorContext{
		UserID:     uuid.Nil.String(),
		Email:      fmt.Sprintf("billing-%s@scandrix.dev", event.Provider),
		ClientIP:   "127.0.0.1",
		UserAgent:  fmt.Sprintf("ScanDrixBillingHook/%s", event.Provider),
		AuthMethod: "system",
	}

	target := audit.TargetContext{
		OrganizationID: event.OrganizationID,
		WorkspaceID:    event.WorkspaceID,
		TargetEntityID: event.ID,
		TargetType:     "BillingEvent",
	}

	auditEv := audit.EnterpriseLogEvent{
		ID:        uuid.New(),
		Category:  audit.CategoryOrgSettings,
		Action:    string(event.EventType),
		Actor:     actor,
		Target:    target,
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"provider":   event.Provider,
			"event_type": event.EventType,
			"event_id":   event.ID,
		},
	}

	_ = d.auditListener.Emit(auditEv)
}

// Stop terminates worker goroutines and flushes remaining queue elements.
func (d *HooksDispatcher) Stop() {
	d.mu.Lock()
	if d.stopped {
		d.mu.Unlock()
		return
	}
	d.stopped = true
	d.mu.Unlock()

	close(d.quit)
	close(d.workQueue)
	d.wg.Wait()
}
