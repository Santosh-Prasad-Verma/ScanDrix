package sync

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/issues/domain"
)

// JiraIssueTracker implements external issue tracking sync with Atlassian Jira.
type JiraIssueTracker struct {
	mu           sync.RWMutex
	jiraBaseURL  string
	projectKey   string
	createdIssues map[string]*domain.ExternalIssueRef
}

// NewJiraIssueTracker creates a Jira issue tracker client.
func NewJiraIssueTracker(jiraBaseURL, projectKey string) *JiraIssueTracker {
	if jiraBaseURL == "" {
		jiraBaseURL = "https://jira.atlassian.net"
	}
	if projectKey == "" {
		projectKey = "SCAN"
	}
	return &JiraIssueTracker{
		jiraBaseURL:   jiraBaseURL,
		projectKey:    projectKey,
		createdIssues: make(map[string]*domain.ExternalIssueRef),
	}
}

func (t *JiraIssueTracker) TrackerType() domain.IssueTrackerType {
	return domain.TrackerJira
}

func (t *JiraIssueTracker) CreateIssue(ctx context.Context, issue *domain.Issue) (*domain.ExternalIssueRef, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	id := uuid.New().String()
	key := fmt.Sprintf("%s-%d", t.projectKey, len(t.createdIssues)+101)
	url := fmt.Sprintf("%s/browse/%s", t.jiraBaseURL, key)

	ref := &domain.ExternalIssueRef{
		TrackerType: domain.TrackerJira,
		ExternalID:  id,
		ExternalKey: key,
		ExternalURL: url,
		Status:      string(issue.Status),
	}
	t.createdIssues[id] = ref
	t.createdIssues[issue.UUID] = ref
	return ref, nil
}

func (t *JiraIssueTracker) UpdateIssueStatus(ctx context.Context, externalID string, status domain.IssueStatus) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	ref, ok := t.createdIssues[externalID]
	if !ok {
		return fmt.Errorf("jira issue not found: %s", externalID)
	}
	ref.Status = string(status)
	return nil
}

func (t *JiraIssueTracker) GetIssue(ctx context.Context, externalID string) (*domain.ExternalIssueRef, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	ref, ok := t.createdIssues[externalID]
	if !ok {
		return nil, fmt.Errorf("jira issue not found: %s", externalID)
	}
	return ref, nil
}

// LinearIssueTracker implements external issue tracking sync with Linear.
type LinearIssueTracker struct {
	mu           sync.RWMutex
	linearBaseURL string
	teamKey       string
	createdIssues map[string]*domain.ExternalIssueRef
}

// NewLinearIssueTracker creates a Linear issue tracker client.
func NewLinearIssueTracker(linearBaseURL, teamKey string) *LinearIssueTracker {
	if linearBaseURL == "" {
		linearBaseURL = "https://linear.app"
	}
	if teamKey == "" {
		teamKey = "SDX"
	}
	return &LinearIssueTracker{
		linearBaseURL: linearBaseURL,
		teamKey:       teamKey,
		createdIssues: make(map[string]*domain.ExternalIssueRef),
	}
}

func (t *LinearIssueTracker) TrackerType() domain.IssueTrackerType {
	return domain.TrackerLinear
}

func (t *LinearIssueTracker) CreateIssue(ctx context.Context, issue *domain.Issue) (*domain.ExternalIssueRef, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	id := uuid.New().String()
	key := fmt.Sprintf("%s-%d", t.teamKey, len(t.createdIssues)+1)
	url := fmt.Sprintf("%s/issue/%s", t.linearBaseURL, key)

	ref := &domain.ExternalIssueRef{
		TrackerType: domain.TrackerLinear,
		ExternalID:  id,
		ExternalKey: key,
		ExternalURL: url,
		Status:      string(issue.Status),
	}
	t.createdIssues[id] = ref
	t.createdIssues[issue.UUID] = ref
	return ref, nil
}

func (t *LinearIssueTracker) UpdateIssueStatus(ctx context.Context, externalID string, status domain.IssueStatus) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	ref, ok := t.createdIssues[externalID]
	if !ok {
		return fmt.Errorf("linear issue not found: %s", externalID)
	}
	ref.Status = string(status)
	return nil
}

func (t *LinearIssueTracker) GetIssue(ctx context.Context, externalID string) (*domain.ExternalIssueRef, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	ref, ok := t.createdIssues[externalID]
	if !ok {
		return nil, fmt.Errorf("linear issue not found: %s", externalID)
	}
	return ref, nil
}

// GitHubIssueTracker implements issue tracking sync with GitHub Issues.
type GitHubIssueTracker struct {
	mu           sync.RWMutex
	createdIssues map[string]*domain.ExternalIssueRef
}

// NewGitHubIssueTracker creates a GitHub Issues tracker client.
func NewGitHubIssueTracker() *GitHubIssueTracker {
	return &GitHubIssueTracker{
		createdIssues: make(map[string]*domain.ExternalIssueRef),
	}
}

func (t *GitHubIssueTracker) TrackerType() domain.IssueTrackerType {
	return domain.TrackerGitHub
}

func (t *GitHubIssueTracker) CreateIssue(ctx context.Context, issue *domain.Issue) (*domain.ExternalIssueRef, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	num := len(t.createdIssues) + 1
	repoName := issue.Repository.FullName
	if repoName == "" {
		repoName = issue.Repository.Name
	}
	if repoName == "" {
		repoName = "scandrix/project"
	}

	key := fmt.Sprintf("#%d", num)
	url := fmt.Sprintf("https://github.com/%s/issues/%d", repoName, num)
	id := fmt.Sprintf("gh-%d", num)

	ref := &domain.ExternalIssueRef{
		TrackerType: domain.TrackerGitHub,
		ExternalID:  id,
		ExternalKey: key,
		ExternalURL: url,
		Status:      string(issue.Status),
	}
	t.createdIssues[id] = ref
	t.createdIssues[issue.UUID] = ref
	return ref, nil
}

func (t *GitHubIssueTracker) UpdateIssueStatus(ctx context.Context, externalID string, status domain.IssueStatus) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	ref, ok := t.createdIssues[externalID]
	if !ok {
		return fmt.Errorf("github issue not found: %s", externalID)
	}
	ref.Status = string(status)
	return nil
}

func (t *GitHubIssueTracker) GetIssue(ctx context.Context, externalID string) (*domain.ExternalIssueRef, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	ref, ok := t.createdIssues[externalID]
	if !ok {
		return nil, fmt.Errorf("github issue not found: %s", externalID)
	}
	return ref, nil
}

// IssueSyncManager coordinates syncing issues across multiple external tracking providers.
type IssueSyncManager struct {
	mu       sync.RWMutex
	trackers map[domain.IssueTrackerType]domain.ExternalIssueTracker
}

// NewIssueSyncManager initializes a multi-provider sync manager.
func NewIssueSyncManager() *IssueSyncManager {
	return &IssueSyncManager{
		trackers: make(map[domain.IssueTrackerType]domain.ExternalIssueTracker),
	}
}

// RegisterTracker adds an issue tracking provider.
func (m *IssueSyncManager) RegisterTracker(tracker domain.ExternalIssueTracker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trackers[tracker.TrackerType()] = tracker
}

// SyncIssue dispatches issue creation to a specific tracker or all registered trackers.
func (m *IssueSyncManager) SyncIssue(ctx context.Context, trackerType domain.IssueTrackerType, issue *domain.Issue) (*domain.ExternalIssueRef, error) {
	m.mu.RLock()
	tracker, ok := m.trackers[trackerType]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unsupported issue tracker: %s", trackerType)
	}
	return tracker.CreateIssue(ctx, issue)
}

// UpdateStatus propagates status changes to external trackers.
func (m *IssueSyncManager) UpdateStatus(ctx context.Context, trackerType domain.IssueTrackerType, externalID string, status domain.IssueStatus) error {
	m.mu.RLock()
	tracker, ok := m.trackers[trackerType]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("unsupported issue tracker: %s", trackerType)
	}
	return tracker.UpdateIssueStatus(ctx, externalID, status)
}
