package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/codehound/codehound/core/pkg/audit"
	"github.com/codehound/codehound/core/pkg/config"
	"github.com/codehound/codehound/core/pkg/database"
	"github.com/google/uuid"
)

func TestAuditLoggerPipeline(t *testing.T) {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, database.DefaultConfig(cfg.DatabaseURL))
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	defer pool.Close()

	tenantID := uuid.New()
	resourceID := uuid.New()

	logger := audit.NewLogger(pool, 100)
	defer logger.Close()

	// Log sync event
	evt := audit.EventInput{
		TenantID:     tenantID,
		ActorType:    "API_KEY",
		Action:       "SCAN_LAUNCHED",
		ResourceType: "SCAN",
		ResourceID:   resourceID,
		Payload:      map[string]any{"scan_type": "FULL", "commit_sha": "abcd1234ef"},
	}

	if err := logger.LogSync(ctx, evt); err != nil {
		t.Fatalf("failed to log sync event: %v", err)
	}

	// Verify insertion into partitioned audit_events
	var count int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_events WHERE resource_id = $1;`, resourceID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query audit_events: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 audit event in db, got %d", count)
	}
}
