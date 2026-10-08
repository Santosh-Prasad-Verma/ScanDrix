package controllers_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/internal/integrations/github"
	"github.com/scandrix/backend/internal/review"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/require"
)

type settingsSCMSpy struct{ reviews, checks int }

func (s *settingsSCMSpy) SubmitPullRequestReview(context.Context, string, string, int, github.PullReviewSubmission) error {
	s.reviews++
	return nil
}
func (s *settingsSCMSpy) UpdateCheckRun(context.Context, string, string, int64, github.UpdateCheckRunRequest) error {
	s.checks++
	return nil
}

// Applies actual migrations and uses a non-superuser, non-BYPASSRLS role for
// every application query. The optional browser check uses these real rows.
func TestRepositorySettingsPostgres(t *testing.T) {
	dsn := os.Getenv("SCANDRIX_ANALYTICS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SCANDRIX_ANALYTICS_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer admin.Close()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	schema, role := "settings_test_"+suffix, "settings_role_"+suffix
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema+"; CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS")
	require.NoError(t, err)
	defer func() {
		_, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE; DROP ROLE "+role)
		require.NoError(t, err)
	}()
	config, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	config.ConnConfig.RuntimeParams["search_path"] = schema
	setup, err := pgxpool.NewWithConfig(ctx, config.Copy())
	require.NoError(t, err)
	defer setup.Close()
	for _, filename := range []string{
		"../../../migrations/001_initial_schema.sql",
		"../../../migrations/044_repository_review_settings.sql",
		"../../../migrations/048_repository_code_review_config.sql",
	} {
		migration, err := os.ReadFile(filename)
		require.NoError(t, err)
		_, err = setup.Exec(ctx, string(migration))
		require.NoError(t, err)
	}
	_, err = setup.Exec(ctx, "GRANT USAGE ON SCHEMA "+schema+" TO "+role+"; GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA "+schema+" TO "+role)
	require.NoError(t, err)
	ws, foreignWS, repoID, secondRepo, foreignRepo := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	_, err = setup.Exec(ctx, `INSERT INTO workspaces(id,slug,name) VALUES ($1,'settings-test','Settings test'),($2,'foreign-test','Foreign test')`, ws, foreignWS)
	require.NoError(t, err)
	_, err = setup.Exec(ctx, `INSERT INTO tracked_repositories(id,workspace_id,provider,external_id,namespace_path) VALUES ($1,$4,'github','primary','test/primary'),($2,$4,'github','secondary','test/secondary'),($3,$5,'github','foreign','other/private')`, repoID, secondRepo, foreignRepo, ws, foreignWS)
	require.NoError(t, err)
	config.AfterConnect = func(ctx context.Context, connection *pgx.Conn) error {
		_, err := connection.Exec(ctx, "SET ROLE "+role)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)
	defer pool.Close()
	repository := database.NewRepository(&database.Client{Pool: pool})
	codeController := controllers.NewCodeManagementController(repository)
	parameterController := controllers.NewParametersController(repository)
	currentRole := models.RoleOwner
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestContext := auth.WithWorkspaceContext(r.Context(), ws)
			requestContext = auth.WithAccountContext(requestContext, &models.AccountProfile{WorkspaceID: ws, Role: currentRole})
			next.ServeHTTP(w, r.WithContext(requestContext))
		})
	})
	router.Mount("/repositories", codeController.CLIRepositoriesConfigRoutes(parameterController))
	request := func(method, id, body string) *httptest.ResponseRecorder {
		t.Helper()
		path := "/repositories/" + id + "/settings"
		if method == "DELETE" {
			path = "/repositories/" + id
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	path := repoID.String()
	w := request("PATCH", path, `{"configValue":{"autoReviewEnabled":false,"dryRunEnabled":true,"branchesMonitored":["release/*"],"ignoredPaths":["vendor/**"],"modelOverride":"test-model"}}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	w = request("GET", path, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	var response struct {
		Data struct {
			RepositoryID uuid.UUID                       `json:"repositoryId"`
			ConfigValue  models.RepositoryReviewSettings `json:"configValue"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, repoID, response.Data.RepositoryID)
	require.False(t, response.Data.ConfigValue.AutoReviewEnabled)
	require.True(t, response.Data.ConfigValue.DryRunEnabled)
	require.Equal(t, []string{"release/*"}, response.Data.ConfigValue.BranchesMonitored)
	stored, err := repository.GetRepositoryReviewSettings(ctx, ws, repoID)
	require.NoError(t, err)
	require.Equal(t, response.Data.ConfigValue, stored)

	// Independent concurrent patches must both survive the JSONB merge.
	ignoreBots, active := true, false
	errors := make(chan error, 2)
	go func() {
		_, err := repository.PatchRepositoryReviewSettings(ctx, ws, repoID, models.RepositoryReviewSettingsPatch{IgnoreBots: &ignoreBots})
		errors <- err
	}()
	go func() {
		_, err := repository.PatchRepositoryReviewSettings(ctx, ws, repoID, models.RepositoryReviewSettingsPatch{Active: &active})
		errors <- err
	}()
	require.NoError(t, <-errors)
	require.NoError(t, <-errors)
	stored, err = repository.GetRepositoryReviewSettings(ctx, ws, repoID)
	require.NoError(t, err)
	require.False(t, stored.Active)
	require.True(t, stored.IgnoreBots)
	require.Equal(t, "test-model", stored.ModelOverride)
	_, err = repository.GetRepositoryReviewSettings(ctx, ws, foreignRepo)
	require.ErrorIs(t, err, pgx.ErrNoRows)
	require.Equal(t, 404, request("GET", foreignRepo.String(), "").Code)
	require.Equal(t, 404, request("PATCH", foreignRepo.String(), `{"configValue":{"active":false}}`).Code)
	foreignStored, err := repository.GetRepositoryReviewSettings(ctx, foreignWS, foreignRepo)
	require.NoError(t, err)
	require.True(t, foreignStored.Active)
	currentRole = models.RoleViewer
	require.Equal(t, 403, request("PATCH", path, `{"configValue":{"active":true}}`).Code)
	currentRole = models.RoleOwner
	// Explicit clearing is persisted, rather than substituted with branch defaults.
	require.Equal(t, 200, request("PATCH", path, `{"configValue":{"active":true,"ignoreBots":false,"branchesMonitored":[],"ignoredPaths":[],"modelOverride":""}}`).Code)

	// The production orchestrator reads that exact persisted policy: automatic
	// jobs are skipped; manual dry-run jobs are persisted but never published.
	engine := review.NewOrchestrator(repository, nil, nil, nil)
	publisher := &settingsSCMSpy{}
	engine.SetSCMPublisher(publisher)
	task := review.ExecutionTask{ReviewID: uuid.New(), WorkspaceID: ws, RepositoryID: repoID, RepoNamespace: "test/primary", PullNumber: 8, Author: "fixture-author", BaseBranch: "main", Title: "Policy acceptance", RawDiff: "diff --git a/main.txt b/main.txt\n--- a/main.txt\n+++ b/main.txt\n@@ -0,0 +1 @@\n+fixture\n"}
	require.NoError(t, engine.ProcessReview(ctx, task))
	execution, err := repository.GetReview(ctx, ws, task.ReviewID)
	require.NoError(t, err)
	require.Equal(t, models.ReviewStateSkipped, execution.State)
	require.NotNil(t, execution.CompletedAt)
	task.ReviewID, task.Manual = uuid.New(), true
	require.NoError(t, engine.ProcessReview(ctx, task))
	execution, err = repository.GetReview(ctx, ws, task.ReviewID)
	require.NoError(t, err)
	require.Equal(t, models.ReviewStateCompleted, execution.State)
	require.Zero(t, publisher.reviews)
	require.Zero(t, publisher.checks)

	historyID := uuid.New()
	require.NoError(t, repository.CreateReview(ctx, &models.PullRequestReview{ID: historyID, WorkspaceID: ws, RepositoryID: secondRepo, Title: "Retained review", State: models.ReviewStateCompleted}))
	require.Equal(t, 204, request("DELETE", secondRepo.String(), "").Code)
	tracked, err := repository.ListTrackedRepositories(ctx, ws)
	require.NoError(t, err)
	require.Len(t, tracked, 1)
	_, err = repository.GetReview(ctx, ws, historyID)
	require.NoError(t, err)
	require.Equal(t, 404, request("PATCH", secondRepo.String(), `{"configValue":{"active":true}}`).Code)

	// Browser checks are test-only and opt-in. The server mounts production
	// controllers and JWT middleware, never canned API responses.
	if script := os.Getenv("SCANDRIX_SETTINGS_BROWSER_CHECK"); script != "" {
		for i := 0; i < 60; i++ {
			_, err := repository.TrackRepository(ctx, ws, models.ProviderGitHub, "browser-"+uuid.NewString(), "test/browser-"+uuid.NewString(), "main")
			require.NoError(t, err)
		}
		secret := os.Getenv("JWT_SECRET")
		require.NotEmpty(t, secret)
		authenticator := auth.NewAuthenticator(secret)
		owner, viewer := uuid.New(), uuid.New()
		ownerToken, err := authenticator.GenerateTokenWithEmail(owner, ws, models.RoleOwner, "owner@example.test")
		require.NoError(t, err)
		viewerToken, err := authenticator.GenerateTokenWithEmail(viewer, ws, models.RoleViewer, "viewer@example.test")
		require.NoError(t, err)
		browserRouter := chi.NewRouter()
		browserRouter.Use(authenticator.Middleware)
		browserRouter.Mount("/api/v1/code-management", codeController.Routes())
		browserRouter.Mount("/api/v1/cli/config/repositories", codeController.CLIRepositoriesConfigRoutes(parameterController))
		browserRouter.Mount("/api/v1/cockpit", controllers.NewCockpitController(repository).WithAnalytics(database.NewPostgresAnalyticsRepository(repository.Client())).Routes())
		browserRouter.Mount("/api/v1/pull-requests", controllers.NewPullRequestController(repository, nil).Routes())
		server := httptest.NewUnstartedServer(browserRouter)
		server.Listener.Close()
		server.Listener, err = net.Listen("tcp", os.Getenv("SCANDRIX_TEST_API_ADDR"))
		require.NoError(t, err)
		server.Start()
		defer server.Close()
		command := exec.CommandContext(ctx, "node", script)
		command.Env = append(os.Environ(), "SCANDRIX_TEST_OWNER_TOKEN="+ownerToken, "SCANDRIX_TEST_VIEWER_TOKEN="+viewerToken, "SCANDRIX_TEST_REPOSITORY_ID="+repoID.String())
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		require.NoError(t, command.Run())
		stored, err = repository.GetRepositoryReviewSettings(ctx, ws, repoID)
		require.NoError(t, err)
		require.Equal(t, []string{"main", "release/*"}, stored.BranchesMonitored)
		require.Equal(t, []string{"vendor/**"}, stored.IgnoredPaths)
	}
}
