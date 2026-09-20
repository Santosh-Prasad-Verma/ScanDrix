// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package operations

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/scandrix/backend/pkg/models"
)

func TestAuditLogService_RecordAndVerifyIntegrity(t *testing.T) {
	ctx := context.Background()
	svc := NewAuditLogService()

	// 1. Append records
	rec1, err := svc.Record(ctx, "org-1", "scandrix/core", models.ProviderGitHub, "alice", ActionReviewDispatched, SeverityInfo, 101, "sha-1", map[string]any{"mode": "heavy"})
	if err != nil || rec1 == nil {
		t.Fatalf("failed to record entry 1: %v", err)
	}

	rec2, err := svc.Record(ctx, "org-1", "scandrix/core", models.ProviderGitHub, "drixy", ActionSecretDetected, SeverityCritical, 101, "sha-1", map[string]any{"rule": "jwt-token"})
	if err != nil || rec2 == nil {
		t.Fatalf("failed to record entry 2: %v", err)
	}

	if rec2.PrevHash != rec1.CurrentHash {
		t.Fatalf("expected prevHash of rec2 to equal currentHash of rec1")
	}

	// 2. Verify Integrity (clean)
	valid, count, err := svc.VerifyIntegrity()
	if !valid || err != nil || count != 2 {
		t.Fatalf("integrity check failed on clean chain: valid=%v, count=%d, err=%v", valid, count, err)
	}

	// 3. Query
	records, total := svc.Query(ctx, AuditLogFilter{
		OrganizationID: "org-1",
		Severity:       SeverityCritical,
	})
	if total != 1 || len(records) != 1 || records[0].Action != ActionSecretDetected {
		t.Fatalf("unexpected query result: total=%d, records=%+v", total, records)
	}

	// 4. Export JSON and CSV
	jsonBytes, err := svc.ExportJSON(AuditLogFilter{OrganizationID: "org-1"})
	if err != nil || !strings.Contains(string(jsonBytes), "SECRET_DETECTED") {
		t.Fatalf("unexpected json export: %s", string(jsonBytes))
	}

	csvStr := svc.ExportCSV(AuditLogFilter{OrganizationID: "org-1"})
	if !strings.Contains(csvStr, "scandrix/core") || !strings.Contains(csvStr, "CRITICAL") {
		t.Fatalf("unexpected csv export: %s", csvStr)
	}
}

func TestAuditLogService_TamperDetection(t *testing.T) {
	ctx := context.Background()
	svc := NewAuditLogService()

	_, _ = svc.Record(ctx, "org-1", "scandrix/core", models.ProviderGitHub, "alice", ActionReviewDispatched, SeverityInfo, 101, "sha-1", nil)
	_, _ = svc.Record(ctx, "org-1", "scandrix/core", models.ProviderGitHub, "bob", ActionCommentCreated, SeverityInfo, 101, "sha-1", nil)

	// Tamper with record 0
	svc.records[0].Actor = "eve-attacker"

	valid, idx, err := svc.VerifyIntegrity()
	if valid || err == nil || idx != 0 {
		t.Fatalf("expected tamper detection at index 0, got valid=%v, idx=%d, err=%v", valid, idx, err)
	}
}

func TestAuditLogService_TimeAndPaginationFilters(t *testing.T) {
	ctx := context.Background()
	svc := NewAuditLogService()

	for i := 1; i <= 10; i++ {
		_, _ = svc.Record(ctx, "org-1", "scandrix/repo", models.ProviderGitLab, "bot", ActionQualityGatePassed, SeverityInfo, i, "sha", nil)
	}

	// Limit & offset
	records, total := svc.Query(ctx, AuditLogFilter{
		Limit:  3,
		Offset: 2,
	})
	if total != 10 || len(records) != 3 {
		t.Fatalf("unexpected pagination: total=%d, count=%d", total, len(records))
	}

	// Time filtering
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)
	records, total = svc.Query(ctx, AuditLogFilter{
		FromTime: &past,
		ToTime:   &future,
	})
	if total != 10 {
		t.Fatalf("expected all 10 records within range, got %d", total)
	}
}
