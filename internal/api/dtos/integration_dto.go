package dtos

import (
	"time"

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
