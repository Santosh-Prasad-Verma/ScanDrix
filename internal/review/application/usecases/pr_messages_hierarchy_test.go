// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package usecases

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/review/domain"
	"github.com/scandrix/backend/pkg/models"
)

type mockMessagesRepo struct {
	mu      sync.RWMutex
	storage map[string]*domain.PullRequestMessages
}

func newMockMessagesRepo() *mockMessagesRepo {
	return &mockMessagesRepo{
		storage: make(map[string]*domain.PullRequestMessages),
	}
}

func makeKey(orgID string, level domain.ConfigLevel, repoID, dirID string) string {
	return strings.Join([]string{orgID, string(level), repoID, dirID}, ":")
}

func (m *mockMessagesRepo) Create(ctx context.Context, msg *domain.PullRequestMessages) (*domain.PullRequestMessages, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := makeKey(msg.OrganizationID, msg.ConfigLevel, msg.RepositoryID, msg.DirectoryID)
	cp := *msg
	m.storage[key] = &cp
	return &cp, nil
}

func (m *mockMessagesRepo) Update(ctx context.Context, msg *domain.PullRequestMessages) (*domain.PullRequestMessages, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := makeKey(msg.OrganizationID, msg.ConfigLevel, msg.RepositoryID, msg.DirectoryID)
	cp := *msg
	m.storage[key] = &cp
	return &cp, nil
}

func (m *mockMessagesRepo) Delete(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.storage {
		if v.ID == id {
			delete(m.storage, k)
			break
		}
	}
	return nil
}

func (m *mockMessagesRepo) DeleteByFilter(ctx context.Context, filter domain.MessagesFilter) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := makeKey(filter.OrganizationID, filter.ConfigLevel, filter.RepositoryID, filter.DirectoryID)
	if _, exists := m.storage[key]; exists {
		delete(m.storage, key)
		return 1, nil
	}
	return 0, nil
}

func (m *mockMessagesRepo) Find(ctx context.Context, filter domain.MessagesFilter) ([]domain.PullRequestMessages, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []domain.PullRequestMessages
	for _, v := range m.storage {
		if filter.OrganizationID != "" && v.OrganizationID != filter.OrganizationID {
			continue
		}
		if filter.ConfigLevel != "" && v.ConfigLevel != filter.ConfigLevel {
			continue
		}
		if filter.RepositoryID != "" && v.RepositoryID != filter.RepositoryID {
			continue
		}
		res = append(res, *v)
	}
	return res, nil
}

func (m *mockMessagesRepo) FindOne(ctx context.Context, filter domain.MessagesFilter) (*domain.PullRequestMessages, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := makeKey(filter.OrganizationID, filter.ConfigLevel, filter.RepositoryID, filter.DirectoryID)
	if v, exists := m.storage[key]; exists {
		cp := *v
		return &cp, nil
	}
	return nil, nil
}

func (m *mockMessagesRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.PullRequestMessages, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, v := range m.storage {
		if v.ID == id {
			cp := *v
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockMessagesRepo) FindOverrideCountsByOrg(ctx context.Context, orgID string) ([]domain.DirectoryOverrideCount, error) {
	return nil, nil
}

func TestReviewMessageTemplateEngine_Render(t *testing.T) {
	engine := NewReviewMessageTemplateEngine()

	template := "Review for PR #{{pr_number}} by @{{author}} on {{branch}}. Issues: {{critical_count}} critical, {{total_findings}} total. Status: {{status_badge}}."
	vars := TemplateVariables{
		PRNumber:       42,
		Author:         "octocat",
		Branch:         "feature/payments",
		CriticalCount:  2,
		TotalFindings:  5,
		PassedReview:   false,
		ReviewDuration: 1200 * time.Millisecond,
	}

	rendered := engine.Render(template, vars)

	expected := "Review for PR #42 by @octocat on feature/payments. Issues: 2 critical, 5 total. Status: CHANGES_REQUESTED."
	if rendered != expected {
		t.Errorf("template render mismatch:\nGot:  %s\nWant: %s", rendered, expected)
	}
}

func TestPRMessagesHierarchy_ResolveEffectiveForFile(t *testing.T) {
	repo := newMockMessagesRepo()
	coord := NewPRMessagesHierarchyCoordinator(repo)
	ctx := context.Background()

	orgID := "org-alpha"
	repoID := "repo-omega"

	// 1. Set Global Config
	globalStart := domain.PullRequestMessageContent{Content: "Global Start", Status: domain.MessageStatusEveryPush}
	_, _, err := coord.SaveWithInheritanceDetection(ctx, &domain.PullRequestMessages{
		OrganizationID:     orgID,
		ConfigLevel:        domain.ConfigLevelGlobal,
		StartReviewMessage: &globalStart,
	})
	if err != nil {
		t.Fatalf("failed to save global: %v", err)
	}

	// 2. Set Repository Config
	repoStart := domain.PullRequestMessageContent{Content: "Repo Start", Status: domain.MessageStatusEveryPush}
	_, _, err = coord.SaveWithInheritanceDetection(ctx, &domain.PullRequestMessages{
		OrganizationID:     orgID,
		ConfigLevel:        domain.ConfigLevelRepository,
		RepositoryID:       repoID,
		StartReviewMessage: &repoStart,
	})
	if err != nil {
		t.Fatalf("failed to save repo: %v", err)
	}

	// 3. Set Directory Override for "services/auth/"
	dirStart := domain.PullRequestMessageContent{Content: "Auth Directory Start", Status: domain.MessageStatusEveryPush}
	_, _, err = coord.SaveWithInheritanceDetection(ctx, &domain.PullRequestMessages{
		OrganizationID:     orgID,
		ConfigLevel:        domain.ConfigLevelDirectory,
		RepositoryID:       repoID,
		DirectoryID:        "services/auth",
		StartReviewMessage: &dirStart,
	})
	if err != nil {
		t.Fatalf("failed to save directory: %v", err)
	}

	// Test A: File in services/auth/login.go should resolve Directory Config
	effAuth, err := coord.ResolveEffectiveForFile(ctx, orgID, repoID, "services/auth/login.go")
	if err != nil || effAuth.StartReviewMessage.Content != "Auth Directory Start" {
		t.Errorf("expected Auth Directory Start, got %v (err: %v)", effAuth.StartReviewMessage, err)
	}

	// Test B: File in services/payment/stripe.go should resolve Repository Config
	effPayment, err := coord.ResolveEffectiveForFile(ctx, orgID, repoID, "services/payment/stripe.go")
	if err != nil || effPayment.StartReviewMessage.Content != "Repo Start" {
		t.Errorf("expected Repo Start, got %v (err: %v)", effPayment.StartReviewMessage, err)
	}

	// Test C: File in other repo should resolve Global Config
	effOther, err := coord.ResolveEffectiveForFile(ctx, orgID, "other-repo", "main.go")
	if err != nil || effOther.StartReviewMessage.Content != "Global Start" {
		t.Errorf("expected Global Start, got %v (err: %v)", effOther.StartReviewMessage, err)
	}
}

func TestPRMessagesHierarchy_InheritancePruning(t *testing.T) {
	repo := newMockMessagesRepo()
	coord := NewPRMessagesHierarchyCoordinator(repo)
	ctx := context.Background()

	orgID := "org-inherit"
	repoID := "repo-inherit"

	// 1. Create Global config
	globalContent := domain.PullRequestMessageContent{Content: "Standard Enterprise Review", Status: domain.MessageStatusEveryPush}
	_, _, _ = coord.SaveWithInheritanceDetection(ctx, &domain.PullRequestMessages{
		OrganizationID:   orgID,
		ConfigLevel:      domain.ConfigLevelGlobal,
		EndReviewMessage: &globalContent,
	})

	// 2. Create custom repository config with DIFFERENT message
	repoCustom := domain.PullRequestMessageContent{Content: "Custom Repo Review", Status: domain.MessageStatusEveryPush}
	_, inherited, err := coord.SaveWithInheritanceDetection(ctx, &domain.PullRequestMessages{
		OrganizationID:   orgID,
		ConfigLevel:      domain.ConfigLevelRepository,
		RepositoryID:     repoID,
		EndReviewMessage: &repoCustom,
	})
	if err != nil || inherited {
		t.Fatalf("expected custom repo config to be saved (inherited=false), got %v", inherited)
	}

	// 3. Update repository config to MATCH Global config exactly
	_, inherited, err = coord.SaveWithInheritanceDetection(ctx, &domain.PullRequestMessages{
		OrganizationID:   orgID,
		ConfigLevel:      domain.ConfigLevelRepository,
		RepositoryID:     repoID,
		EndReviewMessage: &globalContent, // Matches parent!
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inherited {
		t.Errorf("expected configuration matching parent to be marked as inherited")
	}

	// Verify the redundant repo override was pruned from storage
	key := makeKey(orgID, domain.ConfigLevelRepository, repoID, "")
	repo.mu.RLock()
	_, exists := repo.storage[key]
	repo.mu.RUnlock()

	if exists {
		t.Errorf("expected redundant repository config to be pruned from storage")
	}
}

func TestPRMessagesHierarchy_RenderReviewSummaryBanner(t *testing.T) {
	coord := NewPRMessagesHierarchyCoordinator(newMockMessagesRepo())

	endMsg := domain.PullRequestMessageContent{
		Content: "Automated scan for **{{author}}** on `{{branch}}`.",
		Status:  domain.MessageStatusEveryPush,
	}
	cfg := &domain.PullRequestMessages{
		EndReviewMessage: &endMsg,
	}

	vars := TemplateVariables{
		PRNumber:       101,
		Author:         "alice",
		Branch:         "fix/sqli",
		HeadSHA:        "a1b2c3d4e5f6",
		CriticalCount:  1,
		HighCount:      0,
		MediumCount:    1,
		LowCount:       0,
		PassedReview:   false,
		ReviewDuration: 850 * time.Millisecond,
	}

	findings := []models.CodeFinding{
		{
			FilePath:      "pkg/db/query.go",
			StartLine:     25,
			Severity:      models.SeverityCritical,
			Title:         "SQL Injection Hazard",
			Description:   "Raw string interpolation in database query",
			Remediation:   "Use parameterized query",
			SuggestedDiff: "- db.Query(fmt.Sprintf(...))\n+ db.Query(query, id)",
		},
	}

	banner := coord.RenderReviewSummaryBanner(cfg, vars, findings)

	// Verify markdown elements
	if !strings.Contains(banner, "## ❌ ScanDrix Automated Review: Changes Requested") {
		t.Errorf("banner missing expected header")
	}
	if !strings.Contains(banner, "Automated scan for **alice** on `fix/sqli`.") {
		t.Errorf("banner missing rendered template body")
	}
	if !strings.Contains(banner, "🔴 Critical | 1 | Action Required") {
		t.Errorf("banner missing critical severity count table row")
	}
	if !strings.Contains(banner, "```suggestion") {
		t.Errorf("banner missing committable suggestion block")
	}
	if !strings.Contains(banner, "Reviewed by [ScanDrix AI](https://scandrix.dev)") {
		t.Errorf("banner missing footer trace link")
	}
}

func TestPRMessagesHierarchy_ConcurrentStress(t *testing.T) {
	repo := newMockMessagesRepo()
	coord := NewPRMessagesHierarchyCoordinator(repo)

	var wg sync.WaitGroup
	workers := 25

	for w := 0; w < workers; w++ {
		wg.Add(2)

		// Worker A: write/update
		go func(id int) {
			defer wg.Done()
			c := domain.PullRequestMessageContent{
				Content: "Worker content",
				Status:  domain.MessageStatusEveryPush,
			}
			_, _, _ = coord.SaveWithInheritanceDetection(context.Background(), &domain.PullRequestMessages{
				OrganizationID:   "stress-org",
				ConfigLevel:      domain.ConfigLevelRepository,
				RepositoryID:     "stress-repo",
				EndReviewMessage: &c,
			})
		}(w)

		// Worker B: resolve effective
		go func(id int) {
			defer wg.Done()
			_, _ = coord.ResolveEffectiveForFile(context.Background(), "stress-org", "stress-repo", "pkg/file.go")
		}(w)
	}

	wg.Wait()
}
