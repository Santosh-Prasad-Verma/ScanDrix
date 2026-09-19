package contextresolver_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	contextresolver "github.com/scandrix/backend/internal/core/context_resolver"
	"github.com/scandrix/backend/internal/core/crypto"
	"github.com/scandrix/backend/internal/core/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Thread-Safe Mock Readers for Matrix Testing
// ============================================================================

type threadSafeIntegrationReader struct {
	mu          sync.RWMutex
	configs     map[string][]contextresolver.IntegrationConfigModel
	callCounter atomic.Int64
	failErr     error
}

func newThreadSafeIntegrationReader() *threadSafeIntegrationReader {
	return &threadSafeIntegrationReader{
		configs: make(map[string][]contextresolver.IntegrationConfigModel),
	}
}

func (r *threadSafeIntegrationReader) key(orgID uuid.UUID, configKey string) string {
	return fmt.Sprintf("%s:%s", orgID.String(), configKey)
}

func (r *threadSafeIntegrationReader) AddConfig(orgID uuid.UUID, configKey string, cfg contextresolver.IntegrationConfigModel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.key(orgID, configKey)
	r.configs[k] = append(r.configs[k], cfg)
}

func (r *threadSafeIntegrationReader) SetFailErr(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failErr = err
}

func (r *threadSafeIntegrationReader) FindByOrganizationAndConfigKey(
	ctx context.Context,
	organizationID uuid.UUID,
	configKey string,
) ([]contextresolver.IntegrationConfigModel, error) {
	r.callCounter.Add(1)
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.failErr != nil {
		return nil, r.failErr
	}
	k := r.key(organizationID, configKey)
	list, exists := r.configs[k]
	if !exists {
		return nil, nil
	}
	// Return copy to prevent race conditions on slice mutation
	cp := make([]contextresolver.IntegrationConfigModel, len(list))
	copy(cp, list)
	return cp, nil
}

type threadSafeParametersReader struct {
	mu          sync.RWMutex
	params      map[string]*contextresolver.ParameterModel
	callCounter atomic.Int64
	failErr     error
}

func newThreadSafeParametersReader() *threadSafeParametersReader {
	return &threadSafeParametersReader{
		params: make(map[string]*contextresolver.ParameterModel),
	}
}

func (r *threadSafeParametersReader) key(key string, orgID, teamID uuid.UUID) string {
	return fmt.Sprintf("%s:%s:%s", key, orgID.String(), teamID.String())
}

func (r *threadSafeParametersReader) SetParam(key string, orgID, teamID uuid.UUID, model *contextresolver.ParameterModel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := r.key(key, orgID, teamID)
	r.params[k] = model
}

func (r *threadSafeParametersReader) SetFailErr(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failErr = err
}

func (r *threadSafeParametersReader) FindByKey(
	ctx context.Context,
	key string,
	organizationID, teamID uuid.UUID,
) (*contextresolver.ParameterModel, error) {
	r.callCounter.Add(1)
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.failErr != nil {
		return nil, r.failErr
	}
	k := r.key(key, organizationID, teamID)
	return r.params[k], nil
}

type matrixTeamCliKeyLookup struct {
	mu           sync.RWMutex
	keys         map[string]*domain.TeamCliKey
	lastUsedHits map[uuid.UUID]int64
	lookupCount  atomic.Int64
}

func newMatrixTeamCliKeyLookup() *matrixTeamCliKeyLookup {
	return &matrixTeamCliKeyLookup{
		keys:         make(map[string]*domain.TeamCliKey),
		lastUsedHits: make(map[uuid.UUID]int64),
	}
}

func (l *matrixTeamCliKeyLookup) RegisterKey(key *domain.TeamCliKey) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.keys[key.KeyHash] = key
}

func (l *matrixTeamCliKeyLookup) FindByHash(ctx context.Context, keyHash string) (*domain.TeamCliKey, error) {
	l.lookupCount.Add(1)
	l.mu.RLock()
	defer l.mu.RUnlock()

	key, exists := l.keys[keyHash]
	if !exists {
		return nil, nil
	}
	return key, nil
}

func (l *matrixTeamCliKeyLookup) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lastUsedHits[id]++
	return nil
}

type matrixWorkspaceRepo struct {
	mu         sync.RWMutex
	workspaces map[uuid.UUID]*domain.Workspace
}

func newMatrixWorkspaceRepo() *matrixWorkspaceRepo {
	return &matrixWorkspaceRepo{
		workspaces: make(map[uuid.UUID]*domain.Workspace),
	}
}

func (r *matrixWorkspaceRepo) Register(ws *domain.Workspace) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workspaces[ws.ID] = ws
}

func (r *matrixWorkspaceRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Workspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.workspaces[id], nil
}

func (r *matrixWorkspaceRepo) FindBySlug(ctx context.Context, slug string) (*domain.Workspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, ws := range r.workspaces {
		if ws.Slug == slug {
			return ws, nil
		}
	}
	return nil, nil
}

func (r *matrixWorkspaceRepo) Create(ctx context.Context, ws *domain.Workspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workspaces[ws.ID] = ws
	return nil
}

func (r *matrixWorkspaceRepo) Update(ctx context.Context, ws *domain.Workspace) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workspaces[ws.ID] = ws
	return nil
}

func (r *matrixWorkspaceRepo) Delete(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.workspaces, id)
	return nil
}

func (r *matrixWorkspaceRepo) List(ctx context.Context, query domain.PaginationQuery) (*domain.PaginatedResult[*domain.Workspace], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []*domain.Workspace
	for _, ws := range r.workspaces {
		list = append(list, ws)
	}
	return &domain.PaginatedResult[*domain.Workspace]{
		Items:      list,
		TotalCount: int64(len(list)),
		Page:       1,
		PageSize:   len(list),
		TotalPages: 1,
	}, nil
}

// ============================================================================
// ContextResolutionService Matrix Tests
// ============================================================================

func TestContextResolutionMatrix_GetTeamID_ValidationAndErrors(t *testing.T) {
	intReader := newThreadSafeIntegrationReader()
	paramReader := newThreadSafeParametersReader()
	svc := contextresolver.NewContextResolutionService(intReader, paramReader)

	orgID := uuid.New()
	ctx := context.Background()

	t.Run("Nil OrganizationID", func(t *testing.T) {
		res, err := svc.GetTeamIDByOrganizationAndRepository(ctx, uuid.Nil, "repo-123")
		assert.Error(t, err)
		assert.Equal(t, uuid.Nil, res)
		assert.Contains(t, err.Error(), "organizationId and repositoryId must be provided")
	})

	t.Run("Empty RepositoryID", func(t *testing.T) {
		res, err := svc.GetTeamIDByOrganizationAndRepository(ctx, orgID, "   ")
		assert.Error(t, err)
		assert.Equal(t, uuid.Nil, res)
		assert.Contains(t, err.Error(), "organizationId and repositoryId must be provided")
	})

	t.Run("Reader Returns Error", func(t *testing.T) {
		intReader.SetFailErr(assert.AnError)
		defer intReader.SetFailErr(nil)

		res, err := svc.GetTeamIDByOrganizationAndRepository(ctx, orgID, "repo-123")
		assert.ErrorIs(t, err, assert.AnError)
		assert.Equal(t, uuid.Nil, res)
	})

	t.Run("No Active Integrations", func(t *testing.T) {
		res, err := svc.GetTeamIDByOrganizationAndRepository(ctx, orgID, "repo-123")
		assert.ErrorIs(t, err, contextresolver.ErrNoActiveIntegrations)
		assert.Equal(t, uuid.Nil, res)
	})

	t.Run("Repository Not Found in Integrations", func(t *testing.T) {
		teamID := uuid.New()
		intReader.AddConfig(orgID, "repositories", contextresolver.IntegrationConfigModel{
			UUID:      uuid.New(),
			TeamID:    teamID,
			ConfigKey: "repositories",
			ConfigValue: []map[string]any{
				{"id": "other-repo", "name": "Other Repo"},
			},
		})

		res, err := svc.GetTeamIDByOrganizationAndRepository(ctx, orgID, "target-repo")
		assert.Error(t, err)
		assert.ErrorIs(t, err, contextresolver.ErrRepositoryNotFound)
		assert.Equal(t, uuid.Nil, res)
	})
}

func TestContextResolutionMatrix_GetTeamID_PayloadVariations(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name         string
		configValue  any
		searchRepoID string
		expectedTeam bool
	}{
		{
			name: "Raw Slice of Repo Objects by ID",
			configValue: []map[string]any{
				{"id": "repo-alpha", "name": "Alpha Repo", "external_id": "ext-100"},
				{"id": "repo-beta", "name": "Beta Repo", "external_id": "ext-200"},
			},
			searchRepoID: "repo-beta",
			expectedTeam: true,
		},
		{
			name: "Raw Slice of Repo Objects by ExternalID",
			configValue: []map[string]any{
				{"id": "repo-gamma", "name": "Gamma Repo", "external_id": "ext-300"},
			},
			searchRepoID: "ext-300",
			expectedTeam: true,
		},
		{
			name: "Wrapped Repositories Struct by ID",
			configValue: map[string]any{
				"repositories": []map[string]any{
					{"id": "repo-delta", "name": "Delta Repo"},
					{"id": "repo-epsilon", "name": "Epsilon Repo"},
				},
			},
			searchRepoID: "repo-delta",
			expectedTeam: true,
		},
		{
			name: "Wrapped Repositories Struct by ExternalID",
			configValue: map[string]any{
				"repositories": []map[string]any{
					{"id": "repo-zeta", "external_id": "ext-600", "name": "Zeta Repo"},
				},
			},
			searchRepoID: "ext-600",
			expectedTeam: true,
		},
		{
			name:         "Nil Config Value",
			configValue:  nil,
			searchRepoID: "any-repo",
			expectedTeam: false,
		},
		{
			name:         "Primitive String Value",
			configValue:  "not-a-repo-json",
			searchRepoID: "any-repo",
			expectedTeam: false,
		},
		{
			name:         "Empty Map Value",
			configValue:  map[string]any{},
			searchRepoID: "any-repo",
			expectedTeam: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			intReader := newThreadSafeIntegrationReader()
			paramReader := newThreadSafeParametersReader()
			svc := contextresolver.NewContextResolutionService(intReader, paramReader)

			orgID := uuid.New()
			teamID := uuid.New()

			intReader.AddConfig(orgID, "repositories", contextresolver.IntegrationConfigModel{
				UUID:        uuid.New(),
				TeamID:      teamID,
				ConfigKey:   "repositories",
				ConfigValue: tc.configValue,
			})

			resTeam, err := svc.GetTeamIDByOrganizationAndRepository(ctx, orgID, tc.searchRepoID)
			if tc.expectedTeam {
				require.NoError(t, err)
				assert.Equal(t, teamID, resTeam)
			} else {
				assert.Error(t, err)
				assert.Equal(t, uuid.Nil, resTeam)
			}
		})
	}
}

func TestContextResolutionMatrix_GetDirectoryPath_Matrix(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.New()
	teamID := uuid.New()
	repoID := "scandrix-core"

	t.Run("Short-Circuit on Nil or Empty Inputs", func(t *testing.T) {
		svc := contextresolver.NewContextResolutionService(newThreadSafeIntegrationReader(), newThreadSafeParametersReader())

		p1, err1 := svc.GetDirectoryPathByOrganizationAndRepository(ctx, uuid.Nil, repoID, "dir-1")
		assert.NoError(t, err1)
		assert.Empty(t, p1)

		p2, err2 := svc.GetDirectoryPathByOrganizationAndRepository(ctx, orgID, "", "dir-1")
		assert.NoError(t, err2)
		assert.Empty(t, p2)

		p3, err3 := svc.GetDirectoryPathByOrganizationAndRepository(ctx, orgID, repoID, "")
		assert.NoError(t, err3)
		assert.Empty(t, p3)
	})

	t.Run("Code Review Config None", func(t *testing.T) {
		intReader := newThreadSafeIntegrationReader()
		paramReader := newThreadSafeParametersReader()
		svc := contextresolver.NewContextResolutionService(intReader, paramReader)

		intReader.AddConfig(orgID, "repositories", contextresolver.IntegrationConfigModel{
			UUID:      uuid.New(),
			TeamID:    teamID,
			ConfigKey: "repositories",
			ConfigValue: []map[string]any{
				{"id": repoID, "name": "ScanDrix Core"},
			},
		})

		// Parameter not added in paramReader
		p, err := svc.GetDirectoryPathByOrganizationAndRepository(ctx, orgID, repoID, "dir-1")
		assert.ErrorIs(t, err, contextresolver.ErrCodeReviewConfigNone)
		assert.Empty(t, p)
	})

	t.Run("Repository Missing in Code Review Config", func(t *testing.T) {
		intReader := newThreadSafeIntegrationReader()
		paramReader := newThreadSafeParametersReader()
		svc := contextresolver.NewContextResolutionService(intReader, paramReader)

		intReader.AddConfig(orgID, "repositories", contextresolver.IntegrationConfigModel{
			UUID:      uuid.New(),
			TeamID:    teamID,
			ConfigKey: "repositories",
			ConfigValue: []map[string]any{
				{"id": repoID, "name": "ScanDrix Core"},
			},
		})

		paramReader.SetParam("code_review_config", orgID, teamID, &contextresolver.ParameterModel{
			UUID: uuid.New(),
			Key:  "code_review_config",
			ConfigValue: []map[string]any{
				{"id": "different-repo", "name": "Different Repo"},
			},
		})

		p, err := svc.GetDirectoryPathByOrganizationAndRepository(ctx, orgID, repoID, "dir-1")
		assert.Error(t, err)
		assert.ErrorIs(t, err, contextresolver.ErrRepositoryNotFound)
		assert.Empty(t, p)
	})

	t.Run("Repository Has No Directories Configured", func(t *testing.T) {
		intReader := newThreadSafeIntegrationReader()
		paramReader := newThreadSafeParametersReader()
		svc := contextresolver.NewContextResolutionService(intReader, paramReader)

		intReader.AddConfig(orgID, "repositories", contextresolver.IntegrationConfigModel{
			UUID:      uuid.New(),
			TeamID:    teamID,
			ConfigKey: "repositories",
			ConfigValue: []map[string]any{
				{"id": repoID, "name": "ScanDrix Core"},
			},
		})

		paramReader.SetParam("code_review_config", orgID, teamID, &contextresolver.ParameterModel{
			UUID: uuid.New(),
			Key:  "code_review_config",
			ConfigValue: []map[string]any{
				{
					"id":          repoID,
					"name":        "ScanDrix Core",
					"directories": []map[string]any{},
				},
			},
		})

		p, err := svc.GetDirectoryPathByOrganizationAndRepository(ctx, orgID, repoID, "dir-1")
		assert.Error(t, err)
		assert.ErrorIs(t, err, contextresolver.ErrNoDirectoriesConfig)
		assert.Empty(t, p)
	})

	t.Run("Directory Not Found for Repository", func(t *testing.T) {
		intReader := newThreadSafeIntegrationReader()
		paramReader := newThreadSafeParametersReader()
		svc := contextresolver.NewContextResolutionService(intReader, paramReader)

		intReader.AddConfig(orgID, "repositories", contextresolver.IntegrationConfigModel{
			UUID:      uuid.New(),
			TeamID:    teamID,
			ConfigKey: "repositories",
			ConfigValue: []map[string]any{
				{"id": repoID, "name": "ScanDrix Core"},
			},
		})

		paramReader.SetParam("code_review_config", orgID, teamID, &contextresolver.ParameterModel{
			UUID: uuid.New(),
			Key:  "code_review_config",
			ConfigValue: []map[string]any{
				{
					"id":   repoID,
					"name": "ScanDrix Core",
					"directories": []map[string]any{
						{"id": "dir-pkg", "path": "pkg/models"},
						{"id": "dir-core", "path": "internal/core"},
					},
				},
			},
		})

		p, err := svc.GetDirectoryPathByOrganizationAndRepository(ctx, orgID, repoID, "dir-nonexistent")
		assert.Error(t, err)
		assert.ErrorIs(t, err, contextresolver.ErrDirectoryNotFound)
		assert.Empty(t, p)
	})

	t.Run("Successful Directory Path Resolution", func(t *testing.T) {
		intReader := newThreadSafeIntegrationReader()
		paramReader := newThreadSafeParametersReader()
		svc := contextresolver.NewContextResolutionService(intReader, paramReader)

		intReader.AddConfig(orgID, "repositories", contextresolver.IntegrationConfigModel{
			UUID:      uuid.New(),
			TeamID:    teamID,
			ConfigKey: "repositories",
			ConfigValue: []map[string]any{
				{"id": repoID, "name": "ScanDrix Core"},
			},
		})

		paramReader.SetParam("code_review_config", orgID, teamID, &contextresolver.ParameterModel{
			UUID: uuid.New(),
			Key:  "code_review_config",
			ConfigValue: []map[string]any{
				{
					"id":   repoID,
					"name": "ScanDrix Core",
					"directories": []map[string]any{
						{"id": "dir-pkg", "path": "pkg/models"},
						{"id": "dir-core", "path": "internal/core"},
						{"id": "dir-api", "path": "apps/api"},
					},
				},
			},
		})

		path, err := svc.GetDirectoryPathByOrganizationAndRepository(ctx, orgID, repoID, "dir-core")
		require.NoError(t, err)
		assert.Equal(t, "internal/core", path)

		pathPkg, errPkg := svc.GetDirectoryPathByOrganizationAndRepository(ctx, orgID, repoID, "dir-pkg")
		require.NoError(t, errPkg)
		assert.Equal(t, "pkg/models", pathPkg)
	})
}

func TestContextResolutionMatrix_GetRepositoryName_Matrix(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.New()
	teamID := uuid.New()
	repoID := "repo-super"

	t.Run("Short-Circuit on Nil or Empty", func(t *testing.T) {
		svc := contextresolver.NewContextResolutionService(newThreadSafeIntegrationReader(), newThreadSafeParametersReader())

		n1, err1 := svc.GetRepositoryNameByOrganizationAndRepository(ctx, uuid.Nil, repoID)
		assert.NoError(t, err1)
		assert.Empty(t, n1)

		n2, err2 := svc.GetRepositoryNameByOrganizationAndRepository(ctx, orgID, "")
		assert.NoError(t, err2)
		assert.Empty(t, n2)
	})

	t.Run("Successful Resolution with Wrapped Repositories Format", func(t *testing.T) {
		intReader := newThreadSafeIntegrationReader()
		paramReader := newThreadSafeParametersReader()
		svc := contextresolver.NewContextResolutionService(intReader, paramReader)

		intReader.AddConfig(orgID, "repositories", contextresolver.IntegrationConfigModel{
			UUID:      uuid.New(),
			TeamID:    teamID,
			ConfigKey: "repositories",
			ConfigValue: map[string]any{
				"repositories": []map[string]any{
					{"id": repoID, "name": "ScanDrix / Engine"},
				},
			},
		})

		paramReader.SetParam("code_review_config", orgID, teamID, &contextresolver.ParameterModel{
			UUID: uuid.New(),
			Key:  "code_review_config",
			ConfigValue: map[string]any{
				"repositories": []map[string]any{
					{"id": repoID, "name": "ScanDrix / Engine"},
				},
			},
		})

		name, err := svc.GetRepositoryNameByOrganizationAndRepository(ctx, orgID, repoID)
		require.NoError(t, err)
		assert.Equal(t, "ScanDrix / Engine", name)
	})
}

// ============================================================================
// High-Concurrency Multi-Tenant Resolution Stress Test
// ============================================================================

func TestContextResolutionMatrix_HighConcurrencyMultiTenantStress(t *testing.T) {
	const numOrgs = 10
	const reposPerOrg = 5
	const dirsPerRepo = 3
	const concurrencyWorkers = 50
	const iterationsPerWorker = 20

	intReader := newThreadSafeIntegrationReader()
	paramReader := newThreadSafeParametersReader()
	svc := contextresolver.NewContextResolutionService(intReader, paramReader)

	type testRepo struct {
		orgID    uuid.UUID
		teamID   uuid.UUID
		repoID   string
		repoName string
		dirs     map[string]string // dirID -> path
	}

	var allRepos []testRepo

	// Prepopulate fixtures
	for o := 0; o < numOrgs; o++ {
		orgID := uuid.New()
		teamID := uuid.New()

		var repoList []map[string]any

		for r := 0; r < reposPerOrg; r++ {
			repoID := fmt.Sprintf("repo-org%d-%d", o, r)
			repoName := fmt.Sprintf("Repository Org%d %d", o, r)
			dirMap := make(map[string]string)
			var dirSlice []map[string]any

			for d := 0; d < dirsPerRepo; d++ {
				dirID := fmt.Sprintf("dir-%d-%d-%d", o, r, d)
				dirPath := fmt.Sprintf("src/module_%d/sub_%d", o, d)
				dirMap[dirID] = dirPath
				dirSlice = append(dirSlice, map[string]any{
					"id":   dirID,
					"path": dirPath,
				})
			}

			repoList = append(repoList, map[string]any{
				"id":          repoID,
				"name":        repoName,
				"directories": dirSlice,
			})

			allRepos = append(allRepos, testRepo{
				orgID:    orgID,
				teamID:   teamID,
				repoID:   repoID,
				repoName: repoName,
				dirs:     dirMap,
			})
		}

		intReader.AddConfig(orgID, "repositories", contextresolver.IntegrationConfigModel{
			UUID:        uuid.New(),
			TeamID:      teamID,
			ConfigKey:   "repositories",
			ConfigValue: repoList,
		})

		paramReader.SetParam("code_review_config", orgID, teamID, &contextresolver.ParameterModel{
			UUID:        uuid.New(),
			Key:         "code_review_config",
			ConfigValue: repoList,
		})
	}

	var wg sync.WaitGroup
	wg.Add(concurrencyWorkers)

	var successTeamCount atomic.Int64
	var successNameCount atomic.Int64
	var successDirCount atomic.Int64

	ctx := context.Background()

	for w := 0; w < concurrencyWorkers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for iter := 0; iter < iterationsPerWorker; iter++ {
				idx := (workerID*iterationsPerWorker + iter) % len(allRepos)
				target := allRepos[idx]

				// 1. Resolve Team ID
				resolvedTeam, err := svc.GetTeamIDByOrganizationAndRepository(ctx, target.orgID, target.repoID)
				if err == nil && resolvedTeam == target.teamID {
					successTeamCount.Add(1)
				}

				// 2. Resolve Repository Name
				resolvedName, err := svc.GetRepositoryNameByOrganizationAndRepository(ctx, target.orgID, target.repoID)
				if err == nil && resolvedName == target.repoName {
					successNameCount.Add(1)
				}

				// 3. Resolve all directory paths
				for dirID, expectedPath := range target.dirs {
					resolvedPath, err := svc.GetDirectoryPathByOrganizationAndRepository(ctx, target.orgID, target.repoID, dirID)
					if err == nil && resolvedPath == expectedPath {
						successDirCount.Add(1)
					}
				}
			}
		}(w)
	}

	wg.Wait()

	totalExpectedOps := int64(concurrencyWorkers * iterationsPerWorker)
	assert.Equal(t, totalExpectedOps, successTeamCount.Load())
	assert.Equal(t, totalExpectedOps, successNameCount.Load())
	assert.Equal(t, totalExpectedOps*dirsPerRepo, successDirCount.Load())
}

// ============================================================================
// Resolver (JWT & Team Key) Matrix Tests
// ============================================================================

func TestResolverMatrix_ExtractFromRequest_Variations(t *testing.T) {
	jwtSecret := "test-secret-key-32-bytes-long-super-secure"
	wsRepo := newMatrixWorkspaceRepo()
	keyLookup := newMatrixTeamCliKeyLookup()

	resolver := contextresolver.NewResolver(jwtSecret, wsRepo, nil).WithTeamKeyLookup(keyLookup)

	wsID := uuid.New()
	wsRepo.Register(&domain.Workspace{
		BaseEntity: domain.BaseEntity{ID: wsID},
		Name:       "Production Org",
		Slug:       "prod-org",
		Status:     "ACTIVE",
		Tier:       "ENTERPRISE",
	})

	t.Run("No Auth Headers Returns ErrUnauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/reviews", nil)
		u, tenant, err := resolver.ExtractFromRequest(req)
		assert.ErrorIs(t, err, contextresolver.ErrUnauthorized)
		assert.Nil(t, u)
		assert.Nil(t, tenant)
	})

	t.Run("Header x-team-key Success", func(t *testing.T) {
		rawKey := "scandrix_live_0123456789abcdef0123456789abcdef"
		hash := crypto.HashToken(rawKey)

		teamID := uuid.New()
		keyID := uuid.New()
		keyLookup.RegisterKey(&domain.TeamCliKey{
			TenantScopedEntity: domain.TenantScopedEntity{
				BaseEntity:  domain.BaseEntity{ID: keyID},
				WorkspaceID: wsID,
			},
			TeamID:    teamID,
			KeyHash:   hash,
			KeyPrefix: "scandrix_live",
			Name:      "CI/CD Worker Key",
		})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/reviews", nil)
		req.Header.Set("x-team-key", rawKey)

		u, tenant, err := resolver.ExtractFromRequest(req)
		require.NoError(t, err)
		require.NotNil(t, u)
		require.NotNil(t, tenant)

		assert.Equal(t, "TEAM_API_KEY", u.AuthMethod)
		assert.Equal(t, wsID, u.WorkspaceID)
		assert.Contains(t, u.TeamIDs, teamID)
		assert.Equal(t, wsID, tenant.WorkspaceID)
		assert.Equal(t, "ENTERPRISE", tenant.Tier)
	})

	t.Run("Header Authorization Bearer scandrix_* Success", func(t *testing.T) {
		rawKey := "scandrix_ci_abcdef0123456789abcdef0123456789"
		hash := crypto.HashToken(rawKey)

		teamID := uuid.New()
		keyID := uuid.New()
		keyLookup.RegisterKey(&domain.TeamCliKey{
			TenantScopedEntity: domain.TenantScopedEntity{
				BaseEntity:  domain.BaseEntity{ID: keyID},
				WorkspaceID: wsID,
			},
			TeamID:    teamID,
			KeyHash:   hash,
			KeyPrefix: "scandrix_ci",
			Name:      "PR Reviewer Bot",
		})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/reviews", nil)
		req.Header.Set("Authorization", "Bearer "+rawKey)

		u, tenant, err := resolver.ExtractFromRequest(req)
		require.NoError(t, err)
		require.NotNil(t, u)
		require.NotNil(t, tenant)

		assert.Equal(t, "TEAM_API_KEY", u.AuthMethod)
		assert.Equal(t, wsID, u.WorkspaceID)
	})

	t.Run("Header Authorization JWT Bearer Token Success", func(t *testing.T) {
		userID := uuid.New()
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub":            userID.String(),
			"workspace_id":   wsID.String(),
			"email":          "developer@scandrix.dev",
			"role":           "DEVELOPER",
			"is_super_admin": false,
			"exp":            time.Now().Add(time.Hour).Unix(),
		})
		tokenString, err := token.SignedString([]byte(jwtSecret))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/reviews", nil)
		req.Header.Set("Authorization", "Bearer "+tokenString)

		u, tenant, err := resolver.ExtractFromRequest(req)
		require.NoError(t, err)
		require.NotNil(t, u)
		require.NotNil(t, tenant)

		assert.Equal(t, userID, u.UserID)
		assert.Equal(t, wsID, u.WorkspaceID)
		assert.Equal(t, "developer@scandrix.dev", u.Email)
		assert.Equal(t, "DEVELOPER", u.Role)
		assert.False(t, u.IsSuperAdmin)
		assert.Equal(t, "JWT", u.AuthMethod)
	})

	t.Run("Cookie scandrix_session Fallback Success", func(t *testing.T) {
		userID := uuid.New()
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub":          userID.String(),
			"workspace_id": wsID.String(),
			"email":        "admin@scandrix.dev",
			"role":         "ORG_ADMIN",
			"exp":          time.Now().Add(time.Hour).Unix(),
		})
		tokenString, err := token.SignedString([]byte(jwtSecret))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		req.AddCookie(&http.Cookie{
			Name:  "scandrix_session",
			Value: tokenString,
		})

		u, tenant, err := resolver.ExtractFromRequest(req)
		require.NoError(t, err)
		require.NotNil(t, u)
		require.NotNil(t, tenant)

		assert.Equal(t, userID, u.UserID)
		assert.Equal(t, "admin@scandrix.dev", u.Email)
		assert.Equal(t, "ORG_ADMIN", u.Role)
	})
}

func TestResolverMatrix_JWT_AdversarialTampering(t *testing.T) {
	jwtSecret := "matrix-test-jwt-secret-key-long"
	wsRepo := newMatrixWorkspaceRepo()
	resolver := contextresolver.NewResolver(jwtSecret, wsRepo, nil)

	wsID := uuid.New()
	wsRepo.Register(&domain.Workspace{
		BaseEntity: domain.BaseEntity{ID: wsID},
		Name:       "Matrix Workspace",
		Status:     "ACTIVE",
	})

	t.Run("Invalid Signing Algorithm None", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
			"sub":          uuid.New().String(),
			"workspace_id": wsID.String(),
			"role":         "ADMIN",
			"exp":          time.Now().Add(time.Hour).Unix(),
		})
		tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("Authorization", "Bearer "+tokenString)

		_, _, err = resolver.ExtractFromRequest(req)
		assert.ErrorIs(t, err, contextresolver.ErrInvalidToken)
	})

	t.Run("Expired JWT Token", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub":          uuid.New().String(),
			"workspace_id": wsID.String(),
			"exp":          time.Now().Add(-10 * time.Minute).Unix(),
		})
		tokenString, err := token.SignedString([]byte(jwtSecret))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("Authorization", "Bearer "+tokenString)

		_, _, err = resolver.ExtractFromRequest(req)
		assert.ErrorIs(t, err, contextresolver.ErrInvalidToken)
	})

	t.Run("Tampered Signature", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub":          uuid.New().String(),
			"workspace_id": wsID.String(),
			"exp":          time.Now().Add(time.Hour).Unix(),
		})
		tokenString, err := token.SignedString([]byte("wrong-secret-signature"))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("Authorization", "Bearer "+tokenString)

		_, _, err = resolver.ExtractFromRequest(req)
		assert.ErrorIs(t, err, contextresolver.ErrInvalidToken)
	})

	t.Run("Suspended Workspace Rejection", func(t *testing.T) {
		suspendedWsID := uuid.New()
		wsRepo.Register(&domain.Workspace{
			BaseEntity: domain.BaseEntity{ID: suspendedWsID},
			Name:       "Suspended Workspace",
			Status:     "SUSPENDED",
		})

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub":          uuid.New().String(),
			"workspace_id": suspendedWsID.String(),
			"exp":          time.Now().Add(time.Hour).Unix(),
		})
		tokenString, err := token.SignedString([]byte(jwtSecret))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("Authorization", "Bearer "+tokenString)

		_, _, err = resolver.ExtractFromRequest(req)
		assert.ErrorIs(t, err, contextresolver.ErrWorkspaceSuspended)
	})
}

func TestResolverMatrix_TeamCliKey_PrefixAndStatusInvariants(t *testing.T) {
	jwtSecret := "team-key-matrix-secret"
	wsRepo := newMatrixWorkspaceRepo()
	keyLookup := newMatrixTeamCliKeyLookup()
	resolver := contextresolver.NewResolver(jwtSecret, wsRepo, nil).WithTeamKeyLookup(keyLookup)

	wsID := uuid.New()
	wsRepo.Register(&domain.Workspace{
		BaseEntity: domain.BaseEntity{ID: wsID},
		Name:       "CliKey Workspace",
		Status:     "ACTIVE",
	})

	t.Run("Reject Non-ScanDrix Prefix", func(t *testing.T) {
		invalidPrefixKey := "unauthorized_prefix_key_12345"
		req := httptest.NewRequest(http.MethodGet, "/api", nil)
		req.Header.Set("x-team-key", invalidPrefixKey)

		_, _, err := resolver.ExtractFromRequest(req)
		assert.ErrorIs(t, err, contextresolver.ErrInvalidToken)
	})

	t.Run("Reject Revoked Team CLI Key", func(t *testing.T) {
		rawKey := "scandrix_revoked_abcdef0123456789"
		hash := crypto.HashToken(rawKey)
		revokedTime := time.Now().Add(-time.Hour)

		keyLookup.RegisterKey(&domain.TeamCliKey{
			TenantScopedEntity: domain.TenantScopedEntity{
				BaseEntity:  domain.BaseEntity{ID: uuid.New()},
				WorkspaceID: wsID,
			},
			TeamID:    uuid.New(),
			KeyHash:   hash,
			KeyPrefix: "scandrix_revoked",
			RevokedAt: &revokedTime, // Revoked
		})

		req := httptest.NewRequest(http.MethodGet, "/api", nil)
		req.Header.Set("x-team-key", rawKey)

		_, _, err := resolver.ExtractFromRequest(req)
		assert.ErrorIs(t, err, contextresolver.ErrInvalidToken)
	})

	t.Run("UpdateLastUsed Atomic Counter on Success", func(t *testing.T) {
		rawKey := "scandrix_active_9876543210fedcba"
		hash := crypto.HashToken(rawKey)

		keyID := uuid.New()
		keyLookup.RegisterKey(&domain.TeamCliKey{
			TenantScopedEntity: domain.TenantScopedEntity{
				BaseEntity:  domain.BaseEntity{ID: keyID},
				WorkspaceID: wsID,
			},
			TeamID:    uuid.New(),
			KeyHash:   hash,
			KeyPrefix: "scandrix_active",
		})

		req := httptest.NewRequest(http.MethodGet, "/api", nil)
		req.Header.Set("x-team-key", rawKey)

		for i := 0; i < 5; i++ {
			u, tenant, err := resolver.ExtractFromRequest(req)
			require.NoError(t, err)
			assert.NotNil(t, u)
			assert.NotNil(t, tenant)
		}

		// Allow async update last used goroutines to complete
		time.Sleep(50 * time.Millisecond)

		keyLookup.mu.RLock()
		hits := keyLookup.lastUsedHits[keyID]
		keyLookup.mu.RUnlock()
		assert.Equal(t, int64(5), hits)
	})
}

func TestResolverMatrix_ContextMiddlewareInjectionAndExtraction(t *testing.T) {
	wsID := uuid.New()
	userID := uuid.New()

	originalTenant := &contextresolver.TenantContext{
		WorkspaceID: wsID,
		Tier:        "ENTERPRISE",
		SpendLimit:  500.0,
		SpendUsage:  12.5,
	}

	originalUser := &contextresolver.UserIdentity{
		UserID:       userID,
		WorkspaceID:  wsID,
		Email:        "architect@scandrix.dev",
		Role:         "ARCHITECT",
		IsSuperAdmin: true,
		AuthMethod:   "JWT",
		Permissions: map[string]bool{
			"admin:all": true,
		},
	}

	ctx := context.Background()
	ctx = contextresolver.WithContext(ctx, originalTenant, originalUser)

	extractedTenant, okTenant := contextresolver.GetTenantContext(ctx)
	require.True(t, okTenant)
	require.NotNil(t, extractedTenant)
	assert.Equal(t, wsID, extractedTenant.WorkspaceID)
	assert.Equal(t, "ENTERPRISE", extractedTenant.Tier)
	assert.Equal(t, 500.0, extractedTenant.SpendLimit)

	extractedUser, okUser := contextresolver.GetUserIdentity(ctx)
	require.True(t, okUser)
	require.NotNil(t, extractedUser)
	assert.Equal(t, userID, extractedUser.UserID)
	assert.Equal(t, "architect@scandrix.dev", extractedUser.Email)
	assert.True(t, extractedUser.IsSuperAdmin)
	assert.True(t, extractedUser.Permissions["admin:all"])

	// Test Empty Context Fallbacks
	emptyCtx := context.Background()
	eTenant, hasTenant := contextresolver.GetTenantContext(emptyCtx)
	assert.False(t, hasTenant)
	assert.Nil(t, eTenant)

	eUser, hasUser := contextresolver.GetUserIdentity(emptyCtx)
	assert.False(t, hasUser)
	assert.Nil(t, eUser)
}

// Ensure HMAC helper utility is exercised
func BenchmarkHMACSignatureCheck(b *testing.B) {
	key := []byte("benchmark-secret-key-scandrix")
	msg := []byte("sample-data-payload-to-sign")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h := hmac.New(sha256.New, key)
		h.Write(msg)
		_ = h.Sum(nil)
	}
}
