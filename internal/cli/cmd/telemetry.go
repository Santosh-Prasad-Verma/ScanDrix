// ScanDrix AI - Developer Terminal & CI/CD CLI
// Package: cmd
// File: telemetry.go

package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/scandrix/backend/internal/config"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/telemetry/beacon"
	"github.com/spf13/cobra"
)

var forceTelemetrySend bool

var telemetryCmd = &cobra.Command{
	Use:   "telemetry",
	Short: "Inspect, preview, and test self-hosted anonymous usage telemetry",
	Long: `The telemetry command enables operators to inspect what data is transmitted
by ScanDrix's anonymous daily heartbeat, preview the exact payload, or force-send
a heartbeat to verify network connectivity.`,
}

var telemetryPreviewCmd = &cobra.Command{
	Use:   "preview",
	Short: "Preview the exact anonymous heartbeat payload without sending it",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		service, cleanup, err := initBeaconService(ctx)
		if err != nil {
			return err
		}
		defer cleanup()

		payload, err := service.Preview(ctx)
		if err != nil {
			return fmt.Errorf("failed generating telemetry preview: %w", err)
		}

		jsonBytes, err := beacon.MarshalPayloadToJSON(payload)
		if err != nil {
			return fmt.Errorf("failed formatting json payload: %w", err)
		}

		fmt.Println(string(jsonBytes))
		return nil
	},
}

var telemetrySendCmd = &cobra.Command{
	Use:   "send",
	Short: "Trigger the anonymous heartbeat send workflow",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		service, cleanup, err := initBeaconService(ctx)
		if err != nil {
			return err
		}
		defer cleanup()

		if forceTelemetrySend {
			_ = os.Setenv("SCANDRIX_TELEMETRY_FORCE", "1")
		}

		start := time.Now()
		if err := service.Run(ctx); err != nil {
			return fmt.Errorf("beacon execution error: %w", err)
		}

		fmt.Printf("✔ Telemetry heartbeat executed in %v\n", time.Since(start))
		return nil
	},
}

var telemetryStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Display current telemetry configuration and opt-out state",
	Run: func(cmd *cobra.Command, args []string) {
		provider := beacon.NewBeaconHTTPProvider(nil)
		disabled := provider.IsDisabled()

		fmt.Println("\n=== ScanDrix Telemetry Status ===")
		if disabled {
			fmt.Println("Status:   DISABLED (SCANDRIX_TELEMETRY_DISABLED or DO_NOT_TRACK is active)")
		} else {
			fmt.Println("Status:   ENABLED (Sending aggregated non-PII daily heartbeats)")
		}
		endpoint := os.Getenv("SCANDRIX_TELEMETRY_ENDPOINT")
		if endpoint == "" {
			endpoint = beacon.DefaultTelemetryEndpoint
		}
		fmt.Printf("Endpoint: %s\n", endpoint)
		fmt.Println("\nTransparency:")
		fmt.Println("  * No source code, filenames, PR titles, or identities are transmitted.")
		fmt.Println("  * Run 'scandrix telemetry preview' to inspect the unredacted payload.")
		fmt.Println("  * Set SCANDRIX_TELEMETRY_DISABLED=true to disable completely.")
	},
}

func init() {
	telemetrySendCmd.Flags().BoolVarP(&forceTelemetrySend, "force", "f", false, "Bypass daily deduplication and force transmission")

	telemetryCmd.AddCommand(telemetryPreviewCmd)
	telemetryCmd.AddCommand(telemetrySendCmd)
	telemetryCmd.AddCommand(telemetryStatusCmd)

	RootCmd.AddCommand(telemetryCmd)
}

func initBeaconService(ctx context.Context) (*beacon.SelfHostedBeaconService, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		// Fallback to in-memory store if config/DB cannot load
		store := beacon.NewInMemoryTelemetryStateStore()
		collector := beacon.NewHeartbeatCollectorService(nil, slog.Default())
		transport := beacon.NewBeaconHTTPProvider(slog.Default())
		return beacon.NewSelfHostedBeaconService(store, collector, transport, slog.Default()), func() {}, nil
	}

	dbClient, err := database.NewClient(ctx, cfg.DatabaseURL)
	if err != nil {
		// Fallback to in-memory store
		store := beacon.NewInMemoryTelemetryStateStore()
		collector := beacon.NewHeartbeatCollectorService(nil, slog.Default())
		transport := beacon.NewBeaconHTTPProvider(slog.Default())
		return beacon.NewSelfHostedBeaconService(store, collector, transport, slog.Default()), func() {}, nil
	}

	cleanup := func() {
		dbClient.Close()
	}

	store := beacon.NewPostgresTelemetryStateStore(dbClient.Pool)
	_ = store.EnsureTable(ctx)
	collector := beacon.NewHeartbeatCollectorService(dbClient.Pool, slog.Default())
	transport := beacon.NewBeaconHTTPProvider(slog.Default())
	service := beacon.NewSelfHostedBeaconService(store, collector, transport, slog.Default())

	return service, cleanup, nil
}
