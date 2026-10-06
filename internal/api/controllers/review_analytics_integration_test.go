package controllers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/internal/database"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/require"
)

// Uses an isolated schema on an explicitly supplied local/test database. Rows
// are real Postgres fixtures; no analytics result is mocked.
func TestReviewAnalyticsPostgres(t *testing.T) {
	dsn := os.Getenv("SCANDRIX_ANALYTICS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SCANDRIX_ANALYTICS_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer admin.Close()
	schema := "analytics_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	defer func() { _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); require.NoError(t, err) }()
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(ctx, `
		CREATE TABLE tracked_repositories(id uuid,workspace_id uuid,namespace_path text);
		CREATE TABLE pull_request_reviews(id uuid,workspace_id uuid,repository_id uuid,state text,created_at timestamptz,completed_at timestamptz,pull_number int,title text);
		CREATE TABLE code_findings(id uuid,review_id uuid,workspace_id uuid,file_path text,start_line int,end_line int,severity text,category text,title text,description text,remediation text,suggested_diff text,fingerprint text,created_at timestamptz);
		CREATE TABLE tracked_issues(id uuid,workspace_id uuid,repository_id uuid,title text,description text,file_path text,start_line int,end_line int,severity text,category text,status text,origin_review_id uuid,remediation text,fingerprint text,external_issue_url text,created_at timestamptz,updated_at timestamptz,resolved_at timestamptz);
	`)
	require.NoError(t, err)
	ws, other, repoA, repoB := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO tracked_repositories VALUES ($1,$3,'test/api'),($2,$3,'test/ui')`, repoA, repoB, ws)
	require.NoError(t, err)
	reviews := []struct {
		id, tenant, repo  uuid.UUID
		state, start, end string
	}{
		{uuid.New(), ws, repoA, "COMPLETED", "2026-10-01T00:00:00Z", "2026-10-01T00:02:00Z"},
		{uuid.New(), ws, repoA, "FAILED", "2026-10-02T00:00:00Z", "2026-10-02T00:01:00Z"},
		{uuid.New(), ws, repoB, "COMPLETED", "2026-10-03T00:00:00Z", "2026-10-03T00:04:00Z"},
		{uuid.New(), other, repoA, "COMPLETED", "2026-10-01T00:00:00Z", "2026-10-01T01:00:00Z"},
		{uuid.New(), ws, repoA, "COMPLETED", "2026-09-01T00:00:00Z", "2026-09-01T01:00:00Z"},
		{uuid.New(), ws, repoA, "COMPLETED", "2026-10-05T00:00:00Z", "2026-10-05T01:00:00Z"},
	}
	for i, r := range reviews {
		_, err = pool.Exec(ctx, `INSERT INTO pull_request_reviews VALUES ($1,$2,$3,$4,$5,$6,$7,'Test review')`, r.id, r.tenant, r.repo, r.state, r.start, r.end, i+1)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO code_findings VALUES($1,$2,$3,'src/auth.go',1,2,'HIGH','SECURITY','Validate input','Test description','Validate input',NULL,'fixture',$4)`, uuid.New(), r.id, r.tenant, r.start)
		require.NoError(t, err)
	}
	client := &database.Client{Pool: pool}
	repository := database.NewRepository(client)
	ctrl := controllers.NewCockpitController(repository).WithAnalytics(database.NewPostgresAnalyticsRepository(client))
	issueCtrl := controllers.NewIssuesController(repository)
	role := models.RoleMember
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), auth.WorkspaceContextKey, ws)
			ctx = auth.WithAccountContext(ctx, &models.AccountProfile{WorkspaceID: ws, Role: role})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	router.Mount("/cockpit", ctrl.Routes())
	router.Mount("/issues", issueCtrl.Routes())
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	window := "start=2026-10-01T00:00:00Z&end=2026-10-05T00:00:00Z"
	w := request("GET", "/cockpit/review-summary?"+window, "")
	require.Equal(t, 200, w.Code, w.Body.String())
	var report models.ReviewAnalytics
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &report))
	require.Equal(t, 3, report.Total)
	require.Equal(t, 2, report.Completed)
	require.Equal(t, 1, report.Failed)
	require.Equal(t, 3, report.Findings)
	require.Equal(t, 3.0, *report.MeanMinutes) // (2+4)/2; tenant/time exclusions must hold.
	require.Nil(t, report.ImplementationRate)
	require.Len(t, report.Unavailable, 2)
	w = request("GET", "/cockpit/review-summary?"+window+"&repositoryId="+repoA.String(), "")
	require.Equal(t, 200, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &report))
	require.Equal(t, 2, report.Total)
	require.Equal(t, 2, report.Findings)
	require.Equal(t, 2.0, *report.MeanMinutes)
	w = request("GET", "/cockpit/suggestions?"+window+"&limit=1&page=2&severity=HIGH", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	var suggestions models.SuggestionSearchResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &suggestions))
	require.Equal(t, 3, suggestions.Total)
	require.Len(t, suggestions.Items, 1)
	require.Equal(t, reviews[1].id, suggestions.Items[0].ReviewID)
	for _, query := range []string{"repositoryId=invalid", "start=invalid", "start=2026-10-05T00:00:00Z&end=2026-10-01T00:00:00Z"} {
		require.Equal(t, 400, request("GET", "/cockpit/review-summary?"+query, "").Code)
	}
	require.Equal(t, 400, request("GET", "/cockpit/suggestions?severity=BAD", "").Code)
	require.Equal(t, 400, request("GET", "/cockpit/suggestions?page=-1", "").Code)
	w = request("GET", "/cockpit/review-summary?"+window+"&repositoryId="+uuid.NewString(), "")
	require.Equal(t, 200, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &report))
	require.Equal(t, 0, report.Total)
	require.Nil(t, report.MeanMinutes)

	issueID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO tracked_issues VALUES($1,$2,$3,'Fixture issue','Description','a.go',1,2,'HIGH','SECURITY','OPEN',$4,'Remediation','fixture','',now(),now(),NULL)`, issueID, ws, repoA, reviews[0].id)
	require.NoError(t, err)
	path := "/issues/" + issueID.String()
	role = models.RoleViewer
	require.Equal(t, 403, request("PATCH", path, `{"status":"RESOLVED"}`).Code)
	role = models.RoleMember
	require.Equal(t, 400, request("PATCH", path, `{"status":"INVALID"}`).Code)
	w = request("PATCH", path, `{"status":"RESOLVED"}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	var resolved bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT status='RESOLVED' AND resolved_at IS NOT NULL FROM tracked_issues WHERE id=$1`, issueID).Scan(&resolved))
	require.True(t, resolved)
	require.Equal(t, 200, request("PATCH", path, `{"status":"OPEN"}`).Code)
	require.NoError(t, pool.QueryRow(ctx, `SELECT status='OPEN' AND resolved_at IS NULL FROM tracked_issues WHERE id=$1`, issueID).Scan(&resolved))
	require.True(t, resolved)
	_, err = pool.Exec(ctx, `UPDATE tracked_issues SET workspace_id=$1 WHERE id=$2`, other, issueID)
	require.NoError(t, err)
	require.Equal(t, 404, request("PATCH", path, `{"status":"RESOLVED"}`).Code)
}
