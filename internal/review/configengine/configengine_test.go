// ScanDrix AI - Enterprise Multi-Agent Code Review Platform
// Copyright (c) 2026 ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// Domain: scandrix.dev

package configengine_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scandrix/backend/internal/review/configengine"
	"github.com/scandrix/backend/internal/review/diff"
	"github.com/scandrix/backend/internal/review/domain"
)

type mockConfigPersistenceProvider struct {
	orgConfigs  map[uuid.UUID]*domain.CodeReviewConfig
	teamConfigs map[string]*domain.CodeReviewConfig // key: orgID:teamID
	repoConfigs map[string]*domain.CodeReviewConfig // key: orgID:repoID
	repoTeams   map[string]*uuid.UUID               // key: orgID:repoID
}

func newMockPersistence() *mockConfigPersistenceProvider {
	return &mockConfigPersistenceProvider{
		orgConfigs:  make(map[uuid.UUID]*domain.CodeReviewConfig),
		teamConfigs: make(map[string]*domain.CodeReviewConfig),
		repoConfigs: make(map[string]*domain.CodeReviewConfig),
		repoTeams:   make(map[string]*uuid.UUID),
	}
}

func (m *mockConfigPersistenceProvider) GetOrganizationConfig(ctx context.Context, orgID uuid.UUID) (*domain.CodeReviewConfig, error) {
	return m.orgConfigs[orgID], nil
}

func (m *mockConfigPersistenceProvider) GetTeamConfig(ctx context.Context, orgID, teamID uuid.UUID) (*domain.CodeReviewConfig, error) {
	k := orgID.String() + ":" + teamID.String()
	return m.teamConfigs[k], nil
}

func (m *mockConfigPersistenceProvider) GetRepositoryConfig(ctx context.Context, orgID, repoID uuid.UUID) (*domain.CodeReviewConfig, error) {
	k := orgID.String() + ":" + repoID.String()
	return m.repoConfigs[k], nil
}

func (m *mockConfigPersistenceProvider) GetRepositoryTeamID(ctx context.Context, orgID, repoID uuid.UUID) (*uuid.UUID, error) {
	k := orgID.String() + ":" + repoID.String()
	return m.repoTeams[k], nil
}

func TestInRepoConfigParser_YAML(t *testing.T) {
	parser := configengine.NewInRepoConfigParser()

	yamlContent := `
version: "1.0"
review_mode: "deep"
sensitivity: "strict"
max_comments_per_review: 25
committable_suggestions: true
require_ticket_context: true
ignored_file_patterns:
  - "**/*.min.js"
  - "vendor/**"
excluded_branch_patterns:
  - "release/*"
review_options:
  bug: true
  security: true
  performance: true
`
	cfg, warnings, err := parser.ParseContent(".scandrix.yml", []byte(yamlContent))
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.NotNil(t, cfg)

	assert.Equal(t, configengine.ModeDeep, *cfg.ReviewMode)
	assert.Equal(t, configengine.SensitivityStrict, *cfg.Sensitivity)
	assert.Equal(t, 25, *cfg.MaxCommentsPerReview)
	assert.True(t, *cfg.CommittableSuggestions)
	assert.True(t, *cfg.RequireTicketContext)
	assert.Contains(t, cfg.IgnoredFilePatterns, "**/*.min.js")
	assert.Contains(t, cfg.ExcludedBranchPatterns, "release/*")
	assert.True(t, cfg.ReviewOptions.Bug)
}

func TestInRepoConfigParser_JSON(t *testing.T) {
	parser := configengine.NewInRepoConfigParser()

	jsonContent := `{
		"version": "1.0",
		"review_mode": "fast",
		"sensitivity": "lenient",
		"max_comments_per_review": 5,
		"ignored_file_patterns": ["*.generated.go"]
	}`

	cfg, warnings, err := parser.ParseContent(".scandrix/config.json", []byte(jsonContent))
	require.NoError(t, err)
	assert.Empty(t, warnings)
	require.NotNil(t, cfg)

	assert.Equal(t, configengine.ModeFast, *cfg.ReviewMode)
	assert.Equal(t, configengine.SensitivityLenient, *cfg.Sensitivity)
	assert.Equal(t, 5, *cfg.MaxCommentsPerReview)
	assert.Contains(t, cfg.IgnoredFilePatterns, "*.generated.go")
}

func TestInRepoConfigParser_ValidationWarnings(t *testing.T) {
	parser := configengine.NewInRepoConfigParser()

	yamlContent := `
review_mode: "ultra_fast"
sensitivity: "super_strict"
max_comments_per_review: 500
`
	cfg, warnings, err := parser.ParseContent(".scandrix.yml", []byte(yamlContent))
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Len(t, warnings, 3)
	assert.Equal(t, configengine.ModeNormal, *cfg.ReviewMode)
	assert.Equal(t, configengine.SensitivityStandard, *cfg.Sensitivity)
	assert.Equal(t, 100, *cfg.MaxCommentsPerReview)
}

func TestInRepoConfigParser_ExtractFromPatches(t *testing.T) {
	parser := configengine.NewInRepoConfigParser()

	patches := []*diff.FilePatch{
		{
			OldPath: "pkg/auth/token.go",
			NewPath: "pkg/auth/token.go",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{Type: diff.LineAddition, Content: "func Validate() {}"},
					},
				},
			},
		},
		{
			OldPath: ".scandrix.yml",
			NewPath: ".scandrix.yml",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{Type: diff.LineAddition, Content: "review_mode: fast"},
						{Type: diff.LineAddition, Content: "sensitivity: strict"},
					},
				},
			},
		},
	}

	cfg, path, warnings, err := parser.ExtractFromPatches(patches)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, ".scandrix.yml", path)
	require.NotNil(t, cfg)
	assert.Equal(t, configengine.ModeFast, *cfg.ReviewMode)
	assert.Equal(t, configengine.SensitivityStrict, *cfg.Sensitivity)
}

func TestCascadeResolver_FullFiveTierInheritance(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.New()
	teamID := uuid.New()
	repoID := uuid.New()

	mock := newMockPersistence()

	// 1. Org Config: Sets Sensitivity to STRICT, ReviewMode to NORMAL
	mock.orgConfigs[orgID] = &domain.CodeReviewConfig{
		ReviewMode:     "normal",
		Sensitivity:    "STRICT",
		MaxSuggestions: 20,
		IgnorePaths:    []string{"org-global.log"},
	}

	// 2. Team Config: Links repo to team, overrides MaxSuggestions to 30
	mock.repoTeams[orgID.String()+":"+repoID.String()] = &teamID
	mock.teamConfigs[orgID.String()+":"+teamID.String()] = &domain.CodeReviewConfig{
		MaxSuggestions: 30,
		IgnorePaths:    []string{"team-cache/**"},
	}

	// 3. Repo Config: Sets RequireTicketContext, overrides ReviewMode to DEEP
	mock.repoConfigs[orgID.String()+":"+repoID.String()] = &domain.CodeReviewConfig{
		ReviewMode:           "deep",
		RequireTicketContext: true,
		IgnorePaths:          []string{"repo-specific/**"},
	}

	// 4. In-Repo Override: Sets ReviewMode to FAST
	fastMode := configengine.ModeFast
	inRepo := &configengine.InRepoConfiguration{
		ReviewMode:          &fastMode,
		IgnoredFilePatterns: []string{"inrepo-temp/**"},
	}

	resolver := configengine.NewCascadeResolver(mock, nil)
	result, err := resolver.Resolve(ctx, configengine.ResolveInput{
		WorkspaceID:  orgID,
		RepositoryID: repoID,
		InRepoConfig: inRepo,
	})
	require.NoError(t, err)

	// In-repo wins for ReviewMode
	assert.Equal(t, "fast", result.Config.ReviewMode)
	assert.Equal(t, configengine.ScopeInRepo, result.AuditTrail.ReviewMode.Scope)

	// Org wins for Sensitivity
	assert.Equal(t, "STRICT", result.Config.Sensitivity)
	assert.Equal(t, configengine.ScopeOrganization, result.AuditTrail.Sensitivity.Scope)

	// Team wins for MaxCommentsPerReview
	assert.Equal(t, 30, result.Config.MaxSuggestions)
	assert.Equal(t, configengine.ScopeTeam, result.AuditTrail.MaxCommentsPerReview.Scope)

	// Repo wins for RequireTicketContext
	assert.True(t, result.Config.RequireTicketContext)
	assert.Equal(t, configengine.ScopeRepository, result.AuditTrail.RequireTicketContext.Scope)

	// Ignored file patterns are a UNION across all tiers with audit tracking!
	assert.Contains(t, result.Config.IgnorePaths, "org-global.log")
	assert.Contains(t, result.Config.IgnorePaths, "team-cache/**")
	assert.Contains(t, result.Config.IgnorePaths, "repo-specific/**")
	assert.Contains(t, result.Config.IgnorePaths, "inrepo-temp/**")

	assert.Equal(t, configengine.ScopeOrganization, result.AuditTrail.IgnoredFilePatternsSources["org-global.log"])
	assert.Equal(t, configengine.ScopeTeam, result.AuditTrail.IgnoredFilePatternsSources["team-cache/**"])
	assert.Equal(t, configengine.ScopeRepository, result.AuditTrail.IgnoredFilePatternsSources["repo-specific/**"])
	assert.Equal(t, configengine.ScopeInRepo, result.AuditTrail.IgnoredFilePatternsSources["inrepo-temp/**"])
}
