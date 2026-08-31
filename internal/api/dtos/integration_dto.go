package dtos

import (
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/pkg/models"
)

// ConnectSCMRequest supplies credentials to connect an SCM host.
type ConnectSCMRequest struct {
	Provider      models.SCMProvider `json:"provider"`
	BaseURL       string             `json:"base_url,omitempty"`
	AccessToken   string             `json:"access_token"`
	WebhookSecret string             `json:"webhook_secret"`
}

// IntegrationStatusResponse reports connection status of external providers.
type IntegrationStatusResponse struct {
	Provider     models.SCMProvider `json:"provider"`
	IsConnected  bool               `json:"is_connected"`
	AccountName  string             `json:"account_name"`
	RepoCount    int                `json:"repo_count"`
	LastSyncedAt *time.Time         `json:"last_synced_at,omitempty"`
}

// TestConnectionResponse reports external API connectivity check results.
type TestConnectionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	User    string `json:"user,omitempty"`
}

// ConnectPMRequest supplies credentials to connect Jira, Linear, or Azure Boards.
type ConnectPMRequest struct {
	Platform     string `json:"platform"` // "jira", "linear", "azureboards"
	BaseURL      string `json:"base_url,omitempty"`
	APIToken     string `json:"api_token"`
	Email        string `json:"email,omitempty"`        // Jira email
	Organization string `json:"organization,omitempty"` // Azure Boards organization
}

// ExportFindingToPMRequest triggers manual issue creation for a specific code finding.
type ExportFindingToPMRequest struct {
	FindingID      uuid.UUID `json:"finding_id"`
	Platform       string    `json:"platform"`
	ProjectKey     string    `json:"project_key"`
	IssueType      string    `json:"issue_type,omitempty"`
	PullRequestURL string    `json:"pull_request_url,omitempty"`
}

// PMAutoTicketConfigRequest configures automated ticket generation for a repository.
type PMAutoTicketConfigRequest struct {
	RepositoryID uuid.UUID `json:"repository_id"`
	Enabled      bool      `json:"enabled"`
	Platform     string    `json:"platform"`
	ProjectKey   string    `json:"project_key"`
	IssueType    string    `json:"issue_type"`
	MinSeverity  string    `json:"min_severity"`
}

// PMTicketResponse details an external ticket linked to a finding.
type PMTicketResponse struct {
	ID        uuid.UUID `json:"id"`
	FindingID uuid.UUID `json:"finding_id"`
	Platform  string    `json:"platform"`
	TicketKey string    `json:"ticket_key"`
	TicketURL string    `json:"ticket_url"`
	CreatedAt time.Time `json:"created_at"`
}
