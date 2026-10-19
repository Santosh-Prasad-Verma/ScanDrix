package warehouse

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnowflakeDriver_DDL_And_Formatting(t *testing.T) {
	driver := NewSnowflakeDriver(nil)
	assert.Equal(t, WarehouseTypeSnowflake, driver.Type())

	cfg := &WarehouseConfig{
		SnowflakeAccount:   "scandrix.us-east-1",
		SnowflakeWarehouse: "ANALYTICS_WH",
		SnowflakeDatabase:  "CORP_DW",
		SnowflakeSchema:    "SCANDRIX",
		SnowflakeUsername:  "SCANDRIX_USER",
	}

	// 1. DDL Generation
	ddl := driver.GenerateSchemaDDL(cfg)
	assert.Contains(t, ddl, "CREATE DATABASE IF NOT EXISTS CORP_DW;")
	assert.Contains(t, ddl, "CREATE TABLE IF NOT EXISTS SCANDRIX_PRS")
	assert.Contains(t, ddl, "CREATE TABLE IF NOT EXISTS SCANDRIX_FINDINGS")
	assert.Contains(t, ddl, "CREATE TABLE IF NOT EXISTS SCANDRIX_DORA_METRICS")
	assert.Contains(t, ddl, "CREATE TABLE IF NOT EXISTS SCANDRIX_TOKEN_USAGE")

	// 2. FormatBatchPayload
	wsID := uuid.New()
	now := time.Now().UTC()
	batch := &WarehouseBatch{
		WorkspaceID: wsID,
		PRs: []PRRecord{
			{
				ID:                   "pr-101",
				WorkspaceID:          wsID.String(),
				RepoID:               "repo-1",
				PRNumber:             42,
				Title:                "Fix security vulnerability in auth",
				Author:               "dev@example.com",
				State:                "MERGED",
				Additions:            50,
				Deletions:            10,
				ChangedFiles:         2,
				CycleTimeSeconds:     3600.0,
				ReviewLatencySeconds: 120.0,
				CreatedAt:            now,
				CompletedAt:          &now,
			},
		},
		Findings: []FindingRecord{
			{
				ID:              "finding-1",
				WorkspaceID:     wsID.String(),
				ReviewID:        "pr-101",
				RuleID:          "OWASP-A01",
				Category:        "SECURITY",
				Severity:        "HIGH",
				FilePath:        "auth.go",
				LineNumber:      10,
				Status:          "RESOLVED",
				ConfidenceScore: 0.95,
				DetectedAt:      now,
			},
		},
		DORAMetrics: []DORARecord{
			{
				WorkspaceID:          wsID.String(),
				Period:               "2026-10",
				DeploymentFrequency:  "10/week",
				LeadTimeMinutes:      30.0,
				ChangeFailureRate:    0.02,
				TimeToRestoreMinutes: 15.0,
				Rating:               "ELITE",
				ComputedAt:           now,
			},
		},
		TokenUsage: []TokenUsageRecord{
			{
				WorkspaceID:      wsID.String(),
				Task:             "REVIEW",
				Model:            "gpt-4o",
				PromptTokens:     1000,
				CompletionTokens: 200,
				TotalTokens:      1200,
				EstimatedCostUSD: 0.005,
				Timestamp:        now,
			},
		},
		GeneratedAt: now,
	}

	payload, err := driver.FormatBatchPayload(batch)
	require.NoError(t, err)
	assert.Contains(t, payload, "MERGE INTO SCANDRIX_PRS")
	assert.Contains(t, payload, "MERGE INTO SCANDRIX_FINDINGS")
	assert.Contains(t, payload, "MERGE INTO SCANDRIX_DORA_METRICS")
	assert.Contains(t, payload, "INSERT INTO SCANDRIX_TOKEN_USAGE")
	assert.Contains(t, payload, "pr-101")
	assert.Contains(t, payload, "OWASP-A01")

	// 3. ExportBatch (dry-run mode)
	res, err := driver.ExportBatch(context.Background(), cfg, batch)
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, 4, res.RecordsWritten)
	assert.Greater(t, res.PayloadBytes, int64(100))
}

func TestBigQueryDriver_DDL_And_Formatting(t *testing.T) {
	driver := NewBigQueryDriver(nil)
	assert.Equal(t, WarehouseTypeBigQuery, driver.Type())

	cfg := &WarehouseConfig{
		BigQueryProjectID: "corp-security-cloud",
		BigQueryDatasetID: "scandrix_bi_dw",
		BigQueryLocation:  "EU",
	}

	// 1. DDL Generation
	ddl := driver.GenerateSchemaDDL(cfg)
	assert.Contains(t, ddl, "CREATE SCHEMA IF NOT EXISTS `corp-security-cloud.scandrix_bi_dw`")
	assert.Contains(t, ddl, "location = 'EU'")
	assert.Contains(t, ddl, "CREATE TABLE IF NOT EXISTS `corp-security-cloud.scandrix_bi_dw.scandrix_prs`")
	assert.Contains(t, ddl, "PARTITION BY DATE(created_at)")
	assert.Contains(t, ddl, "CLUSTER BY workspace_id, repo_id, state")

	// 2. FormatBatchPayload
	wsID := uuid.New()
	now := time.Now().UTC()
	batch := &WarehouseBatch{
		WorkspaceID: wsID,
		PRs: []PRRecord{
			{
				ID:          "pr-202",
				WorkspaceID: wsID.String(),
				RepoID:      "backend-svc",
				PRNumber:    99,
				Title:       "Refactor gRPC pipeline",
				Author:      "jane@corp.io",
				State:       "MERGED",
				CreatedAt:   now,
			},
		},
		GeneratedAt: now,
	}

	payload, err := driver.FormatBatchPayload(batch)
	require.NoError(t, err)
	assert.Contains(t, payload, "scandrix_prs")
	assert.Contains(t, payload, "pr-202")

	// 3. ExportBatch
	res, err := driver.ExportBatch(context.Background(), cfg, batch)
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, 1, res.RecordsWritten)
}

func TestWarehouseSyncer_EndToEnd(t *testing.T) {
	syncer := NewWarehouseSyncer(nil)
	wsID := uuid.New()
	ctx := context.Background()

	// 1. Get default config
	cfg, err := syncer.GetConfig(ctx, wsID)
	require.NoError(t, err)
	assert.False(t, cfg.Enabled)
	assert.Equal(t, WarehouseTypeSnowflake, cfg.Type)

	// 2. Save new config with masked password
	cfg.Enabled = true
	cfg.SnowflakeAccount = "enterprise-dw.snowflakecomputing.com"
	cfg.SnowflakeWarehouse = "REPORTING_WH"
	cfg.SnowflakeDatabase = "SCANDRIX_DW"
	cfg.SnowflakeUsername = "ADMIN"
	cfg.SnowflakePassword = "SuperSecretPassword123!"

	saved, err := syncer.SaveConfig(ctx, cfg)
	require.NoError(t, err)
	assert.True(t, saved.Enabled)
	assert.Equal(t, "••••••••••••••••", saved.SnowflakePassword)

	// 3. Test Connection
	testRes, err := syncer.TestConnection(ctx, saved)
	require.NoError(t, err)
	assert.True(t, testRes.Success)
	assert.Contains(t, testRes.Message, "Configuration validated for Snowflake")

	// 4. Trigger Sync
	run, err := syncer.TriggerSync(ctx, wsID, true)
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", run.Status)
	assert.Greater(t, run.TotalRecords, 0)
	assert.Greater(t, run.PayloadBytes, int64(0))

	// 5. Get History
	history, err := syncer.GetHistory(ctx, wsID, 10)
	require.NoError(t, err)
	assert.Len(t, history, 1)
	assert.Equal(t, run.ID, history[0].ID)

	// 6. Schema DDL retrieval
	ddl, err := syncer.GetSchemaDDL(ctx, wsID, WarehouseTypeSnowflake)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(ddl, "-- ==="))
}
