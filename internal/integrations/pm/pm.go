package pm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
)

// PMAdapter defines the unified contract across Jira, Linear, and Azure Boards.
type PMAdapter interface {
	Platform() PMPlatform
	CreateIssue(ctx context.Context, req IssueCreationRequest) (*PMIssue, error)
	GetIssue(ctx context.Context, issueKey string) (*PMIssue, error)
	AddComment(ctx context.Context, issueKey, comment string) error
	LinkPR(ctx context.Context, issueKey, prURL string) error
}

// NewAdapterFromConfig instantiates the appropriate PM adapter based on platform credentials.
func NewAdapterFromConfig(cfg PMConfig) (PMAdapter, error) {
	switch cfg.Platform {
	case PlatformJira:
		return NewJiraAdapter(cfg.BaseURL, cfg.Email, cfg.APIToken), nil
	case PlatformLinear:
		return NewLinearAdapter(cfg.APIToken), nil
	case PlatformAzureBoards:
		return NewAzureBoardsAdapter(cfg.BaseURL, cfg.Organization, cfg.APIToken), nil
	default:
		return nil, fmt.Errorf("unsupported PM platform: %s", cfg.Platform)
	}
}

// PMDispatcher coordinates ticket creation and synchronization across teams and workspaces.
type PMDispatcher struct {
	mu       sync.RWMutex
	adapters map[string]PMAdapter // key: wsID:platform
}

// NewPMDispatcher initializes the dispatcher registry.
func NewPMDispatcher() *PMDispatcher {
	return &PMDispatcher{
		adapters: make(map[string]PMAdapter),
	}
}

// RegisterAdapter associates an active PM adapter with a workspace.
func (d *PMDispatcher) RegisterAdapter(wsID uuid.UUID, adapter PMAdapter) {
	d.mu.Lock()
	defer d.mu.Unlock()
	key := fmt.Sprintf("%s:%s", wsID.String(), adapter.Platform())
	d.adapters[key] = adapter
}

// GetAdapter retrieves the adapter for a given workspace and platform.
func (d *PMDispatcher) GetAdapter(wsID uuid.UUID, platform PMPlatform) (PMAdapter, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", wsID.String(), platform)
	adapter, exists := d.adapters[key]
	if !exists {
		return nil, fmt.Errorf("no %s integration configured for workspace %s", platform, wsID)
	}
	return adapter, nil
}

// ExportFinding formats and dispatches a finding to the target project management system.
func (d *PMDispatcher) ExportFinding(
	ctx context.Context,
	wsID uuid.UUID,
	platform PMPlatform,
	projectKey string,
	finding models.CodeFinding,
	prURL string,
) (*PMIssue, error) {
	adapter, err := d.GetAdapter(wsID, platform)
	if err != nil {
		return nil, err
	}

	title := fmt.Sprintf("[ScanDrix] %s in %s:%d", finding.Title, finding.FilePath, finding.StartLine)
	desc := FormatIssueDescription(finding, prURL)

	req := IssueCreationRequest{
		WorkspaceID:    wsID,
		FindingID:      finding.ID,
		ProjectKey:     projectKey,
		IssueType:      "Bug",
		Title:          title,
		Description:    desc,
		Severity:       finding.Severity,
		Category:       finding.Category,
		FilePath:       finding.FilePath,
		Line:           finding.StartLine,
		Remediation:    finding.Remediation,
		PullRequestURL: prURL,
	}

	issue, err := adapter.CreateIssue(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed exporting finding to %s: %w", platform, err)
	}

	// Link pull request if URL provided
	if prURL != "" {
		_ = adapter.LinkPR(ctx, issue.Key, prURL)
	}

	return issue, nil
}

// AutoTicketManager coordinates automated ticketing rules from code findings.
type AutoTicketManager struct {
	repo       *database.Repository
	dispatcher *PMDispatcher
}

// NewAutoTicketManager initializes an auto-ticket manager with database access.
func NewAutoTicketManager(repo *database.Repository, dispatcher *PMDispatcher) *AutoTicketManager {
	if dispatcher == nil {
		dispatcher = NewPMDispatcher()
	}
	return &AutoTicketManager{
		repo:       repo,
		dispatcher: dispatcher,
	}
}

// ProcessFindingsForAutoTicket evaluates discovered findings against repository ticketing policies.
func (m *AutoTicketManager) ProcessFindingsForAutoTicket(
	ctx context.Context,
	wsID, repoID uuid.UUID,
	findings []models.CodeFinding,
	prURL string,
) ([]*PMIssue, error) {
	if m.repo == nil || len(findings) == 0 {
		return nil, nil
	}

	config, err := m.repo.GetPMAutoTicketConfig(ctx, wsID, repoID)
	if err != nil || config == nil || !config.Enabled {
		return nil, nil // Auto-ticketing disabled for this repo
	}

	targetPlatform := PMPlatform(strings.ToLower(config.Platform))
	adapter, err := m.dispatcher.GetAdapter(wsID, targetPlatform)
	if err != nil {
		slog.Debug("No cached PM adapter for workspace auto-ticketing, checking database", "platform", targetPlatform, "workspace_id", wsID)
		// Query integration connections for real encrypted token
		conns, connErr := m.repo.ListIntegrationConnections(ctx, wsID)
		if connErr == nil {
			for _, c := range conns {
				if strings.EqualFold(string(c.Provider), string(targetPlatform)) && c.IsConnected && c.AccessTokenEnc != "" {
					cfg := PMConfig{
						Platform: targetPlatform,
						APIToken: c.AccessTokenEnc,
						BaseURL:  "https://api.atlassian.com", // Default for Jira Cloud or configured URL
					}
					if targetPlatform == PlatformLinear {
						cfg.BaseURL = "https://api.linear.app/graphql"
					} else if targetPlatform == PlatformAzureBoards {
						cfg.BaseURL = "https://dev.azure.com"
					}
					if a, err := NewAdapterFromConfig(cfg); err == nil {
						m.dispatcher.RegisterAdapter(wsID, a)
						adapter = a
						break
					}
				}
			}
		}
	}

	if adapter == nil {
		slog.Warn("Auto-ticket configured but target PM integration is not active or token missing",
			"workspace_id", wsID, "platform", targetPlatform)
		return nil, nil
	}

	var createdIssues []*PMIssue
	minRank := severityRank(models.FindingSeverity(config.MinSeverity))

	for _, f := range findings {
		if severityRank(f.Severity) < minRank {
			continue // Below minimum severity threshold
		}

		// Prevent duplicate ticket creation for the same finding
		existingTickets, _ := m.repo.GetTicketsForFinding(ctx, wsID, f.ID)
		if len(existingTickets) > 0 {
			continue
		}

		issue, err := m.dispatcher.ExportFinding(ctx, wsID, targetPlatform, config.ProjectKey, f, prURL)
		if err != nil {
			slog.Warn("Failed creating automated PM ticket for finding", "finding_id", f.ID, "error", err)
			continue
		}

		// Persist ticket linkage in database
		if err := m.repo.InsertFindingTicket(ctx, wsID, f.ID, string(targetPlatform), issue.Key, issue.URL); err != nil {
			slog.Error("Failed recording finding ticket in database", "finding_id", f.ID, "ticket", issue.Key, "error", err)
		}

		// Emit outbox event
		payload, _ := json.Marshal(map[string]any{
			"workspace_id":  wsID.String(),
			"repository_id": repoID.String(),
			"finding_id":    f.ID.String(),
			"platform":      string(targetPlatform),
			"ticket_key":    issue.Key,
			"ticket_url":    issue.URL,
			"severity":      string(f.Severity),
			"created_at":    time.Now().UTC().Format(time.RFC3339),
		})
		outbox := &models.OutboxRecord{
			ID:          uuid.New(),
			WorkspaceID: wsID,
			EventType:   "pm.ticket.created",
			Payload:     payload,
			Status:      models.OutboxPending,
			CreatedAt:   time.Now().UTC(),
		}
		_ = m.repo.InsertOutboxEvent(ctx, outbox)

		createdIssues = append(createdIssues, issue)
		slog.Info("Automated PM ticket created from security finding",
			"finding_id", f.ID,
			"ticket_key", issue.Key,
			"platform", targetPlatform,
		)
	}

	return createdIssues, nil
}

func severityRank(s models.FindingSeverity) int {
	switch strings.ToUpper(string(s)) {
	case "CRITICAL":
		return 5
	case "HIGH":
		return 4
	case "MEDIUM":
		return 3
	case "LOW":
		return 2
	case "INFO":
		return 1
	default:
		return 0
	}
}

// FormatIssueDescription constructs a comprehensive markdown defect description.
func FormatIssueDescription(f models.CodeFinding, prURL string) string {
	desc := fmt.Sprintf("### 🛡️ ScanDrix Security Vulnerability Report\n\n"+
		"**Severity:** %s\n"+
		"**Category:** %s\n"+
		"**Location:** `%s:%d`\n\n"+
		"#### Problem Description\n%s\n\n",
		f.Severity, f.Category, f.FilePath, f.StartLine, f.Description)

	if f.Remediation != "" {
		desc += fmt.Sprintf("#### Recommended Remediation\n%s\n\n", f.Remediation)
	}

	if prURL != "" {
		desc += fmt.Sprintf("**Detected in Pull Request:** [%s](%s)\n", prURL, prURL)
	}

	desc += "---\n*Automated issue created by ScanDrix Enterprise Code Assurance.*"
	return desc
}
