package domain

import (
	"time"

	"github.com/google/uuid"
)

// Tenant represents an organization or enterprise account.
type Tenant struct {
	ID        uuid.UUID      `json:"id" db:"id"`
	Slug      string         `json:"slug" db:"slug"`
	Name      string         `json:"name" db:"name"`
	Plan      TenantPlan     `json:"plan" db:"plan"`
	Status    TenantStatus   `json:"status" db:"status"`
	Settings  map[string]any `json:"settings" db:"settings"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt time.Time      `json:"updated_at" db:"updated_at"`
}

// User represents an authenticated team member.
type User struct {
	ID             uuid.UUID `json:"id" db:"id"`
	TenantID       uuid.UUID `json:"tenant_id" db:"tenant_id"`
	ExternalAuthID string    `json:"external_auth_id" db:"external_auth_id"`
	Email          string    `json:"email" db:"email"`
	DisplayName    string    `json:"display_name" db:"display_name"`
	Role           string    `json:"role" db:"role"`
	IsActive       bool      `json:"is_active" db:"is_active"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// APIKey represents scoped machine credentials.
type APIKey struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	TenantID        uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	CreatedByUserID *uuid.UUID `json:"created_by_user_id,omitempty" db:"created_by_user_id"`
	Name            string     `json:"name" db:"name"`
	KeyPrefix       string     `json:"key_prefix" db:"key_prefix"`
	KeyHash         string     `json:"-" db:"key_hash"`
	Scopes          []string   `json:"scopes" db:"scopes"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty" db:"last_used_at"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty" db:"revoked_at"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
}

// PolicyProfile defines security thresholds and governance rules.
type PolicyProfile struct {
	ID        uuid.UUID      `json:"id" db:"id"`
	TenantID  uuid.UUID      `json:"tenant_id" db:"tenant_id"`
	Name      string         `json:"name" db:"name"`
	IsDefault bool           `json:"is_default" db:"is_default"`
	Rules     map[string]any `json:"rules" db:"rules"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt time.Time      `json:"updated_at" db:"updated_at"`
}

// Project represents a codebase under analysis.
type Project struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	TenantID        uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	PolicyProfileID *uuid.UUID `json:"policy_profile_id,omitempty" db:"policy_profile_id"`
	Slug            string     `json:"slug" db:"slug"`
	Name            string     `json:"name" db:"name"`
	DefaultBranch   string     `json:"default_branch" db:"default_branch"`
	Version         int        `json:"version" db:"version"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// Repository represents a linked Git remote repository.
type Repository struct {
	ID                   uuid.UUID  `json:"id" db:"id"`
	TenantID             uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ProjectID            uuid.UUID  `json:"project_id" db:"project_id"`
	Provider             string     `json:"provider" db:"provider"`
	ExternalRepoID       *string    `json:"external_repo_id,omitempty" db:"external_repo_id"`
	CloneURL             string     `json:"clone_url" db:"clone_url"`
	DefaultBranch        string     `json:"default_branch" db:"default_branch"`
	LastIndexedCommitSHA *string    `json:"last_indexed_commit_sha,omitempty" db:"last_indexed_commit_sha"`
	LastIndexedAt        *time.Time `json:"last_indexed_at,omitempty" db:"last_indexed_at"`
	CreatedAt            time.Time  `json:"created_at" db:"created_at"`
}

// Commit represents an immutable Git snapshot.
type Commit struct {
	ID            uuid.UUID `json:"id" db:"id"`
	RepositoryID  uuid.UUID `json:"repository_id" db:"repository_id"`
	SHA           string    `json:"sha" db:"sha"`
	ParentSHAs    []string  `json:"parent_shas" db:"parent_shas"`
	AuthorName    string    `json:"author_name" db:"author_name"`
	AuthorEmail   string    `json:"author_email" db:"author_email"`
	CommitMessage string    `json:"commit_message" db:"commit_message"`
	CommittedAt   time.Time `json:"committed_at" db:"committed_at"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// CodeFile represents a source file in a commit snapshot.
type CodeFile struct {
	ID             uuid.UUID `json:"id" db:"id"`
	RepositoryID   uuid.UUID `json:"repository_id" db:"repository_id"`
	CommitID       uuid.UUID `json:"commit_id" db:"commit_id"`
	FilePath       string    `json:"file_path" db:"file_path"`
	Language       string    `json:"language" db:"language"`
	SizeBytes      int64     `json:"size_bytes" db:"size_bytes"`
	ContentHash    string    `json:"content_hash" db:"content_hash"`
	ObjectStoreURI string    `json:"object_store_uri" db:"object_store_uri"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// CodeSymbol represents an AST symbol (function, class, route, etc.).
type CodeSymbol struct {
	ID            uuid.UUID `json:"id" db:"id"`
	CodeFileID    uuid.UUID `json:"code_file_id" db:"code_file_id"`
	SymbolKey     string    `json:"symbol_key" db:"symbol_key"`
	Name          string    `json:"name" db:"name"`
	Kind          string    `json:"kind" db:"kind"`
	StartLine     int       `json:"start_line" db:"start_line"`
	EndLine       int       `json:"end_line" db:"end_line"`
	StartColumn   int       `json:"start_column" db:"start_column"`
	EndColumn     int       `json:"end_column" db:"end_column"`
	Signature     *string   `json:"signature,omitempty" db:"signature"`
	Visibility    string    `json:"visibility" db:"visibility"`
	TaintedInputs []string  `json:"tainted_inputs" db:"tainted_inputs"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// CodeEdge represents a relationship in the code intelligence graph.
type CodeEdge struct {
	ID           uuid.UUID `json:"id" db:"id"`
	CommitID     uuid.UUID `json:"commit_id" db:"commit_id"`
	FromSymbolID uuid.UUID `json:"from_symbol_id" db:"from_symbol_id"`
	ToSymbolID   uuid.UUID `json:"to_symbol_id" db:"to_symbol_id"`
	EdgeType     string    `json:"edge_type" db:"edge_type"`
	Confidence   float64   `json:"confidence" db:"confidence"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// Scan represents an audit or verification run.
type Scan struct {
	ID                uuid.UUID   `json:"id" db:"id"`
	TenantID          uuid.UUID   `json:"tenant_id" db:"tenant_id"`
	ProjectID         uuid.UUID   `json:"project_id" db:"project_id"`
	RepositoryID      uuid.UUID   `json:"repository_id" db:"repository_id"`
	CommitID          uuid.UUID   `json:"commit_id" db:"commit_id"`
	ScanType          ScanType    `json:"scan_type" db:"scan_type"`
	Status            ScanStatus  `json:"status" db:"status"`
	HealthScore       *int        `json:"health_score,omitempty" db:"health_score"`
	InitiatedByUserID *uuid.UUID  `json:"initiated_by_user_id,omitempty" db:"initiated_by_user_id"`
	WorkflowID        string      `json:"workflow_id" db:"workflow_id"`
	ErrorMessage      *string     `json:"error_message,omitempty" db:"error_message"`
	StartedAt         *time.Time  `json:"started_at,omitempty" db:"started_at"`
	CompletedAt       *time.Time  `json:"completed_at,omitempty" db:"completed_at"`
	CreatedAt         time.Time   `json:"created_at" db:"created_at"`
}

// Finding represents a deduplicated security finding or logic flaw.
type Finding struct {
	ID                uuid.UUID       `json:"id" db:"id"`
	TenantID          uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	ProjectID         uuid.UUID       `json:"project_id" db:"project_id"`
	CanonicalKey      string          `json:"canonical_key" db:"canonical_key"`
	Category          FindingCategory `json:"category" db:"category"`
	Severity          FindingSeverity `json:"severity" db:"severity"`
	State             FindingState    `json:"state" db:"state"`
	Confidence        float64         `json:"confidence" db:"confidence"`
	BlastRadiusScore  *int            `json:"blast_radius_score,omitempty" db:"blast_radius_score"`
	Title             string          `json:"title" db:"title"`
	Description       string          `json:"description" db:"description"`
	PrimaryFile       string          `json:"primary_file" db:"primary_file"`
	PrimaryLine       int             `json:"primary_line" db:"primary_line"`
	CWEID             *string         `json:"cwe_id,omitempty" db:"cwe_id"`
	CVEID             *string         `json:"cve_id,omitempty" db:"cve_id"`
	FirstSeenScanID   uuid.UUID       `json:"first_seen_scan_id" db:"first_seen_scan_id"`
	LastSeenScanID    uuid.UUID       `json:"last_seen_scan_id" db:"last_seen_scan_id"`
	CreatedAt         time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at" db:"updated_at"`
}

// FindingOccurrence represents a range-partitioned occurrence of a finding in a specific commit.
type FindingOccurrence struct {
	ID              uuid.UUID `json:"id" db:"id"`
	FindingID       uuid.UUID `json:"finding_id" db:"finding_id"`
	ScanID          uuid.UUID `json:"scan_id" db:"scan_id"`
	CommitSHA       string    `json:"commit_sha" db:"commit_sha"`
	FilePath        string    `json:"file_path" db:"file_path"`
	StartLine       int       `json:"start_line" db:"start_line"`
	EndLine         int       `json:"end_line" db:"end_line"`
	CodeSnippetHash string    `json:"code_snippet_hash" db:"code_snippet_hash"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

// Evidence represents verifiable proof attached to a finding.
type Evidence struct {
	ID             uuid.UUID      `json:"id" db:"id"`
	FindingID      uuid.UUID      `json:"finding_id" db:"finding_id"`
	EvidenceType   string         `json:"evidence_type" db:"evidence_type"`
	SourceAnalyzer string         `json:"source_analyzer" db:"source_analyzer"`
	Strength       string         `json:"strength" db:"strength"`
	Summary        string         `json:"summary" db:"summary"`
	Payload        map[string]any `json:"payload" db:"payload"`
	ArtifactURI    *string        `json:"artifact_uri,omitempty" db:"artifact_uri"`
	CreatedAt      time.Time      `json:"created_at" db:"created_at"`
}

// Patch represents an automated fix proposal.
type Patch struct {
	ID               uuid.UUID   `json:"id" db:"id"`
	FindingID        uuid.UUID   `json:"finding_id" db:"finding_id"`
	ScanID           uuid.UUID   `json:"scan_id" db:"scan_id"`
	Status           PatchStatus `json:"status" db:"status"`
	UnifiedDiff      string      `json:"unified_diff" db:"unified_diff"`
	DiffHash         string      `json:"diff_hash" db:"diff_hash"`
	MutationScore    *float64    `json:"mutation_score,omitempty" db:"mutation_score"`
	PRURL            *string     `json:"pr_url,omitempty" db:"pr_url"`
	PRNumber         *int        `json:"pr_number,omitempty" db:"pr_number"`
	AppliedByUserID  *uuid.UUID  `json:"applied_by_user_id,omitempty" db:"applied_by_user_id"`
	CreatedAt        time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at" db:"updated_at"`
}

// Execution represents an isolated sandbox or worker run.
type Execution struct {
	ID               uuid.UUID       `json:"id" db:"id"`
	TenantID         uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	ProjectID        uuid.UUID       `json:"project_id" db:"project_id"`
	ScanID           uuid.UUID       `json:"scan_id" db:"scan_id"`
	ExecutionType    ExecutionType   `json:"execution_type" db:"execution_type"`
	SandboxTier      SandboxTier     `json:"sandbox_tier" db:"sandbox_tier"`
	Status           ExecutionStatus `json:"status" db:"status"`
	ExitCode         *int            `json:"exit_code,omitempty" db:"exit_code"`
	DurationMS       *int64          `json:"duration_ms,omitempty" db:"duration_ms"`
	CPUUsageSeconds  *float64        `json:"cpu_usage_seconds,omitempty" db:"cpu_usage_seconds"`
	PeakMemoryBytes  *int64          `json:"peak_memory_bytes,omitempty" db:"peak_memory_bytes"`
	LogArtifactURI   *string         `json:"log_artifact_uri,omitempty" db:"log_artifact_uri"`
	StartedAt        *time.Time      `json:"started_at,omitempty" db:"started_at"`
	CompletedAt      *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
	CreatedAt        time.Time       `json:"created_at" db:"created_at"`
}

// TestRun represents an individual test execution in a sandbox.
type TestRun struct {
	ID            uuid.UUID `json:"id" db:"id"`
	ExecutionID   uuid.UUID `json:"execution_id" db:"execution_id"`
	TestName      string    `json:"test_name" db:"test_name"`
	Status        string    `json:"status" db:"status"`
	DurationMS    int       `json:"duration_ms" db:"duration_ms"`
	Stdout        string    `json:"stdout" db:"stdout"`
	Stderr        string    `json:"stderr" db:"stderr"`
	MutationScore *float64  `json:"mutation_score,omitempty" db:"mutation_score"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// ModelRun tracks LLM token accounting, model selection, and costs.
type ModelRun struct {
	ID               uuid.UUID `json:"id" db:"id"`
	TenantID         uuid.UUID `json:"tenant_id" db:"tenant_id"`
	ProjectID        uuid.UUID `json:"project_id" db:"project_id"`
	ScanID           uuid.UUID `json:"scan_id" db:"scan_id"`
	Role             string    `json:"role" db:"role"`
	Provider         string    `json:"provider" db:"provider"`
	ModelName        string    `json:"model_name" db:"model_name"`
	PromptTokens     int       `json:"prompt_tokens" db:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens" db:"completion_tokens"`
	TotalCostUSD     float64   `json:"total_cost_usd" db:"total_cost_usd"`
	LatencyMS        int       `json:"latency_ms" db:"latency_ms"`
	Status           string    `json:"status" db:"status"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
}

// AuthorizationGrant represents domain/target authorization proof.
type AuthorizationGrant struct {
	ID                  uuid.UUID  `json:"id" db:"id"`
	TenantID            uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	ProjectID           uuid.UUID  `json:"project_id" db:"project_id"`
	TargetBaseURL       string     `json:"target_base_url" db:"target_base_url"`
	ProofMethod         string     `json:"proof_method" db:"proof_method"`
	ChallengeRecordName string     `json:"challenge_record_name" db:"challenge_record_name"`
	ExpectedTokenHash   string     `json:"-" db:"expected_token_hash"`
	IsVerified          bool       `json:"is_verified" db:"is_verified"`
	VerifiedAt          *time.Time `json:"verified_at,omitempty" db:"verified_at"`
	ValidUntil          time.Time  `json:"valid_until" db:"valid_until"`
	ApprovedByUserID    *uuid.UUID `json:"approved_by_user_id,omitempty" db:"approved_by_user_id"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
}

// TestTarget represents dynamic load and DAST test limits.
type TestTarget struct {
	ID                   uuid.UUID `json:"id" db:"id"`
	ProjectID            uuid.UUID `json:"project_id" db:"project_id"`
	AuthorizationGrantID uuid.UUID `json:"authorization_grant_id" db:"authorization_grant_id"`
	Environment          string    `json:"environment" db:"environment"`
	BaseURL              string    `json:"base_url" db:"base_url"`
	MaxVUCeiling         int       `json:"max_vu_ceiling" db:"max_vu_ceiling"`
	MaxDurationSeconds   int       `json:"max_duration_seconds" db:"max_duration_seconds"`
	ScopePathAllowlist   []string  `json:"scope_path_allowlist" db:"scope_path_allowlist"`
	HeadersVaultRef      *string   `json:"headers_vault_ref,omitempty" db:"headers_vault_ref"`
	CreatedAt            time.Time `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time `json:"updated_at" db:"updated_at"`
}

// Artifact represents an S3 stored file (SARIF, SBOM, JUnit).
type Artifact struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	ExecutionID  *uuid.UUID `json:"execution_id,omitempty" db:"execution_id"`
	ScanID       *uuid.UUID `json:"scan_id,omitempty" db:"scan_id"`
	ArtifactType string     `json:"artifact_type" db:"artifact_type"`
	S3URI        string     `json:"s3_uri" db:"s3_uri"`
	SizeBytes    int64      `json:"size_bytes" db:"size_bytes"`
	Checksum     string     `json:"checksum" db:"checksum"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
}

// IdempotencyKey prevents duplicate processing of webhooks and API calls.
type IdempotencyKey struct {
	ID                 uuid.UUID      `json:"id" db:"id"`
	TenantID           uuid.UUID      `json:"tenant_id" db:"tenant_id"`
	KeyHash            string         `json:"key_hash" db:"key_hash"`
	Operation          string         `json:"operation" db:"operation"`
	RequestPayloadHash string         `json:"request_payload_hash" db:"request_payload_hash"`
	ResponseBody       map[string]any `json:"response_body,omitempty" db:"response_body"`
	StatusCode         *int           `json:"status_code,omitempty" db:"status_code"`
	LockedUntil        time.Time      `json:"locked_until" db:"locked_until"`
	ExpiresAt          time.Time      `json:"expires_at" db:"expires_at"`
	CreatedAt          time.Time      `json:"created_at" db:"created_at"`
}

// AuditEvent represents an append-only compliance trail.
type AuditEvent struct {
	ID           uuid.UUID      `json:"id" db:"id"`
	TenantID     uuid.UUID      `json:"tenant_id" db:"tenant_id"`
	UserID       *uuid.UUID     `json:"user_id,omitempty" db:"user_id"`
	ActorType    string         `json:"actor_type" db:"actor_type"`
	Action       string         `json:"action" db:"action"`
	ResourceType string         `json:"resource_type" db:"resource_type"`
	ResourceID   uuid.UUID      `json:"resource_id" db:"resource_id"`
	IPAddress    *string        `json:"ip_address,omitempty" db:"ip_address"`
	UserAgent    *string        `json:"user_agent,omitempty" db:"user_agent"`
	Payload      map[string]any `json:"payload" db:"payload"`
	CreatedAt    time.Time      `json:"created_at" db:"created_at"`
}
