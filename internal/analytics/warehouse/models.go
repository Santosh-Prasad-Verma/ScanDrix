package warehouse

import (
	"time"

	"github.com/google/uuid"
)

// WarehouseType distinguishes between enterprise analytics data warehouse targets.
type WarehouseType string

const (
	WarehouseTypeSnowflake WarehouseType = "snowflake"
	WarehouseTypeBigQuery  WarehouseType = "bigquery"
)

// WarehouseConfig defines credentials, targets, and synchronization parameters for a workspace.
type WarehouseConfig struct {
	WorkspaceID         uuid.UUID     `json:"workspace_id"`
	Type                WarehouseType `json:"type"`
	Enabled             bool          `json:"enabled"`
	AutoSync            bool          `json:"auto_sync"`
	SyncIntervalMinutes int           `json:"sync_interval_minutes"` // e.g. 60, 360, 1440
	LastSyncAt          *time.Time    `json:"last_sync_at,omitempty"`
	LastWatermark       *time.Time    `json:"last_watermark,omitempty"`

	// Snowflake parameters
	SnowflakeAccount   string `json:"snowflake_account,omitempty"`   // e.g. "xy12345.us-east-1" or "org-account"
	SnowflakeWarehouse string `json:"snowflake_warehouse,omitempty"` // e.g. "COMPUTE_WH"
	SnowflakeDatabase  string `json:"snowflake_database,omitempty"`  // e.g. "SCANDRIX_ANALYTICS"
	SnowflakeSchema    string `json:"snowflake_schema,omitempty"`    // e.g. "PUBLIC"
	SnowflakeUsername  string `json:"snowflake_username,omitempty"`  // e.g. "SCANDRIX_INGEST"
	SnowflakePassword  string `json:"snowflake_password,omitempty"`  // Masked on read
	SnowflakeRole      string `json:"snowflake_role,omitempty"`      // e.g. "ACCOUNTADMIN" or "SYSADMIN"

	// BigQuery parameters
	BigQueryProjectID          string `json:"bigquery_project_id,omitempty"`          // e.g. "my-gcp-project"
	BigQueryDatasetID          string `json:"bigquery_dataset_id,omitempty"`          // e.g. "scandrix_bi"
	BigQueryServiceAccountJSON string `json:"bigquery_service_account_json,omitempty"` // Masked on read
	BigQueryLocation           string `json:"bigquery_location,omitempty"`           // e.g. "US", "EU"

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IngestionRun records the lifecycle and metrics of an ELT pipeline export job.
type IngestionRun struct {
	ID            uuid.UUID     `json:"id"`
	WorkspaceID   uuid.UUID     `json:"workspace_id"`
	WarehouseType WarehouseType `json:"warehouse_type"`
	Status        string        `json:"status"` // "PENDING", "RUNNING", "SUCCESS", "FAILED"
	PRCount       int           `json:"pr_count"`
	FindingsCount int           `json:"findings_count"`
	MetricsCount  int           `json:"metrics_count"`
	UsageCount    int           `json:"usage_count"`
	TotalRecords  int           `json:"total_records"`
	PayloadBytes  int64         `json:"payload_bytes"`
	ErrorMessage  string        `json:"error_message,omitempty"`
	DryRun        bool          `json:"dry_run"`
	StartedAt     time.Time     `json:"started_at"`
	CompletedAt   *time.Time    `json:"completed_at,omitempty"`
}

// PRRecord encapsulates an enterprise pull request metric row.
type PRRecord struct {
	ID                   string     `json:"id"`
	WorkspaceID          string     `json:"workspace_id"`
	RepoID               string     `json:"repo_id"`
	PRNumber             int        `json:"pr_number"`
	Title                string     `json:"title"`
	Author               string     `json:"author"`
	State                string     `json:"state"`
	Additions            int        `json:"additions"`
	Deletions            int        `json:"deletions"`
	ChangedFiles         int        `json:"changed_files"`
	CycleTimeSeconds     float64    `json:"cycle_time_seconds"`
	ReviewLatencySeconds float64    `json:"review_latency_seconds"`
	CreatedAt            time.Time  `json:"created_at"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
}

// FindingRecord encapsulates a detected code finding row for enterprise BI auditability.
type FindingRecord struct {
	ID              string    `json:"id"`
	WorkspaceID     string    `json:"workspace_id"`
	ReviewID        string    `json:"review_id"`
	RuleID          string    `json:"rule_id"`
	Category        string    `json:"category"`
	Severity        string    `json:"severity"`
	FilePath        string    `json:"file_path"`
	LineNumber      int       `json:"line_number"`
	Status          string    `json:"status"`
	ConfidenceScore float64   `json:"confidence_score"`
	DetectedAt      time.Time `json:"detected_at"`
}

// DORARecord encapsulates derived high-level deployment frequency and MTTR metrics.
type DORARecord struct {
	WorkspaceID          string    `json:"workspace_id"`
	Period               string    `json:"period"`
	DeploymentFrequency  string    `json:"deployment_frequency"`
	LeadTimeMinutes      float64   `json:"lead_time_minutes"`
	ChangeFailureRate    float64   `json:"change_failure_rate"`
	TimeToRestoreMinutes float64   `json:"time_to_restore_minutes"`
	Rating               string    `json:"rating"`
	ComputedAt           time.Time `json:"computed_at"`
}

// TokenUsageRecord encapsulates granular LLM token consumption and cost attribution rows.
type TokenUsageRecord struct {
	WorkspaceID      string    `json:"workspace_id"`
	Task             string    `json:"task"`
	Model            string    `json:"model"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	EstimatedCostUSD float64   `json:"estimated_cost_usd"`
	Timestamp        time.Time `json:"timestamp"`
}

// WarehouseBatch contains an aggregated batch of domain rows ready for ELT ingestion.
type WarehouseBatch struct {
	WorkspaceID uuid.UUID          `json:"workspace_id"`
	PRs         []PRRecord         `json:"prs"`
	Findings    []FindingRecord    `json:"findings"`
	DORAMetrics []DORARecord       `json:"dora_metrics"`
	TokenUsage  []TokenUsageRecord `json:"token_usage"`
	GeneratedAt time.Time          `json:"generated_at"`
}

// TotalRecords returns the aggregate count of all records in the batch.
func (b *WarehouseBatch) TotalRecords() int {
	if b == nil {
		return 0
	}
	return len(b.PRs) + len(b.Findings) + len(b.DORAMetrics) + len(b.TokenUsage)
}
