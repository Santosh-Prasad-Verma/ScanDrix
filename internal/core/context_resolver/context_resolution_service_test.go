package contextresolver_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	contextresolver "github.com/scandrix/backend/internal/core/context_resolver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockIntegrationConfigReader struct {
	configs []contextresolver.IntegrationConfigModel
}

func (m *mockIntegrationConfigReader) FindByOrganizationAndConfigKey(ctx context.Context, organizationID uuid.UUID, configKey string) ([]contextresolver.IntegrationConfigModel, error) {
	return m.configs, nil
}

type mockParametersReader struct {
	params map[string]*contextresolver.ParameterModel
}

func (m *mockParametersReader) FindByKey(ctx context.Context, key string, organizationID, teamID uuid.UUID) (*contextresolver.ParameterModel, error) {
	return m.params[key], nil
}

func TestContextResolutionService(t *testing.T) {
	orgID := uuid.New()
	teamID := uuid.New()
	repoID := "repo_12345"
	dirID := "dir_abc"

	mockIntegrations := &mockIntegrationConfigReader{
		configs: []contextresolver.IntegrationConfigModel{
			{
				UUID:      uuid.New(),
				TeamID:    teamID,
				ConfigKey: "repositories",
				ConfigValue: []map[string]any{
					{
						"id":   repoID,
						"name": "scandrix/backend",
					},
				},
			},
		},
	}

	mockParams := &mockParametersReader{
		params: map[string]*contextresolver.ParameterModel{
			"code_review_config": {
				UUID: uuid.New(),
				Key:  "code_review_config",
				ConfigValue: map[string]any{
					"repositories": []map[string]any{
						{
							"id":   repoID,
							"name": "scandrix/backend",
							"directories": []map[string]any{
								{
									"id":   dirID,
									"path": "/src/core",
								},
							},
						},
					},
				},
			},
		},
	}

	svc := contextresolver.NewContextResolutionService(mockIntegrations, mockParams)
	ctx := context.Background()

	// 1. Resolve Team ID
	resolvedTeamID, err := svc.GetTeamIDByOrganizationAndRepository(ctx, orgID, repoID)
	require.NoError(t, err)
	assert.Equal(t, teamID, resolvedTeamID)

	// 2. Resolve Directory Path
	dirPath, err := svc.GetDirectoryPathByOrganizationAndRepository(ctx, orgID, repoID, dirID)
	require.NoError(t, err)
	assert.Equal(t, "/src/core", dirPath)

	// 3. Resolve Repository Name
	repoName, err := svc.GetRepositoryNameByOrganizationAndRepository(ctx, orgID, repoID)
	require.NoError(t, err)
	assert.Equal(t, "scandrix/backend", repoName)

	// 4. Unknown repo should error
	_, err = svc.GetTeamIDByOrganizationAndRepository(ctx, orgID, "nonexistent")
	assert.ErrorIs(t, err, contextresolver.ErrRepositoryNotFound)
}
