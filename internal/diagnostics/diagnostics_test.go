package diagnostics_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/cache/limiter"
	"github.com/scandrix/backend/internal/diagnostics"
	"github.com/scandrix/backend/internal/queue/relay"
)

func TestDiagnosticsAndSelfHealing(t *testing.T) {
	ctx := context.Background()

	outbox := relay.NewOutboxStore()
	cache := limiter.NewTieredCache()
	prober := diagnostics.NewHealthProber(outbox, cache)
	daemon := diagnostics.NewSelfHealingDaemon(prober, outbox, cache)

	// 1. Initial State: All components healthy
	rep1 := prober.RunDiagnostics(ctx)
	if rep1.OverallStatus != diagnostics.StatusHealthy {
		t.Fatalf("expected overall healthy, got %s", rep1.OverallStatus)
	}
	if rep1.Components["queue_outbox"].Status != diagnostics.StatusHealthy {
		t.Fatalf("expected queue healthy, got %+v", rep1.Components["queue_outbox"])
	}
	if rep1.Components["cache_tiered"].Status != diagnostics.StatusHealthy {
		t.Fatalf("expected cache healthy, got %+v", rep1.Components["cache_tiered"])
	}

	// 2. Inject Dead-Letter Message to trigger degraded status
	msgID := uuid.New()
	_ = outbox.Insert(ctx, relay.OutboxMessage{
		ID:        msgID,
		Topic:     "test.topic",
		Payload:   []byte("test payload"),
		CreatedAt: time.Now().UTC(),
	})
	// Force to dead letter by setting attempts = 5
	claimed, _ := outbox.ClaimPending(ctx, "worker-diag", 1, 1*time.Minute)
	if len(claimed) == 1 {
		for i := 0; i < 5; i++ {
			_ = outbox.RecordFailure(ctx, msgID, "simulated upstream failure")
		}
	}

	// Probing should now detect degraded outbox status
	rep2 := prober.RunDiagnostics(ctx)
	if rep2.Components["queue_outbox"].Status != diagnostics.StatusDegraded {
		t.Fatalf("expected degraded queue status, got %+v", rep2.Components["queue_outbox"])
	}
	if rep2.OverallStatus != diagnostics.StatusDegraded {
		t.Fatalf("expected overall degraded status, got %s", rep2.OverallStatus)
	}

	// 3. Trigger Self-Healing Daemon: must execute DLQ Redrive
	actions, _ := daemon.EvaluateAndHeal(ctx)
	if len(actions) != 1 {
		t.Fatalf("expected 1 self-healing action, got %d", len(actions))
	}
	action := actions[0]
	if action.ActionType != diagnostics.ActionDLQRedrive || !action.Success {
		t.Fatalf("expected successful DLQ redrive, got %+v", action)
	}

	// 4. Re-probe: Outbox should be restored to Healthy!
	repRecovered := prober.RunDiagnostics(ctx)
	if repRecovered.Components["queue_outbox"].Status != diagnostics.StatusHealthy {
		t.Fatalf("expected queue restored to healthy after healing, got %+v", repRecovered.Components["queue_outbox"])
	}
	if repRecovered.OverallStatus != diagnostics.StatusHealthy {
		t.Fatalf("expected overall restored to healthy, got %s", repRecovered.OverallStatus)
	}

	// 5. Verify Audit Log
	auditLog := daemon.GetAuditLog()
	if len(auditLog) != 1 || auditLog[0].ID != action.ID {
		t.Fatalf("expected 1 audit log entry, got %+v", auditLog)
	}
}
