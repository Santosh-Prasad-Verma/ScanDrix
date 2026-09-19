// Package domain defines core entities, configuration metadata, and contracts for centralized repository configuration in ScanDrix.
package domain

import (
	"context"
	"io"
	"time"
)

// ConfigLevel indicates whether configuration is defined at org, team, or repository level.
type ConfigLevel string

const (
	ConfigLevelOrganization ConfigLevel = "organization"
	ConfigLevelTeam         ConfigLevel = "team"
	ConfigLevelRepository   ConfigLevel = "repository"
	ConfigLevelDirectory    ConfigLevel = "directory"
)

// RuleScope represents the scope of applicability for review rules.
type RuleScope string

const (
	RuleScopeGlobal     RuleScope = "global"
	RuleScopeRepository RuleScope = "repository"
	RuleScopeDirectory  RuleScope = "directory"
)

// RuleSeverity represents the violation level of a review rule.
type RuleSeverity string

const (
	RuleSeverityCritical RuleSeverity = "critical"
	RuleSeverityHigh     RuleSeverity = "high"
	RuleSeverityMedium   RuleSeverity = "medium"
	RuleSeverityLow      RuleSeverity = "low"
	RuleSeverityInfo     RuleSeverity = "info"
)

// ConfigFileMeta represents file metadata within the centralized configuration repository.
type ConfigFileMeta struct {
	Path           string      `json:"path"`
	Content        string      `json:"content"`
	SHA            string      `json:"sha,omitempty"`
	Level          ConfigLevel `json:"level"`
	RepositoryID   string      `json:"repositoryId,omitempty"`
	DirectoryPaths []string    `json:"directoryPaths,omitempty"`
	ModifiedAt     time.Time   `json:"modifiedAt,omitempty"`
}

// RuleFileMeta represents a custom review rule stored in the central configuration repository.
type RuleFileMeta struct {
	ID              string       `json:"id"`
	UUID            string       `json:"uuid,omitempty"`
	Title           string       `json:"title"`
	Description     string       `json:"description,omitempty"`
	Content         string       `json:"content,omitempty"`
	Prompt          string       `json:"prompt,omitempty"`
	Rule            string       `json:"rule,omitempty"`
	Path            string       `json:"path"`
	RepositoryID    string       `json:"repositoryId,omitempty"`
	DirectoryPaths  []string     `json:"directoryPaths,omitempty"`
	Severity        RuleSeverity `json:"severity"`
	Scope           RuleScope    `json:"scope"`
	Enabled         bool         `json:"enabled"`
	IsMemory        bool         `json:"isMemory,omitempty"`
	FilePatterns    []string     `json:"filePatterns,omitempty"`
	BadExamples     []string     `json:"badExamples,omitempty"`
	GoodExamples    []string     `json:"goodExamples,omitempty"`
	GroupFolderName string       `json:"groupFolderName,omitempty"`
	ModifiedAt      time.Time    `json:"modifiedAt,omitempty"`
}

// TreeItem represents a Git tree leaf item in a remote repository.
type TreeItem struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"` // "blob" or "tree"
	SHA  string `json:"sha"`
	Size int64  `json:"size,omitempty"`
	URL  string `json:"url,omitempty"`
}

// RepoRef identifies a repository by ID and name.
type RepoRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DirectoryGroupFolderRef represents an individual folder reference within a multi-path group.
type DirectoryGroupFolderRef struct {
	Path string `json:"path"`
}

// Action represents the type of file mutation.
type Action string

const (
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
)

// FileMutationOp specifies a file create/update/delete operation to commit in a PR.
type FileMutationOp struct {
	Action    Action `json:"action,omitempty"`
	Path      string `json:"path"`
	Operation string `json:"operation,omitempty"` // "upsert" or "delete"
	Content   string `json:"content,omitempty"`
}

// CustomMessageConfig represents PR comment messages configured at org, repo, or directory level.
type CustomMessageConfig struct {
	UUID             string      `json:"uuid"`
	OrganizationUUID string      `json:"organization_uuid"`
	TeamUUID         string      `json:"team_uuid"`
	RepositoryID     string      `json:"repository_id"`
	DirectoryPaths   []string    `json:"directory_paths,omitempty"`
	ConfigLevel      ConfigLevel `json:"config_level"`
	MessageType      string      `json:"message_type"`
	Content          string      `json:"content"`
	UpdatedAt        time.Time   `json:"updated_at"`
}

// ActorContext identifies the initiator of configuration updates.
type ActorContext struct {
	OrganizationID string `json:"organization_id"`
	Source         string `json:"source"` // "web", "sync", "cli"
	UserEmail      string `json:"user_email"`
	UserID         string `json:"user_id"`
}

// SyncResult details the outcome of synchronizing configuration files.
type SyncResult struct {
	Success     bool   `json:"success"`
	SyncedFiles int    `json:"synced_files"`
	Message     string `json:"message"`
}

// CentralizedPRRequest contains parameters for creating a mutation PR.
type CentralizedPRRequest struct {
	OrganizationID string           `json:"organization_id"`
	TeamID         string           `json:"team_id"`
	RepositoryID   string           `json:"repository_id"`
	BranchName     string           `json:"branch_name"`
	CommitMessage  string           `json:"commit_message"`
	Title          string           `json:"title"`
	Description    string           `json:"description"`
	Mutations      []FileMutationOp `json:"mutations"`
}

// InitRepoRequest specifies arguments for initializing the central repository.
type InitRepoRequest struct {
	OrganizationID      string `json:"organization_id"`
	TeamID              string `json:"team_id"`
	CentralRepositoryID string `json:"central_repository_id"`
	DefaultBranch       string `json:"default_branch"`
}

// InitRepoResult contains the result of initializing the central repository.
type InitRepoResult struct {
	Success bool   `json:"success"`
	PRURL   string `json:"pr_url,omitempty"`
	Message string `json:"message"`
}

// SyncRepoRequest specifies parameters for syncing remote repository configurations.
type SyncRepoRequest struct {
	OrganizationID      string `json:"organization_id"`
	TeamID              string `json:"team_id"`
	CentralRepositoryID string `json:"central_repository_id"`
}

// SyncRepoResult contains details about repository synchronization.
type SyncRepoResult struct {
	Success     bool   `json:"success"`
	SyncedFiles int    `json:"synced_files"`
	Message     string `json:"message"`
}

// CentralizedConfigStatus represents health and synchronization state of the central configuration.
type CentralizedConfigStatus struct {
	IsValid      bool      `json:"isValid"`
	LastSyncAt   time.Time `json:"lastSyncAt"`
	ErrorMessage string    `json:"errorMessage,omitempty"`
	ActiveRules  int       `json:"activeRules"`
	TotalFiles   int       `json:"totalFiles"`
}

// CentralizedConfigService defines operations for managing remote repository configuration trees.
type CentralizedConfigService interface {
	ValidateCentralizedConfig(ctx context.Context, orgID, teamID string) (*CentralizedConfigStatus, error)
	DownloadConfig(ctx context.Context, orgID, teamID string) ([]ConfigFileMeta, error)
	DownloadConfigZip(ctx context.Context, orgID, teamID string) (io.ReadCloser, error)
	InitCentralizedConfig(ctx context.Context, orgID, teamID, defaultBranch string) error
	SyncRepositoryConfig(ctx context.Context, orgID, teamID string) error
}

// CentralizedConfigPRService defines operations for creating PR mutations to the central repository.
type CentralizedConfigPRService interface {
	BuildCentralizedPath(repoFolder, relativePath string) string
	CreateMutationPR(ctx context.Context, orgID, teamID string, ops []FileMutationOp, title, description string) (string, error)
	CreateOrUpdatePR(ctx context.Context, req CentralizedPRRequest) (*InitRepoResult, error)
	ClearActivePullRequestMetadata(orgID, teamID string)
}

// CentralizedConfigActivePullRequest tracks an in-flight mutation PR on the central repository.
type CentralizedConfigActivePullRequest struct {
	PRURL        string    `json:"prUrl"`
	PRNumber     int       `json:"prNumber,omitempty"`
	SourceBranch string    `json:"sourceBranch"`
	TargetBranch string    `json:"targetBranch"`
	Repository   RepoRef   `json:"repository"`
	CreatedAt    time.Time `json:"createdAt,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt,omitempty"`
}

// CentralizedConfigParameter is the team parameter structure holding centralized repository configuration.
type CentralizedConfigParameter struct {
	Enabled           bool                                 `json:"enabled"`
	Repository        *RepoRef                             `json:"repository,omitempty"`
	ActivePullRequest *CentralizedConfigActivePullRequest `json:"activePullRequest,omitempty"`
}

