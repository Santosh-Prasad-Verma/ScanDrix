package controllers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/require"
)

type settingsTransportStore struct {
	controllers.ParametersRepository
	calls                 int
	err                   error
	workspace, repository uuid.UUID
	patch                 models.RepositoryReviewSettingsPatch
}

func (s *settingsTransportStore) GetRepositoryReviewSettings(_ context.Context, ws, repo uuid.UUID) (models.RepositoryReviewSettings, error) {
	s.calls++
	s.workspace, s.repository = ws, repo
	cfg, _ := models.DecodeRepositoryReviewSettings(nil, true)
	return cfg, s.err
}
func (s *settingsTransportStore) PatchRepositoryReviewSettings(ctx context.Context, ws, repo uuid.UUID, patch models.RepositoryReviewSettingsPatch) (models.RepositoryReviewSettings, error) {
	s.patch = patch
	return s.GetRepositoryReviewSettings(ctx, ws, repo)
}
func (s *settingsTransportStore) UntrackRepository(_ context.Context, ws, repo uuid.UUID) error {
	s.calls++
	s.workspace, s.repository = ws, repo
	return s.err
}

func TestRepositorySettingsAuthorizationAndValidation(t *testing.T) {
	ws, repo := uuid.New(), uuid.New()
	for _, tc := range []struct {
		name, method, role, id, body string
		status                       int
		err                          error
	}{
		{"viewer can read", "GET", "VIEWER", repo.String(), "", 200, nil},
		{"owner can patch false and empty", "PATCH", "OWNER", repo.String(), `{"configValue":{"autoReviewEnabled":false,"ignoredPaths":[],"modelOverride":""}}`, 200, nil},
		{"admin can patch", "PATCH", "admin", repo.String(), `{"configValue":{"dryRunEnabled":true}}`, 200, nil},
		{"member denied", "PATCH", "MEMBER", repo.String(), `{"configValue":{}}`, 403, nil},
		{"viewer denied", "PATCH", "VIEWER", repo.String(), `{"configValue":{}}`, 403, nil},
		{"unknown denied", "PATCH", "unexpected", repo.String(), `{"configValue":{}}`, 403, nil},
		{"profile absent denied", "PATCH", "", repo.String(), `{"configValue":{}}`, 403, nil},
		{"provider ID rejected", "PATCH", "OWNER", "123456", `{"configValue":{}}`, 400, nil},
		{"nil UUID rejected", "GET", "OWNER", uuid.Nil.String(), "", 400, nil},
		{"unknown field rejected", "PATCH", "OWNER", repo.String(), `{"configValue":{"admin":true}}`, 400, nil},
		{"null field rejected", "PATCH", "OWNER", repo.String(), `{"configValue":{"ignoreBots":null}}`, 400, nil},
		{"null config rejected", "PATCH", "OWNER", repo.String(), `{"configValue":null}`, 400, nil},
		{"invalid type rejected", "PATCH", "OWNER", repo.String(), `{"configValue":{"active":"false"}}`, 400, nil},
		{"invalid glob rejected", "PATCH", "OWNER", repo.String(), `{"configValue":{"ignoredPaths":["["]}}`, 400, nil},
		{"trailing JSON rejected", "PATCH", "OWNER", repo.String(), `{"configValue":{}} {}`, 400, nil},
		{"oversized input rejected", "PATCH", "OWNER", repo.String(), `{"configValue":{"modelOverride":"` + strings.Repeat("x", 65536) + `"}}`, 400, nil},
		{"foreign repository absent", "GET", "VIEWER", repo.String(), "", 404, pgx.ErrNoRows},
		{"failed write cannot succeed", "PATCH", "OWNER", repo.String(), `{"configValue":{"active":false}}`, 500, errors.New("private database diagnostic")},
		{"owner untracks", "DELETE", "OWNER", repo.String(), "", 204, nil},
		{"member cannot untrack", "DELETE", "MEMBER", repo.String(), "", 403, nil},
		{"failed untrack cannot succeed", "DELETE", "ADMIN", repo.String(), "", 500, errors.New("private database diagnostic")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &settingsTransportStore{err: tc.err}
			router := chi.NewRouter()
			router.Mount("/repositories", controllers.NewCodeManagementController(nil).CLIRepositoriesConfigRoutes(controllers.NewParametersController(store)))
			path := "/repositories/" + tc.id + "/settings"
			if tc.method == "DELETE" {
				path = "/repositories/" + tc.id
			}
			request := httptest.NewRequest(tc.method, path, strings.NewReader(tc.body))
			ctx := context.WithValue(request.Context(), auth.WorkspaceContextKey, ws)
			if tc.role != "" {
				ctx = auth.WithAccountContext(ctx, &models.AccountProfile{WorkspaceID: ws, Role: models.UserRole(tc.role)})
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request.WithContext(ctx))
			require.Equal(t, tc.status, response.Code, response.Body.String())
			require.NotContains(t, response.Body.String(), "private database diagnostic")
			if tc.status == 400 || tc.status == 403 {
				require.Zero(t, store.calls)
			} else {
				require.Equal(t, ws, store.workspace)
				require.Equal(t, repo, store.repository)
			}
			if tc.name == "owner can patch false and empty" {
				require.NotNil(t, store.patch.AutoReviewEnabled)
				require.False(t, *store.patch.AutoReviewEnabled)
				require.NotNil(t, store.patch.IgnoredPaths)
				require.Empty(t, *store.patch.IgnoredPaths)
				require.NotNil(t, store.patch.ModelOverride)
				require.Empty(t, *store.patch.ModelOverride)
			}
		})
	}
}

func TestRepositorySettingsUnavailable(t *testing.T) {
	router := chi.NewRouter()
	router.Mount("/repositories", controllers.NewCodeManagementController(nil).CLIRepositoriesConfigRoutes(controllers.NewParametersController(nil)))
	request := httptest.NewRequest(http.MethodGet, "/repositories/"+uuid.NewString()+"/settings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, request)
	require.Equal(t, 401, w.Code)
	w = httptest.NewRecorder()
	ctx := context.WithValue(request.Context(), auth.WorkspaceContextKey, uuid.New())
	router.ServeHTTP(w, request.WithContext(ctx))
	require.Equal(t, 503, w.Code)
}

/**
 * The per-repository settings tabs edited fourteen fields that the write path then
 * discarded, and the UI reported a successful save. This pins the contract: the
 * full configuration document must reach the store instead of being filtered down
 * to the handful of fields that happen to have fixed backend columns.
 */
func TestRepositorySettingsPersistTheFullConfigurationDocument(t *testing.T) {
	ws, repo := uuid.New(), uuid.New()
	store := &settingsTransportStore{}

	ctrl := controllers.NewParametersController(store)
	cliRoutes := controllers.NewCodeManagementController(nil).CLIRepositoriesConfigRoutes(ctrl)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			req = req.WithContext(context.WithValue(req.Context(), auth.WorkspaceContextKey, ws))
			req = req.WithContext(auth.WithAccountContext(req.Context(), &models.AccountProfile{WorkspaceID: ws, Role: models.RoleOwner}))
			next.ServeHTTP(w, req)
		})
	})
	r.Mount("/repositories", cliRoutes)

	body := `{"configValue":{
		"codeReviewConfig":{
			"customMessages":{"enabled":true,"welcomeComment":"hi","footerDisclaimer":"d","customPrompt":"p"},
			"summary":{"generatePRSummary":true,"behaviourForExistingDescription":"append","includeWalkthrough":true,"includeDiagrams":true},
			"suggestionControl":{"groupingMode":"by_file","limitationType":"file","maxSuggestions":40,"severityLevelFilter":"high","applyFiltersToDrixyRules":false},
			"reviewOptions":{"security":true,"performance":false,"maintainability":true,"style":true},
			"linkedRepositories":["acme/other"],
			"ignoredTitleKeywords":["[WIP]","[DO NOT MERGE]"],
			"runOnDraft":true,
			"isRequestChangesActive":true,
			"pullRequestApprovalActive":true,
			"enableCommittableSuggestions":false,
			"ideRulesSyncEnabled":false,
			"codeReviewVersion":"v2"
		}}}`

	req := httptest.NewRequest("PATCH", "/repositories/"+repo.String()+"/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code, w.Body.String())

	require.NotNil(t, store.patch.CodeReviewConfig, "the configuration document must reach the store")
	doc := *store.patch.CodeReviewConfig

	// Spot-check one value from each previously-dropped group.
	require.Equal(t, map[string]any{"enabled": true, "welcomeComment": "hi", "footerDisclaimer": "d", "customPrompt": "p"}, doc["customMessages"])
	require.Equal(t, true, doc["runOnDraft"])
	require.Equal(t, true, doc["isRequestChangesActive"])
	require.Equal(t, true, doc["pullRequestApprovalActive"])
	require.Equal(t, false, doc["enableCommittableSuggestions"])
	require.Equal(t, false, doc["ideRulesSyncEnabled"])
	require.Equal(t, []any{"acme/other"}, doc["linkedRepositories"])
	require.Equal(t, []any{"[WIP]", "[DO NOT MERGE]"}, doc["ignoredTitleKeywords"])

	suggestion, ok := doc["suggestionControl"].(map[string]any)
	require.True(t, ok, "suggestionControl must survive as a nested document")
	require.Equal(t, "by_file", suggestion["groupingMode"])
	require.Equal(t, float64(40), suggestion["maxSuggestions"])

	// The repository must stay the one that was addressed: the workspace-wide
	// parameters endpoint ignores repositoryId, which is why this document does not
	// travel that way.
	require.Equal(t, repo, store.repository)
	require.Equal(t, ws, store.workspace)
}
