package pm

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// PMPlatform enumerates supported project management tracking tools.
type PMPlatform string

const (
	PlatformJira        PMPlatform = "jira"
	PlatformLinear      PMPlatform = "linear"
	PlatformAzureBoards PMPlatform = "azureboards"
)

// PMIssue represents a tracked ticket in an external project management system.
type PMIssue struct {
	Key       string     `json:"key"` // e.g. "SEC-102", "ENG-404", "AB#1234"
	ID        string     `json:"id"`
	Platform  PMPlatform `json:"platform"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	URL       string     `json:"url"`
	Priority  string     `json:"priority,omitempty"`
	Assignee  string     `json:"assignee,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// IssueCreationRequest defines the payload to export a finding to Jira/Linear/Boards.
type IssueCreationRequest struct {
	WorkspaceID    uuid.UUID              `json:"workspace_id"`
	FindingID      uuid.UUID              `json:"finding_id"`
	ProjectKey     string                 `json:"project_key"` // e.g. "SEC" or Linear team ID
	IssueType      string                 `json:"issue_type"`  // "Bug", "Security", "Task"
	Title          string                 `json:"title"`
	Description    string                 `json:"description"`
	Severity       models.FindingSeverity `json:"severity"`
	Category       string                 `json:"category"`
	FilePath       string                 `json:"file_path"`
	Line           int                    `json:"line"`
	Remediation    string                 `json:"remediation"`
	PullRequestURL string                 `json:"pull_request_url,omitempty"`
}

// PMConfig holds authentication credentials for project management backends.
type PMConfig struct {
	Platform     PMPlatform `json:"platform"`
	BaseURL      string     `json:"base_url"`
	APIToken     string     `json:"api_token"`
	Email        string     `json:"email,omitempty"`        // Jira basic auth email
	Organization string     `json:"organization,omitempty"` // Azure Boards organization
}
