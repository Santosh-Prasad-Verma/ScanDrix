package controllers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/codeanalysis/graph"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubCodeManagementRepo records which provider each destructive call targeted so
// the tests can assert the delete stayed scoped to one connection.
type stubCodeManagementRepo struct {
	connections map[uuid.UUID]models.IntegrationConnection

	deletedProviders  []models.SCMProvider
	untrackedProvider []models.SCMProvider
}

func newStubCodeManagementRepo() *stubCodeManagementRepo {
	return &stubCodeManagementRepo{connections: map[uuid.UUID]models.IntegrationConnection{}}
}

func (s *stubCodeManagementRepo) ListTrackedRepositories(context.Context, uuid.UUID) ([]models.TrackedRepository, error) {
	return nil, nil
}

func (s *stubCodeManagementRepo) GetIntegrationConnection(_ context.Context, _ uuid.UUID, provider models.SCMProvider) (*models.IntegrationConnection, error) {
	for _, c := range s.connections {
		if c.Provider == provider {
			found := c
			return &found, nil
		}
	}
	return nil, nil
}

func (s *stubCodeManagementRepo) GetIntegrationConnectionByID(_ context.Context, _ uuid.UUID, id uuid.UUID) (*models.IntegrationConnection, error) {
	c, ok := s.connections[id]
	if !ok {
		return nil, errNoRows
	}
	return &c, nil
}

func (s *stubCodeManagementRepo) TrackRepository(context.Context, uuid.UUID, models.SCMProvider, string, string, string) (*models.TrackedRepository, error) {
	return nil, nil
}

func (s *stubCodeManagementRepo) UpdateIntegrationRepoCount(context.Context, uuid.UUID, int) error {
	return nil
}

func (s *stubCodeManagementRepo) UpsertIntegrationConnection(context.Context, uuid.UUID, models.SCMProvider, string, string, bool, int) error {
	return nil
}

func (s *stubCodeManagementRepo) DeleteIntegrationConnection(_ context.Context, _ uuid.UUID, provider models.SCMProvider) error {
	s.deletedProviders = append(s.deletedProviders, provider)
	return nil
}

func (s *stubCodeManagementRepo) UntrackAllRepositories(_ context.Context, _ uuid.UUID, provider models.SCMProvider) error {
	s.untrackedProvider = append(s.untrackedProvider, provider)
	return nil
}

func (s *stubCodeManagementRepo) InsertAuditLog(context.Context, uuid.UUID, string, string, string, string, string, string, []byte) error {
	return nil
}

func (s *stubCodeManagementRepo) GetASTNodesByRepository(context.Context, uuid.UUID, uuid.UUID) ([]graph.ASTNode, error) {
	return nil, nil
}

func disconnectRequest(t *testing.T, repo controllers.CodeManagementRepository, wsID uuid.UUID, query string) *httptest.ResponseRecorder {
	t.Helper()
	ctrl := controllers.NewCodeManagementController(repo)
	r := chi.NewRouter()
	r.Mount("/", ctrl.Routes())

	req := httptest.NewRequest(http.MethodDelete, "/delete-integration"+query, nil)
	req = req.WithContext(auth.WithWorkspaceContext(req.Context(), wsID))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

/**
 * Regression guard for destructive behaviour.
 *
 * This handler used to ignore its connectionId argument entirely and always
 * delete both the GitHub and the GitLab connection along with every repository
 * tracked under them, then returned success. Disconnecting a Bitbucket
 * credential therefore destroyed the workspace's GitHub integration with no
 * error surfaced anywhere. These tests pin the scoping so it cannot regress.
 */
func TestDeleteIntegrationIsScopedToTheNamedConnection(t *testing.T) {
	wsID := uuid.New()
	bitbucketID := uuid.New()
	githubID := uuid.New()

	repo := newStubCodeManagementRepo()
	repo.connections[bitbucketID] = models.IntegrationConnection{ID: bitbucketID, WorkspaceID: wsID, Provider: models.ProviderBitbucket}
	repo.connections[githubID] = models.IntegrationConnection{ID: githubID, WorkspaceID: wsID, Provider: models.ProviderGitHub}

	w := disconnectRequest(t, repo, wsID, "?connectionId="+bitbucketID.String())
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Only the requested provider is touched.
	assert.Equal(t, []models.SCMProvider{models.ProviderBitbucket}, repo.deletedProviders,
		"disconnect must delete exactly one provider, not every provider")
	assert.Equal(t, []models.SCMProvider{models.ProviderBitbucket}, repo.untrackedProvider,
		"untracking must be scoped to the same provider")
	assert.NotContains(t, repo.deletedProviders, models.ProviderGitHub,
		"disconnecting a Bitbucket credential must never delete the GitHub integration")

	var payload struct {
		Data struct {
			Success  bool   `json:"success"`
			Provider string `json:"provider"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.True(t, payload.Data.Success)
	assert.Equal(t, string(models.ProviderBitbucket), payload.Data.Provider)
}

func TestDeleteIntegrationRejectsAMissingOrUnknownConnection(t *testing.T) {
	wsID := uuid.New()
	repo := newStubCodeManagementRepo()
	repo.connections[uuid.New()] = models.IntegrationConnection{ID: uuid.New(), WorkspaceID: wsID, Provider: models.ProviderGitHub}

	t.Run("missing connectionId is a bad request", func(t *testing.T) {
		w := disconnectRequest(t, repo, wsID, "")
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Empty(t, repo.deletedProviders, "nothing may be deleted without a named connection")
	})

	t.Run("malformed connectionId is a bad request", func(t *testing.T) {
		w := disconnectRequest(t, repo, wsID, "?connectionId=not-a-uuid")
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Empty(t, repo.deletedProviders)
	})

	t.Run("unknown connectionId is not reported as success", func(t *testing.T) {
		w := disconnectRequest(t, repo, wsID, "?connectionId="+uuid.New().String())
		assert.Equal(t, http.StatusNotFound, w.Code,
			"an unknown connection must not return 200 success")
		assert.Empty(t, repo.deletedProviders,
			"an unknown connection must not fall through to deleting everything")
	})
}
// errNoRows mirrors the driver sentinel the real repository returns for an
// unknown connection id.
var errNoRows = pgx.ErrNoRows
