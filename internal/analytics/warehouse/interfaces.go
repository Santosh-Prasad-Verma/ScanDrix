package warehouse

import (
	"context"
	"time"
)

// ConnectionTestResult reports warehouse validation metrics and schema discovery.
type ConnectionTestResult struct {
	Success          bool     `json:"success"`
	WarehouseType    string   `json:"warehouse_type"`
	LatencyMs        int64    `json:"latency_ms"`
	Message          string   `json:"message"`
	DiscoveredTables []string `json:"discovered_tables"`
	MissingTables    []string `json:"missing_tables"`
	TestedAt         time.Time `json:"tested_at"`
}

// ExportResult contains execution metrics and diagnostics for an ELT export cycle.
type ExportResult struct {
	Success           bool          `json:"success"`
	WarehouseType     WarehouseType `json:"warehouse_type"`
	RecordsWritten    int           `json:"records_written"`
	PayloadBytes      int64         `json:"payload_bytes"`
	ExecutionDuration time.Duration `json:"execution_duration"`
	ExecutionLog      []string      `json:"execution_log"`
	ExecutedAt        time.Time     `json:"executed_at"`
}

// WarehouseDriver defines the pluggable contract for enterprise cloud data warehouses.
type WarehouseDriver interface {
	// Type returns the warehouse driver discriminator.
	Type() WarehouseType

	// TestConnection performs an authentication and connectivity probe against the target.
	TestConnection(ctx context.Context, cfg *WarehouseConfig) (*ConnectionTestResult, error)

	// ExportBatch translates and ships the aggregated domain batch to the target warehouse.
	ExportBatch(ctx context.Context, cfg *WarehouseConfig, batch *WarehouseBatch) (*ExportResult, error)

	// GenerateSchemaDDL returns the authoritative SQL DDL required to initialize the destination schema.
	GenerateSchemaDDL(cfg *WarehouseConfig) string

	// FormatBatchPayload converts the batch into driver-native idempotent statements or rows for review.
	FormatBatchPayload(batch *WarehouseBatch) (string, error)
}
