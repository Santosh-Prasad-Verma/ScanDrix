// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/scandrix/backend/internal/platform/domain/contracts"
	"github.com/scandrix/backend/internal/platform/domain/types"
	"github.com/scandrix/backend/pkg/models"
)

type mockCMServiceForMembers struct {
	contracts.ICodeManagementService
	members []types.PullRequestAuthor
	authors []types.PullRequestAuthor
	memErr  error
	authErr error
}

func (m *mockCMServiceForMembers) Provider() models.SCMProvider {
	return models.ProviderGitHub
}

func (m *mockCMServiceForMembers) GetListMembers(ctx context.Context, orgData types.OrganizationAndTeamData) ([]types.PullRequestAuthor, error) {
	if m.memErr != nil {
		return nil, m.memErr
	}
	return m.members, nil
}

func (m *mockCMServiceForMembers) GetPullRequestAuthors(ctx context.Context, orgData types.OrganizationAndTeamData, determineBots bool) ([]types.PullRequestAuthor, error) {
	if m.authErr != nil {
		return nil, m.authErr
	}
	return m.authors, nil
}

func TestOrganizationMemberListService_Fetch(t *testing.T) {
	ctx := context.Background()
	orgData := types.OrganizationAndTeamData{
		OrganizationID: "org-test-123",
		TeamID:         "team-test-456",
	}

	t.Run("successful fetch with deduplication and bot detection", func(t *testing.T) {
		mockCM := &mockCMServiceForMembers{
			members: []types.PullRequestAuthor{
				{ID: "usr-1", Name: "Alice Developer", Username: "alice"},
				{ID: "bot-1", Name: "drixy[bot]", Username: "drixy-bot", IsBot: true},
			},
			authors: []types.PullRequestAuthor{
				{ID: "usr-1", Name: "Alice Developer", Username: "alice"}, // Duplicate
				{ID: "usr-2", Name: "Bob Architect", Username: "bob"},
			},
		}

		svc := NewOrganizationMemberListService(mockCM, nil)

		res, err := svc.Fetch(ctx, orgData, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.Status != "ok" {
			t.Errorf("expected status 'ok', got %s", res.Status)
		}

		if len(res.Members) != 3 {
			t.Fatalf("expected 3 unique members, got %d", len(res.Members))
		}

		// Verify alphabetical sorting by name (Alice, Bob, drixy[bot])
		if res.Members[0].Name != "Alice Developer" {
			t.Errorf("expected first member Alice Developer, got %s", res.Members[0].Name)
		}
		if res.Members[1].Name != "Bob Architect" {
			t.Errorf("expected second member Bob Architect, got %s", res.Members[1].Name)
		}
		if res.Members[2].Type != "bot" {
			t.Errorf("expected bot type for drixy[bot], got %s", res.Members[2].Type)
		}
	})

	t.Run("cached result on second fetch", func(t *testing.T) {
		mockCM := &mockCMServiceForMembers{
			members: []types.PullRequestAuthor{
				{ID: "usr-1", Name: "Alice Developer", Username: "alice"},
			},
		}
		cache := NewMemoryCache()
		svc := NewOrganizationMemberListService(mockCM, cache)

		// First fetch
		res1, err := svc.Fetch(ctx, orgData, false)
		if err != nil || len(res1.Members) != 1 {
			t.Fatalf("initial fetch failed: %v", err)
		}

		// Mutate mock to return empty
		mockCM.members = nil

		// Second fetch should return cached
		res2, err := svc.Fetch(ctx, orgData, false)
		if err != nil {
			t.Fatalf("cached fetch failed: %v", err)
		}
		if len(res2.Members) != 1 || res2.Members[0].ID != "usr-1" {
			t.Errorf("expected cached item, got %v", res2.Members)
		}

		// Refresh should clear cache and see mutated mock
		res3, err := svc.RefreshMembers(ctx, orgData)
		if err != nil {
			t.Fatalf("refresh failed: %v", err)
		}
		if res3.Status != "unavailable" {
			t.Errorf("expected unavailable after refresh on empty mock, got %s", res3.Status)
		}
	})

	t.Run("total failure returns unavailable", func(t *testing.T) {
		mockCM := &mockCMServiceForMembers{
			memErr:  errors.New("github 500"),
			authErr: errors.New("github 500"),
		}
		svc := NewOrganizationMemberListService(mockCM, nil)

		res, err := svc.Fetch(ctx, orgData, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Status != "unavailable" {
			t.Errorf("expected status 'unavailable', got %s", res.Status)
		}
	})
}

func TestMemoryCache_TTL(t *testing.T) {
	c := NewMemoryCache()
	c.Set("k1", "val1", 50*time.Millisecond)

	val, found := c.Get("k1")
	if !found || val != "val1" {
		t.Fatalf("expected val1, got %v", val)
	}

	time.Sleep(60 * time.Millisecond)

	_, found = c.Get("k1")
	if found {
		t.Fatalf("expected expired entry")
	}
}
