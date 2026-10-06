package controllers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/scandrix/backend/internal/api/controllers"
	"github.com/scandrix/backend/internal/auth"
	"github.com/scandrix/backend/pkg/models"
	"github.com/stretchr/testify/assert"
)

type mockCockpitRepo struct{}

func (m *mockCockpitRepo) CountActiveReviews(ctx context.Context, wsID uuid.UUID) (int, error) {
	return 3, nil
}

func (m *mockCockpitRepo) ListTrackedRepositories(ctx context.Context, wsID uuid.UUID) ([]models.TrackedRepository, error) {
	return []models.TrackedRepository{
		{ID: uuid.New(), NamespacePath: "acme/api", IsActive: true},
	}, nil
}

type mockSpendRepo struct {
	limit float64
}

func (m *mockSpendRepo) GetSpendLimitStatus(ctx context.Context, wsID uuid.UUID) (*models.SpendLimitEvaluation, error) {
	return &models.SpendLimitEvaluation{
		SpentUSD:            45.50,
		LimitUSD:            m.limit,
		PercentageUsed:      18.2,
		IsOverLimit:         false,
		MonthlyBudgetUSD:    m.limit,
		AlertThresholdPct:   85.0,
		NotificationEnabled: true,
	}, nil
}

func (m *mockSpendRepo) GetWorkspaceUsage(ctx context.Context, wsID uuid.UUID, since time.Time) (int64, int64, float64, error) {
	return 100000, 25000, 45.50, nil
}

func (m *mockSpendRepo) UpdateSpendLimit(ctx context.Context, wsID uuid.UUID, limitUSD float64) error {
	m.limit = limitUSD
	return nil
}

type mockMessagesRepo struct {
	settings *models.PullRequestMessageSettings
}

func (m *mockMessagesRepo) GetPullRequestMessages(ctx context.Context, wsID uuid.UUID, repoID *uuid.UUID, directoryID string) (*models.PullRequestMessageSettings, error) {
	if m.settings != nil {
		return m.settings, nil
	}
	return &models.PullRequestMessageSettings{
		WorkspaceID: wsID,
		ConfigLevel: "global",
		StartReviewMessage: models.MessageContentWithStatus{
			Content: "Custom ScanDrix started review...",
			Status:  models.PRMessageStatusActive,
		},
		HideComments:         false,
		SuggestionCopyPrompt: true,
	}, nil
}

func (m *mockMessagesRepo) SavePullRequestMessages(ctx context.Context, wsID uuid.UUID, settings *models.PullRequestMessageSettings) error {
	m.settings = settings
	return nil
}

func TestCockpitController(t *testing.T) {
	wsID := uuid.New()
	ctrl := controllers.NewCockpitController(&mockCockpitRepo{})
	r := chi.NewRouter()

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})

	ctrl.WithAnalytics(&fakeAnalytics{})
	r.Mount("/cockpit", ctrl.Routes())
	r.Mount("/code-health", ctrl.CodeHealthRoutes())
	r.Mount("/productivity", ctrl.ProductivityRoutes())

	// /cockpit/health and /cockpit/health/runs were removed: they returned
	// hardcoded "healthy" plus invented lag/failure counters. The real probes
	// are the router-level /healthz (DB ping) and /livez.

	// 1. /cockpit/overview
	req := httptest.NewRequest("GET", "/cockpit/overview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "activeRepositories")

	// 2. /code-health/overview
	req = httptest.NewRequest("GET", "/code-health/overview", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "healthScore")

	// 4. /productivity/overview
	req = httptest.NewRequest("GET", "/productivity/overview", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "cycleTimeHours")
}

// fakeAnalytics returns fixed values so the tests can assert that real numbers
// are passed through untouched, and that unavailable metrics stay null.
type fakeAnalytics struct{}

func (f *fakeAnalytics) GetCockpitOverview(_ context.Context, _ uuid.UUID) (*models.CockpitOverview, error) {
	mean := 4.5
	return &models.CockpitOverview{
		WorkspaceID:         "ws-1",
		ActiveReviews:       3,
		ActiveRepositories:  7,
		CompletedReviews:    11,
		MeanTimeToReviewMin: &mean,
		Unavailable: []models.AnalyticsUnavailable{
			{Metric: "securityScore", Reason: models.ReasonNoFormula},
		},
	}, nil
}

func (f *fakeAnalytics) GetDoraMetrics(_ context.Context, _ uuid.UUID) (*models.DoraMetrics, error) {
	return &models.DoraMetrics{
		Unavailable: []models.AnalyticsUnavailable{
			{Metric: "rating", Reason: models.ReasonNoDataSource, Detail: "no deployment table"},
		},
	}, nil
}

func (f *fakeAnalytics) GetProductivityMetrics(_ context.Context, _ uuid.UUID) (*models.ProductivityMetrics, error) {
	cycle := 6.0
	return &models.ProductivityMetrics{CycleTimeHours: &cycle}, nil
}

func (f *fakeAnalytics) GetCodeHealthMetrics(_ context.Context, _ uuid.UUID) (*models.CodeHealthMetrics, error) {
	critical := 2
	return &models.CodeHealthMetrics{
		WorkspaceID:       "ws-1",
		CriticalDebtFiles: &critical,
		TotalFindings:     9,
		Unavailable: []models.AnalyticsUnavailable{
			{Metric: "healthScore", Reason: models.ReasonNoDataSource},
		},
	}, nil
}

func (f *fakeAnalytics) ListCodeHotspotFiles(_ context.Context, _ uuid.UUID, _ int) ([]models.CodeHotspotFile, error) {
	return []models.CodeHotspotFile{{FilePath: "a.go", FindingsCount: 3, CriticalCount: 1}}, nil
}

// TestCockpitFailsClosedWithoutAnalytics asserts the metric endpoints refuse to
// answer when no analytics source is wired. They previously served hardcoded
// values, so a deployment with no data access looked identical to a healthy one.
func TestCockpitFailsClosedWithoutAnalytics(t *testing.T) {
	wsID := uuid.New()
	ctrl := controllers.NewCockpitController(nil)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(
				context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID)))
		})
	})
	r.Mount("/cockpit", ctrl.Routes())
	r.Mount("/code-health", ctrl.CodeHealthRoutes())
	r.Mount("/productivity", ctrl.ProductivityRoutes())

	for _, path := range []string{
		"/cockpit/overview", "/code-health/overview",
		"/code-health/files", "/productivity/overview", "/productivity/dora",
	} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code, "path %s must fail closed", path)
		assert.NotContains(t, w.Body.String(), "ELITE")
		assert.NotContains(t, w.Body.String(), "securityScore\":")
	}
}

// TestCockpitReportsUnavailableMetricsRatherThanValues asserts a metric that
// cannot be computed is null and named in the unavailable list, never a number.
func TestCockpitReportsUnavailableMetricsRatherThanValues(t *testing.T) {
	wsID := uuid.New()
	ctrl := controllers.NewCockpitController(nil).WithAnalytics(&fakeAnalytics{})
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(
				context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID)))
		})
	})
	r.Mount("/cockpit", ctrl.Routes())
	r.Mount("/code-health", ctrl.CodeHealthRoutes())
	r.Mount("/productivity", ctrl.ProductivityRoutes())

	req := httptest.NewRequest("GET", "/cockpit/overview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"activeRepositories":7`)
	assert.Contains(t, w.Body.String(), `"meanTimeToReviewMin":4.5`)
	// securityScore has no formula, so it must be null and explained.
	assert.Contains(t, w.Body.String(), `"securityScore":null`)
	assert.Contains(t, w.Body.String(), `"reason":"no_defined_formula"`)

	req = httptest.NewRequest("GET", "/productivity/dora", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"rating":null`)
	assert.Contains(t, w.Body.String(), `"reason":"no_data_source"`)
	assert.NotContains(t, w.Body.String(), "ELITE")
}

func TestSpendLimitController(t *testing.T) {
	wsID := uuid.New()
	spendRepo := &mockSpendRepo{limit: 250.0}
	ctrl := controllers.NewSpendLimitController(spendRepo)
	r := chi.NewRouter()

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID)
			// Spend-limit configuration is owner/admin only (auth.RoleGuard).
			ctx = auth.WithAccountContext(ctx, &models.AccountProfile{WorkspaceID: wsID, Role: models.RoleOwner})
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Mount("/spend-limit", ctrl.Routes())

	// 1. GET /spend-limit/status
	req := httptest.NewRequest("GET", "/spend-limit/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "spent_usd")

	// 2. GET /spend-limit
	req = httptest.NewRequest("GET", "/spend-limit", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "modelPrices")

	// 3. POST /spend-limit
	payload := []byte(`{"limitUsd": 500.0, "alertThresholdPct": 80.0, "notificationEnabled": true}`)
	req = httptest.NewRequest("POST", "/spend-limit", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "500")
}

func TestPullRequestMessagesController(t *testing.T) {
	wsID := uuid.New()
	msgRepo := &mockMessagesRepo{}
	ctrl := controllers.NewPullRequestMessagesController(msgRepo)
	r := chi.NewRouter()

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), auth.WorkspaceContextKey, wsID)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Mount("/pull-request-messages", ctrl.Routes())

	// 1. GET /pull-request-messages
	req := httptest.NewRequest("GET", "/pull-request-messages", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Custom ScanDrix started review...")

	// 2. POST /pull-request-messages
	body := map[string]any{
		"configLevel": "global",
		"startReviewMessage": map[string]any{
			"content": "Updated review message",
			"status":  "ACTIVE",
		},
	}
	bodyBytes, _ := json.Marshal(body)
	req = httptest.NewRequest("POST", "/pull-request-messages", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Updated review message")
}

type stubStatusRepo struct{ err error }

func (s stubStatusRepo) Ping(context.Context) error { return s.err }

func TestSystemController(t *testing.T) {
	r := chi.NewRouter()
	r.Mount("/system", controllers.NewSystemController(stubStatusRepo{}).Routes())

	req := httptest.NewRequest("GET", "/system/info", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "scandrix-api")

	// Version comes from RELEASE_VERSION, not a hardcoded literal.
	req = httptest.NewRequest("GET", "/system/version", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "commit")
}

// A database probe failure must surface as 503, never as a green status.
func TestSystemControllerStatusReflectsRealDependency(t *testing.T) {
	r := chi.NewRouter()
	r.Mount("/system", controllers.NewSystemController(stubStatusRepo{}).Routes())
	r2 := chi.NewRouter()
	r2.Mount("/system", controllers.NewSystemController(stubStatusRepo{err: errors.New("boom")}).Routes())

	req := httptest.NewRequest("GET", "/system/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"database":"up"`)

	w2 := httptest.NewRecorder()
	r2.ServeHTTP(w2, req)
	assert.Equal(t, http.StatusServiceUnavailable, w2.Code)
	assert.Contains(t, w2.Body.String(), `"status":"unhealthy"`)
}
