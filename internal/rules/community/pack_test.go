// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package community_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/rules/community"
	"github.com/scandrix/backend/pkg/models"
)

type mockRuleWriter struct {
	created []*models.ReviewRule
}

func (m *mockRuleWriter) CreateReviewRule(ctx context.Context, rule *models.ReviewRule) error {
	m.created = append(m.created, rule)
	return nil
}

func TestCuratedPacks_Integrity(t *testing.T) {
	packs := community.CuratedPacks()
	if len(packs) == 0 {
		t.Fatal("expected at least one curated pack")
	}

	for _, p := range packs {
		if p.ID == "" || p.Title == "" || p.Category == "" || p.Author == "" {
			t.Errorf("pack %+v missing required fields", p)
		}
		if len(p.Rules) == 0 {
			t.Errorf("pack %s has no rules", p.ID)
		}
		for _, r := range p.Rules {
			if r.Name == "" {
				t.Errorf("rule in pack %s missing name", p.ID)
			}
			if r.RegexRule != "" {
				if _, err := regexp.Compile(r.RegexRule); err != nil {
					t.Errorf("rule %s in pack %s has invalid regex: %v", r.Name, p.ID, err)
				}
			}
		}
	}
}

func TestListPacks_Filtering(t *testing.T) {
	svc := community.NewService()

	// Filter by category
	secPacks := svc.ListPacks("", "SECURITY", "")
	if len(secPacks) == 0 {
		t.Error("expected security packs")
	}
	for _, p := range secPacks {
		if p.Category != "SECURITY" {
			t.Errorf("expected SECURITY category, got %s", p.Category)
		}
	}

	// Filter by search query
	owaspPacks := svc.ListPacks("owasp", "ALL", "")
	if len(owaspPacks) == 0 {
		t.Error("expected OWASP pack to be found by search query")
	}

	// Filter by tag
	goPacks := svc.ListPacks("", "", "go")
	if len(goPacks) == 0 {
		t.Error("expected Go pack to be found by tag")
	}
}

func TestNormalizeRemoteURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "https://github.com/scandrix/rules/blob/main/owasp.json",
			expected: "https://raw.githubusercontent.com/scandrix/rules/main/owasp.json",
		},
		{
			input:    "https://gitlab.com/scandrix/rules/-/blob/main/owasp.yaml",
			expected: "https://gitlab.com/scandrix/rules/-/raw/main/owasp.yaml",
		},
		{
			input:    "https://raw.githubusercontent.com/scandrix/rules/main/owasp.json",
			expected: "https://raw.githubusercontent.com/scandrix/rules/main/owasp.json",
		},
	}

	for _, tc := range tests {
		actual := community.NormalizeRemoteURL(tc.input)
		if actual != tc.expected {
			t.Errorf("NormalizeRemoteURL(%q) = %q, expected %q", tc.input, actual, tc.expected)
		}
	}
}

func TestParseAndValidateRulePack_JSON_and_YAML(t *testing.T) {
	jsonContent := []byte(`{
		"title": "Custom Test Pack",
		"category": "SECURITY",
		"rules": [
			{
				"name": "No Eval",
				"regexRule": "\\beval\\(",
				"severity": "CRITICAL"
			}
		]
	}`)

	pack, err := community.ParseAndValidateRulePack(jsonContent)
	if err != nil {
		t.Fatalf("unexpected error parsing JSON: %v", err)
	}
	if pack.Title != "Custom Test Pack" || len(pack.Rules) != 1 {
		t.Errorf("unexpected pack parsed: %+v", pack)
	}

	yamlContent := []byte(`
title: "YAML Test Pack"
category: "PERFORMANCE"
rules:
  - name: "Sleep In Loop"
    regexRule: "time\\.Sleep"
    severity: "MEDIUM"
`)

	yamlPack, err := community.ParseAndValidateRulePack(yamlContent)
	if err != nil {
		t.Fatalf("unexpected error parsing YAML: %v", err)
	}
	if yamlPack.Title != "YAML Test Pack" || len(yamlPack.Rules) != 1 {
		t.Errorf("unexpected pack parsed from YAML: %+v", yamlPack)
	}

	// Invalid regex should fail
	invalidRegex := []byte(`{
		"title": "Bad Regex Pack",
		"rules": [{"name": "Bad", "regexRule": "(unclosed"}]
	}`)
	_, err = community.ParseAndValidateRulePack(invalidRegex)
	if err == nil {
		t.Fatal("expected error on unclosed regex")
	}
}

func TestInstallPack_Success(t *testing.T) {
	svc := community.NewService()
	writer := &mockRuleWriter{}
	wsID := uuid.New()

	count, err := svc.InstallPack(context.Background(), wsID, "pack-secrets-guard", writer)
	if err != nil {
		t.Fatalf("unexpected error installing pack: %v", err)
	}
	if count != 3 || len(writer.created) != 3 {
		t.Errorf("expected 3 rules installed, got %d", count)
	}
	for _, r := range writer.created {
		if r.WorkspaceID != wsID {
			t.Errorf("expected workspace ID %s, got %s", wsID, r.WorkspaceID)
		}
	}
}
