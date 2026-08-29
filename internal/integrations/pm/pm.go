package pm

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
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
