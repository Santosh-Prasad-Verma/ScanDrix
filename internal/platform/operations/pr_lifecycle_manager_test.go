// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package operations

import (
	"context"
	"testing"
)

func TestPRLifecycleManager_RegisterAndGet(t *testing.T) {
	manager := NewPRLifecycleManager(DefaultReviewCriteria())

	rec := PRLifecycleRecord{
		OrganizationID: "org-1",
		Repository:     "scandrix/core",
		PRNumber:       101,
		HeadSHA:        "sha-1111",
		BaseBranch:     "main",
		HeadBranch:     "feat/audit",
		TotalFiles:     5,
		CriticalCount:  0,
		HighCount:      0,
	}

	saved := manager.RegisterOrUpdatePR(rec)
	if saved == nil || saved.PRNumber != 101 {
		t.Fatalf("unexpected saved record: %+v", saved)
	}

	retrieved, found := manager.GetPRRecord("org-1", "scandrix/core", 101)
	if !found || retrieved.HeadSHA != "sha-1111" {
		t.Fatalf("expected to find record: found=%v, retrieved=%+v", found, retrieved)
	}

	// Update record
	rec.HeadSHA = "sha-2222"
	rec.CriticalCount = 1
	updated := manager.RegisterOrUpdatePR(rec)
	if updated.HeadSHA != "sha-2222" || updated.CriticalCount != 1 {
		t.Fatalf("unexpected updated record: %+v", updated)
	}
}

func TestPRLifecycleManager_EvaluateQualityGate(t *testing.T) {
	ctx := context.Background()
	manager := NewPRLifecycleManager(DefaultReviewCriteria())

	// 1. Draft PR
	draftRecord := &PRLifecycleRecord{
		IsDraft: true,
	}
	state, labels := manager.EvaluateQualityGate(ctx, draftRecord, false, false)
	if state != PRStatePending || len(labels) != 1 || labels[0] != "scandrix:draft" {
		t.Fatalf("unexpected draft evaluation: state=%s, labels=%v", state, labels)
	}

	// 2. Conflicts
	conflictRecord := &PRLifecycleRecord{}
	state, labels = manager.EvaluateQualityGate(ctx, conflictRecord, true, false)
	if state != PRStateBlocked || len(labels) == 0 || labels[0] != "scandrix:conflicts-detected" {
		t.Fatalf("unexpected conflict evaluation: state=%s, labels=%v", state, labels)
	}

	// 3. Secrets Detected
	secretRecord := &PRLifecycleRecord{}
	state, labels = manager.EvaluateQualityGate(ctx, secretRecord, false, true)
	if state != PRStateSecurityAlert || len(labels) < 2 {
		t.Fatalf("unexpected secret evaluation: state=%s, labels=%v", state, labels)
	}

	// 4. Critical Issues
	critRecord := &PRLifecycleRecord{
		CriticalCount: 2,
	}
	state, labels = manager.EvaluateQualityGate(ctx, critRecord, false, false)
	if state != PRStateSecurityAlert {
		t.Fatalf("unexpected critical evaluation: state=%s, labels=%v", state, labels)
	}

	// 5. High Issues
	highRecord := &PRLifecycleRecord{
		HighCount: 1,
	}
	state, labels = manager.EvaluateQualityGate(ctx, highRecord, false, false)
	if state != PRStateChangesRequested {
		t.Fatalf("unexpected high issue evaluation: state=%s, labels=%v", state, labels)
	}

	// 6. Clean Pass
	cleanRecord := &PRLifecycleRecord{
		CriticalCount: 0,
		HighCount:     0,
	}
	state, labels = manager.EvaluateQualityGate(ctx, cleanRecord, false, false)
	if state != PRStatePassed || labels[0] != "scandrix:approved" {
		t.Fatalf("unexpected clean evaluation: state=%s, labels=%v", state, labels)
	}
}

func TestPRLifecycleManager_FormatNotificationPayload(t *testing.T) {
	manager := NewPRLifecycleManager(DefaultReviewCriteria())

	rec := &PRLifecycleRecord{
		Repository:    "scandrix/backend",
		PRNumber:      42,
		State:         PRStateSecurityAlert,
		CriticalCount: 1,
		HighCount:     2,
		TotalFiles:    12,
		LabelsApplied: []string{"scandrix:security-alert"},
	}

	payload := manager.FormatNotificationPayload(rec)
	if payload["title"] != "ScanDrix Code Review: scandrix/backend #42" {
		t.Fatalf("unexpected title: %v", payload["title"])
	}
	if payload["color"] != "#ef4444" {
		t.Fatalf("expected red color for security alert: %v", payload["color"])
	}

	// Passed state
	rec.State = PRStatePassed
	payloadPassed := manager.FormatNotificationPayload(rec)
	if payloadPassed["color"] != "#22c55e" {
		t.Fatalf("expected green color for passed: %v", payloadPassed["color"])
	}
}
