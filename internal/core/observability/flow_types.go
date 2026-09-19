package observability

// StorageEnum defines supported telemetry persistent storage backends.
type StorageEnum string

const (
	StorageInMemory StorageEnum = "memory"
	StorageMongoDB  StorageEnum = "mongodb"
	StoragePostgres StorageEnum = "postgres"
)

// Branded ID string aliases for flow event tracing.
type (
	FlowCallID        = string
	FlowCorrelationID = string
	FlowEventID       = string
	FlowExecutionID   = string
	FlowSessionID     = string
	FlowTenantID      = string
)

// ObservabilityStorageConfig holds parameters for persistent observability stores.
type ObservabilityStorageConfig struct {
	Type                StorageEnum       `json:"type"`
	ConnectionString    string            `json:"connectionString"`
	Database            string            `json:"database"`
	LogsCollection      string            `json:"logsCollection,omitempty"`
	TelemetryCollection string            `json:"telemetryCollection,omitempty"`
	BatchSize           int               `json:"batchSize,omitempty"`
	FlushIntervalMs     int               `json:"flushIntervalMs,omitempty"`
	TTLDays             int               `json:"ttlDays,omitempty"`
	EnableObservability bool              `json:"enableObservability"`
	SecondaryIndexes    []string          `json:"secondaryIndexes,omitempty"`
	BucketKeys          []string          `json:"bucketKeys,omitempty"`
	ExtraOptions        map[string]string `json:"extraOptions,omitempty"`
}

// MongoDBExporterConfig specifies connection and collection routing for document telemetry.
type MongoDBExporterConfig struct {
	ConnectionString string `json:"connectionString"`
	Database         string `json:"database"`
	LogsCollection   string `json:"logsCollection"`
	TelemetryCollection string `json:"telemetryCollection"`
	BatchSize        int    `json:"batchSize"`
	FlushIntervalMs  int    `json:"flushIntervalMs"`
}
