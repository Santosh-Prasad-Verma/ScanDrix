package sync_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	rulesync "github.com/scandrix/backend/internal/rules/sync"
)

func TestRuleSyncAndCodebaseSweep(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wsID := uuid.New()
	syncer := rulesync.NewRuleSyncer()

	// 1. Sync Declarative Custom Rules from JSON
	rawRulesJSON := []byte(`{
		"version": "1.0",
		"rules": [
			{
				"name": "Custom Insecure Token Check",
				"path_pattern": "*.go",
				"regex_rule": "insecure_token_[a-z0-9]{8}",
				"severity": "CRITICAL",
				"category": "SECURITY_SECRET",
				"description": "Found test insecure token",
				"remediation": "Do not hardcode test tokens"
			},
			{
				"name": "Deprecated Logging Library",
				"path_pattern": "*.go",
				"regex_rule": "logrus\\.New\\(\\)",
				"severity": "LOW",
				"category": "MAINTAINABILITY",
				"description": "Prefer slog over logrus",
				"remediation": "Migrate to standard log/slog"
			}
		]
	}`)

	res, err := syncer.SyncFromJSON(ctx, wsID, rawRulesJSON)
	if err != nil {
		t.Fatalf("failed to sync rules from JSON: %v", err)
	}

	if res.AddedCount != 2 || res.InvalidCount != 0 {
		t.Fatalf("expected 2 added rules, got %+v", res)
	}

	// 2. Verify Rule Update / Drift Detection
	resUpdate, err := syncer.SyncFromJSON(ctx, wsID, rawRulesJSON)
	if err != nil {
		t.Fatalf("failed re-syncing rules: %v", err)
	}
	if resUpdate.AddedCount != 0 || resUpdate.InvalidCount != 0 {
		t.Fatalf("expected 0 added on identical re-sync, got %+v", resUpdate)
	}

	// 3. Get Combined Workspace Rules (Catalog + Custom)
	combinedRules := syncer.GetWorkspaceRules(wsID)
	if len(combinedRules) <= 2 {
		t.Fatalf("expected combined rules to include catalog (>2), got %d", len(combinedRules))
	}

	// 4. Test Codebase Sweeper with Concurrent Workers
	sweeper := rulesync.NewCodebaseSweeper(4)

	filesToScan := []rulesync.FileItem{
		{
			Path: "cmd/api/main.go",
			Content: `package main
import "fmt"
func main() {
	token := "insecure_token_abcdef12"
	fmt.Println(token)
}`,
		},
		{
			Path: "pkg/auth/keys.go",
			Content: `package auth
func Key() string {
	return "AKIAIOSFODNN7EXAMPLE"
}`,
		},
		{
			Path: "pkg/utils/clean.go",
			Content: `package utils
func Add(a, b int) int {
	return a + b
}`,
		},
	}

	sweepReq := rulesync.SweepRequest{
		SweepID:       uuid.New(),
		WorkspaceID:   wsID,
		RepositoryID:  uuid.New(),
		RepoNamespace: "acme/auth-service",
		Branch:        "main",
	}

	report, err := sweeper.ExecuteSweep(ctx, sweepReq, combinedRules, filesToScan)
	if err != nil {
		t.Fatalf("codebase sweep failed: %v", err)
	}

	// 5. Verify Sweep Metrics & Findings
	if report.FilesScanned != 3 {
		t.Fatalf("expected 3 files scanned, got %d", report.FilesScanned)
	}
	if report.LinesProcessed < 10 {
		t.Fatalf("expected at least 10 lines processed, got %d", report.LinesProcessed)
	}
	if report.FindingsCount < 2 {
		t.Fatalf("expected at least 2 findings (custom token + AWS key), got %d", report.FindingsCount)
	}
	if report.CriticalCount < 2 {
		t.Fatalf("expected at least 2 critical findings, got %d", report.CriticalCount)
	}
	if report.Duration <= 0 {
		t.Fatalf("expected non-zero sweep duration")
	}
}
