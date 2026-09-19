package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/scandrix/backend/internal/sandbox/contracts"
)

// SandboxInvalidateConsumer listens for sandbox.invalidate events and instructs the lease manager to clean/invalidate resources.
type SandboxInvalidateConsumer struct {
	leaseMgr contracts.ISandboxLeaseManager
}

// NewSandboxInvalidateConsumer creates a consumer for sandbox.invalidate events.
func NewSandboxInvalidateConsumer(leaseMgr contracts.ISandboxLeaseManager) *SandboxInvalidateConsumer {
	return &SandboxInvalidateConsumer{leaseMgr: leaseMgr}
}

// ProcessMessage decodes the invalidation payload and invokes Invalidate on the lease manager.
func (c *SandboxInvalidateConsumer) ProcessMessage(ctx context.Context, payload []byte) error {
	if c.leaseMgr == nil {
		return nil
	}

	var event contracts.SandboxInvalidatePayload
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("failed unmarshaling sandbox invalidate payload: %w", err)
	}

	if event.PrKey == "" {
		slog.Warn("Received sandbox.invalidate message with empty pr_key")
		return nil
	}

	slog.Info("Processing sandbox invalidation event", "pr_key", event.PrKey, "reason", event.Reason)
	return c.leaseMgr.Invalidate(ctx, event.PrKey)
}
