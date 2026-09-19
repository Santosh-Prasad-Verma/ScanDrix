package application

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/scandrix/backend/internal/notifications/domain/catalog"
	"github.com/scandrix/backend/internal/notifications/domain/recipient"
)

const (
	ByokErrorThreshold        = 5
	ByokErrorWindowDuration   = 15 * time.Minute
	ByokErrorCooldownDuration = 1 * time.Hour
)

type byokErrorRecord struct {
	timestamp time.Time
	provider  string
	errorMsg  string
}

// NotificationEmitter defines the minimal interface needed to fire notification events.
type NotificationEmitter interface {
	Emit(ctx context.Context, event catalog.Event, orgID string, payload interface{}, recipients []recipient.Recipient, correlationID string) error
}

// ByokErrorCounter tracks rolling BYOK LLM execution errors per organization and triggers threshold alerts.
type ByokErrorCounter struct {
	mu          sync.Mutex
	emitter     NotificationEmitter
	rateLimiter *NotificationRateLimiter
	buckets     map[string][]byokErrorRecord
}

// NewByokErrorCounter creates a new BYOK error counter.
func NewByokErrorCounter(emitter NotificationEmitter, rateLimiter *NotificationRateLimiter) *ByokErrorCounter {
	return &ByokErrorCounter{
		emitter:     emitter,
		rateLimiter: rateLimiter,
		buckets:     make(map[string][]byokErrorRecord),
	}
}

// Record appends a BYOK error to the organization's window. If threshold is breached and cooldown permits, emits an alert.
func (c *ByokErrorCounter) Record(ctx context.Context, orgID string, provider string, errorMessage string) error {
	if orgID == "" {
		return nil
	}

	c.mu.Lock()
	now := time.Now().UTC()
	cutoff := now.Add(-ByokErrorWindowDuration)

	// Filter out stale errors
	existing := c.buckets[orgID]
	var pruned []byokErrorRecord
	for _, rec := range existing {
		if rec.timestamp.After(cutoff) {
			pruned = append(pruned, rec)
		}
	}
	pruned = append(pruned, byokErrorRecord{
		timestamp: now,
		provider:  provider,
		errorMsg:  errorMessage,
	})
	c.buckets[orgID] = pruned

	if len(pruned) < ByokErrorThreshold {
		c.mu.Unlock()
		return nil
	}

	// Reset bucket to prevent repeated immediate triggers
	delete(c.buckets, orgID)
	c.mu.Unlock()

	// Check cooldown
	cooldownKey := fmt.Sprintf("byok:cooldown:%s", orgID)
	if c.rateLimiter != nil && !c.rateLimiter.ShouldEmit(ctx, cooldownKey, int(ByokErrorCooldownDuration.Seconds())) {
		return nil
	}

	if c.emitter == nil {
		return nil
	}

	windowStart := pruned[0].timestamp.Format(time.RFC3339)
	windowEnd := now.Format(time.RFC3339)

	payload := catalog.ByokLlmErrorsThresholdPayload{
		Provider:    provider,
		ErrorCount:  len(pruned),
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
		SampleError: errorMessage,
	}

	return c.emitter.Emit(
		ctx,
		catalog.EventByokLlmErrorsThreshold,
		orgID,
		payload,
		[]recipient.Recipient{recipient.ByRole(catalog.RoleOwner)},
		fmt.Sprintf("byok-alert-%d", now.UnixNano()),
	)
}
