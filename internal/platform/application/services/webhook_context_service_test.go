// ═══════════════════════════════════════════════════════════════
// ScanDrix AI - Enterprise Code Review Platform
// Copyright (c) 2026 ScanDrix AI. All rights reserved.
// ═══════════════════════════════════════════════════════════════

package services

import (
	"context"
	"testing"

	"github.com/scandrix/backend/pkg/models"
)

type mockCfgReader struct {
	configs []IntegrationConfigRecord
}

func (m *mockCfgReader) FindConfigsByRepository(ctx context.Context, platform models.SCMProvider, repoID string) ([]IntegrationConfigRecord, error) {
	var matched []IntegrationConfigRecord
	for _, c := range m.configs {
		if c.Platform == platform && c.Value == repoID {
			matched = append(matched, c)
		}
	}
	return matched, nil
}

type mockAutoReader struct {
	activeTeams map[string]string // teamID -> autoID
}

func (m *mockAutoReader) IsCodeReviewActive(ctx context.Context, teamID string) (string, bool, error) {
	if autoID, ok := m.activeTeams[teamID]; ok {
		return autoID, true, nil
	}
	return "", false, nil
}

func TestWebhookContextService(t *testing.T) {
	configs := []IntegrationConfigRecord{
		{
			ID:             "cfg-1",
			Platform:       models.SCMProviderGitLab,
			Value:          "repo-100",
			OrganizationID: "org-cloud",
			TeamID:         "team-cloud",
			Host:           "gitlab.com",
			BotUsername:    "drixy",
		},
		{
			ID:             "cfg-2",
			Platform:       models.SCMProviderGitLab,
			Value:          "repo-100",
			OrganizationID: "org-self",
			TeamID:         "team-self",
			Host:           "gitlab.enterprise.internal",
			BotUsername:    "custom-drixy",
		},
	}

	activeTeams := map[string]string{
		"team-cloud": "auto-cloud-1",
		"team-self":  "auto-self-1",
	}

	svc := NewWebhookContextService(&mockCfgReader{configs: configs}, &mockAutoReader{activeTeams: activeTeams})

	// Test Disambiguation by host
	resCloud, err := svc.GetContext(context.Background(), models.SCMProviderGitLab, "repo-100", &WebhookDisambiguator{
		Host: "https://gitlab.com/some/path",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resCloud == nil || resCloud.OrganizationAndTeamData.TeamID != "team-cloud" {
		t.Fatalf("expected team-cloud, got %+v", resCloud)
	}

	resSelf, err := svc.GetContext(context.Background(), models.SCMProviderGitLab, "repo-100", &WebhookDisambiguator{
		Host: "gitlab.enterprise.internal",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resSelf == nil || resSelf.OrganizationAndTeamData.TeamID != "team-self" {
		t.Fatalf("expected team-self, got %+v", resSelf)
	}
	if resSelf.BotUsername != "custom-drixy" {
		t.Errorf("expected custom-drixy, got %s", resSelf.BotUsername)
	}

	// Test Missing repository
	resMissing, err := svc.GetContext(context.Background(), models.SCMProviderGitHub, "non-existent", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resMissing != nil {
		t.Errorf("expected nil for missing repo, got %+v", resMissing)
	}
}
