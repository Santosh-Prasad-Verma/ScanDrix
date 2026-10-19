// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/scandrix/backend/internal/cli/services/api"
	"github.com/scandrix/backend/internal/cli/utils"
	"github.com/spf13/cobra"
)

var (
	warehouseType      string
	warehouseDryRun    bool
	warehouseOutput    string
	warehouseWorkspace string
	warehouseLimit     int
)

var warehouseCmd = &cobra.Command{
	Use:     "warehouse",
	Aliases: []string{"analytics-sync", "wh"},
	Short:   "Manage enterprise BI data warehouse (Snowflake / BigQuery) ELT pipelines",
	Long: `Synchronize pull requests, code findings, DORA metrics, and token usage into
enterprise cloud data warehouses (Snowflake and Google BigQuery) via idempotent ELT streaming.`,
}

var warehouseSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Trigger on-demand ELT synchronization to Snowflake or BigQuery",
	Example: `  scandrix warehouse sync
  scandrix warehouse sync --warehouse=snowflake --dry-run
  scandrix warehouse sync --warehouse=bigquery`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := api.NewClient("", "", "")
		ctx := cmd.Context()

		path := "/api/v1/analytics/warehouse/sync"
		if warehouseWorkspace != "" {
			path = fmt.Sprintf("/api/v1/analytics/warehouse/%s/sync", warehouseWorkspace)
		}
		if warehouseDryRun {
			path += "?dry_run=true"
		}

		status, rawBytes, err := client.DoRaw(ctx, http.MethodPost, path, nil, nil)
		if err != nil {
			return fmt.Errorf("failed to dispatch warehouse sync: %w", err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("server returned error (HTTP %d): %s", status, string(rawBytes))
		}

		var run struct {
			ID            string `json:"id"`
			WarehouseType string `json:"warehouse_type"`
			Status        string `json:"status"`
			PRCount       int    `json:"pr_count"`
			FindingsCount int    `json:"findings_count"`
			MetricsCount  int    `json:"metrics_count"`
			UsageCount    int    `json:"usage_count"`
			TotalRecords  int    `json:"total_records"`
			PayloadBytes  int64  `json:"payload_bytes"`
			DryRun        bool   `json:"dry_run"`
			ErrorMessage  string `json:"error_message,omitempty"`
		}

		if err := json.Unmarshal(rawBytes, &run); err != nil {
			fmt.Println(string(rawBytes))
			return nil
		}

		mode := "LIVE INGESTION"
		if run.DryRun {
			mode = "DRY RUN (STAGED)"
		}

		utils.Success("Warehouse ELT Sync Completed [%s]", mode)
		fmt.Printf("• Job ID:        %s\n", run.ID)
		fmt.Printf("• Destination:   %s\n", strings.ToUpper(run.WarehouseType))
		fmt.Printf("• Status:        %s\n", run.Status)
		fmt.Printf("• Records Total: %d (PRs: %d, Findings: %d, DORA: %d, Tokens: %d)\n",
			run.TotalRecords, run.PRCount, run.FindingsCount, run.MetricsCount, run.UsageCount)
		fmt.Printf("• Payload Size:  %.2f KB\n", float64(run.PayloadBytes)/1024.0)
		if run.ErrorMessage != "" {
			fmt.Printf("• Notice:        %s\n", run.ErrorMessage)
		}
		return nil
	},
}

var warehouseTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Test connectivity and credentials for Snowflake or BigQuery",
	Example: `  scandrix warehouse test --warehouse=snowflake
  scandrix warehouse test --warehouse=bigquery`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := api.NewClient("", "", "")
		ctx := cmd.Context()

		path := "/api/v1/analytics/warehouse/test"
		if warehouseWorkspace != "" {
			path = fmt.Sprintf("/api/v1/analytics/warehouse/%s/test", warehouseWorkspace)
		}

		reqBody := map[string]interface{}{}
		if warehouseType != "" {
			reqBody["type"] = strings.ToLower(warehouseType)
		}

		status, rawBytes, err := client.DoRaw(ctx, http.MethodPost, path, nil, reqBody)
		if err != nil {
			return fmt.Errorf("connection test failed: %w", err)
		}

		var res struct {
			Success          bool     `json:"success"`
			WarehouseType    string   `json:"warehouse_type"`
			LatencyMs        int64    `json:"latency_ms"`
			Message          string   `json:"message"`
			DiscoveredTables []string `json:"discovered_tables"`
		}

		if err := json.Unmarshal(rawBytes, &res); err != nil {
			if status >= 200 && status < 300 {
				utils.Success("Connection probe completed: %s", string(rawBytes))
				return nil
			}
			return fmt.Errorf("connection test failed (HTTP %d): %s", status, string(rawBytes))
		}

		if res.Success {
			utils.Success("Warehouse Connected: %s (%dms latency)", strings.ToUpper(res.WarehouseType), res.LatencyMs)
			fmt.Printf("• Status:  %s\n", res.Message)
			if len(res.DiscoveredTables) > 0 {
				fmt.Printf("• Tables:  %s\n", strings.Join(res.DiscoveredTables, ", "))
			}
		} else {
			utils.Warn("Warehouse Probe Failed: %s", res.Message)
		}
		return nil
	},
}

var warehouseDdlCmd = &cobra.Command{
	Use:   "ddl",
	Short: "Export SQL DDL schema statements for initializing destination tables",
	Example: `  scandrix warehouse ddl --warehouse=snowflake
  scandrix warehouse ddl --warehouse=bigquery --output=schema.sql`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := api.NewClient("", "", "")
		ctx := cmd.Context()

		wh := "snowflake"
		if strings.ToLower(warehouseType) == "bigquery" {
			wh = "bigquery"
		}

		path := fmt.Sprintf("/api/v1/analytics/warehouse/schema-ddl?type=%s", wh)
		if warehouseWorkspace != "" {
			path = fmt.Sprintf("/api/v1/analytics/warehouse/%s/schema-ddl?type=%s", warehouseWorkspace, wh)
		}

		status, rawBytes, err := client.DoRaw(ctx, http.MethodGet, path, nil, nil)
		if err != nil {
			return fmt.Errorf("failed fetching schema DDL: %w", err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("server error (HTTP %d): %s", status, string(rawBytes))
		}

		if warehouseOutput != "" {
			if err := os.WriteFile(warehouseOutput, rawBytes, 0644); err != nil {
				return fmt.Errorf("failed saving DDL to %s: %w", warehouseOutput, err)
			}
			utils.Success("Saved %s DDL schema (%d bytes) to %s", strings.ToUpper(wh), len(rawBytes), warehouseOutput)
		} else {
			fmt.Println(string(rawBytes))
		}
		return nil
	},
}

var warehouseHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "View audit history of past warehouse synchronization jobs",
	Example: `  scandrix warehouse history
  scandrix warehouse history --limit=10`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := api.NewClient("", "", "")
		ctx := cmd.Context()

		limit := 10
		if warehouseLimit > 0 {
			limit = warehouseLimit
		}

		path := fmt.Sprintf("/api/v1/analytics/warehouse/history?limit=%d", limit)
		if warehouseWorkspace != "" {
			path = fmt.Sprintf("/api/v1/analytics/warehouse/%s/history?limit=%d", warehouseWorkspace, limit)
		}

		status, rawBytes, err := client.DoRaw(ctx, http.MethodGet, path, nil, nil)
		if err != nil {
			return fmt.Errorf("failed fetching history: %w", err)
		}
		if status < 200 || status >= 300 {
			return fmt.Errorf("server error (HTTP %d): %s", status, string(rawBytes))
		}

		var runs []struct {
			ID            string `json:"id"`
			WarehouseType string `json:"warehouse_type"`
			Status        string `json:"status"`
			TotalRecords  int    `json:"total_records"`
			PayloadBytes  int64  `json:"payload_bytes"`
			StartedAt     string `json:"started_at"`
		}

		if err := json.Unmarshal(rawBytes, &runs); err != nil || len(runs) == 0 {
			utils.Info("No past warehouse synchronization runs found for this workspace.")
			return nil
		}

		utils.Success("Recent Warehouse Synchronization Runs (%d):", len(runs))
		for _, r := range runs {
			fmt.Printf("• [%s] %s | %s | %d records (%.2f KB) | %s\n",
				r.Status, strings.ToUpper(r.WarehouseType), r.ID[:8], r.TotalRecords, float64(r.PayloadBytes)/1024.0, r.StartedAt)
		}
		return nil
	},
}

func init() {
	warehouseCmd.PersistentFlags().StringVarP(&warehouseWorkspace, "workspace", "w", "", "Target workspace UUID")
	warehouseCmd.PersistentFlags().StringVarP(&warehouseType, "warehouse", "t", "snowflake", "Destination warehouse: snowflake or bigquery")

	warehouseSyncCmd.Flags().BoolVar(&warehouseDryRun, "dry-run", false, "Stage and validate payload without committing to live database")
	warehouseDdlCmd.Flags().StringVarP(&warehouseOutput, "output", "o", "", "Destination file to save DDL statements")
	warehouseHistoryCmd.Flags().IntVarP(&warehouseLimit, "limit", "l", 15, "Max history entries to retrieve")

	warehouseCmd.AddCommand(warehouseSyncCmd)
	warehouseCmd.AddCommand(warehouseTestCmd)
	warehouseCmd.AddCommand(warehouseDdlCmd)
	warehouseCmd.AddCommand(warehouseHistoryCmd)
}
