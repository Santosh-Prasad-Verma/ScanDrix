package consumer_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/scandrix/backend/internal/queue/consumer"
	"github.com/scandrix/backend/internal/sandbox/contracts"
	"github.com/scandrix/backend/internal/sandbox/lease"
)

func TestSandboxInvalidateConsumer(t *testing.T) {
	ctx := context.Background()
	repo := lease.NewMemorySandboxLeaseRepository()
	mgr := lease.NewSandboxLeaseManager(nil, repo, nil)

	prKey := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11:my-repo:42"

	// Seed lease
	_, _ = repo.UpsertAcquire(ctx, prKey, 10, "test")

	c := consumer.NewSandboxInvalidateConsumer(mgr)

	payloadBytes, _ := json.Marshal(contracts.SandboxInvalidatePayload{
		PrKey:  prKey,
		Reason: contracts.ReasonPRClosed,
	})

	if err := c.ProcessMessage(ctx, payloadBytes); err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	// Doc was in CREATING so it should be marked INVALIDATED
	doc, _ := repo.FindByPrKey(ctx, prKey)
	if doc == nil || doc.State != lease.StateInvalidated {
		t.Errorf("expected lease to be INVALIDATED, got: %+v", doc)
	}
}
